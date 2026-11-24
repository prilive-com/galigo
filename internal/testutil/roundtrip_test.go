package testutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// B-118: the rows of RoundTripClosesBody. The url is not the literal of any mutant of the helper.
const b118URL = "http://example.invalid/b118"

var errT1 = errors.New("b118 t1")

type b118CtxKey struct{}

// b118RT runs fn as its RoundTrip.
type b118RT func(*http.Request) (*http.Response, error)

func (f b118RT) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// b118RespBody is a response body; its Close may close the request body too.
type b118RespBody struct {
	io.Reader
	onClose func()
	closed  atomic.Bool
}

func (b *b118RespBody) Close() error {
	b.closed.Store(true)
	if b.onClose != nil {
		b.onClose()
	}
	return nil
}

func TestB118_RoundTripClosesBody(t *testing.T) {
	ctx := context.Background()

	t.Run("closes", func(t *testing.T) {
		closed, err := RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			_ = req.Body.Close()
			return nil, errT1
		}), b118URL, strings.NewReader("b118"))
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, errT1, "[b118:err]")
	})

	t.Run("closes_nil_body", func(t *testing.T) {
		var got []byte
		closed, _ := RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			got, _ = io.ReadAll(req.Body)
			_ = req.Body.Close()
			return nil, errT1
		}), b118URL, nil)
		assert.True(t, closed, "[b118:close]")
		assert.Equal(t, "", string(got), "[b118:bytes]")
	})

	t.Run("never_closes", func(t *testing.T) {
		closed, _ := RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil // no body: the helper skips its close
		}), b118URL, strings.NewReader("b118"))
		assert.False(t, closed, "[b118:close]")
	})

	t.Run("reads_no_close", func(t *testing.T) {
		closed, _ := RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, req.Body)
			return nil, errT1
		}), b118URL, strings.NewReader("b118"))
		assert.False(t, closed, "[b118:close]")
	})

	t.Run("closes_on_response_close", func(t *testing.T) {
		resp := &b118RespBody{Reader: strings.NewReader("{}")}
		closed, _ := RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			resp.onClose = func() { _ = req.Body.Close() }
			return &http.Response{StatusCode: http.StatusOK, Body: resp, Request: req}, nil
		}), b118URL, strings.NewReader("b118"))
		assert.False(t, closed, "[b118:close]") // a close made by the response body's Close is not a close by RoundTrip
		assert.True(t, resp.closed.Load(), "[b118:respclose]")
	})

	t.Run("reads_caller_body", func(t *testing.T) {
		var got []byte
		_, _ = RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			got, _ = io.ReadAll(req.Body)
			_ = req.Body.Close()
			return nil, errT1
		}), b118URL, strings.NewReader("b118"))
		assert.Equal(t, "b118", string(got), "[b118:bytes]")
	})

	t.Run("passes_ctx", func(t *testing.T) {
		var got any
		cctx := context.WithValue(ctx, b118CtxKey{}, "b118-marker")
		_, _ = RoundTripClosesBody(t, cctx, b118RT(func(req *http.Request) (*http.Response, error) {
			got = req.Context().Value(b118CtxKey{})
			_ = req.Body.Close()
			return nil, errT1
		}), b118URL, strings.NewReader("b118"))
		assert.Equal(t, "b118-marker", got, "[b118:ctx]")
	})

	t.Run("passes_url", func(t *testing.T) {
		var got, method string
		_, _ = RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			got, method = req.URL.String(), req.Method
			_ = req.Body.Close()
			return nil, errT1
		}), b118URL, strings.NewReader("b118"))
		assert.Equal(t, b118URL, got, "[b118:url]")
		assert.Equal(t, http.MethodPost, method, "[b118:method]")
	})

	t.Run("read_after_close", func(t *testing.T) {
		var rerr error
		closed, _ := RoundTripClosesBody(t, ctx, b118RT(func(req *http.Request) (*http.Response, error) {
			_ = req.Body.Close()
			_, rerr = req.Body.Read(make([]byte, 1))
			return nil, errT1
		}), b118URL, strings.NewReader("b118"))
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, rerr, io.ErrClosedPipe, "[b118:readclosed]")
	})
}
