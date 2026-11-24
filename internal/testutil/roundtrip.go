package testutil

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// closeRecorder is a request body that records Close.
type closeRecorder struct {
	io.Reader
	closed atomic.Bool
}

func (r *closeRecorder) Close() error {
	r.closed.Store(true)
	return nil
}

// Read fails after Close, as a pipe body does, so a close placed before a read is seen.
func (r *closeRecorder) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return r.Reader.Read(p)
}

// RoundTripClosesBody sends a POST to url with ctx through rt. The request body reads from body (an empty reader
// when body is nil) and records Close; a read after the close returns io.ErrClosedPipe. It reports whether the
// request body was closed when RoundTrip returned, for a transport that closes synchronously (a later close, for
// example by the response body's Close, does not count), and the error RoundTrip returned. It closes a returned
// response body. A request that cannot be built fails the test. net/http.RoundTripper: "RoundTrip must always close
// the body, including on errors" (B-118).
func RoundTripClosesBody(t *testing.T, ctx context.Context, rt http.RoundTripper, url string, body io.Reader) (bool, error) {
	t.Helper()
	if body == nil {
		body = strings.NewReader("")
	}
	rec := &closeRecorder{Reader: body}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, rec)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	closed := rec.closed.Load()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return closed, err
}
