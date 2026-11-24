package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/internal/testutil"
)

// B-118: the engine's failing test transports close the request body (net/http.RoundTripper: "RoundTrip must always
// close the body, including on errors") and keep their error and count.
func TestB118_EngineTransports(t *testing.T) {
	t.Run("adapterCountingTransport", func(t *testing.T) {
		rt := &adapterCountingTransport{}
		body := strings.NewReader("b118")
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), rt, "http://h/x", body)
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, errAdapterBoom, "[b118:err]")
		assert.Equal(t, int32(1), rt.calls.Load(), "[b118:count]")
		assert.Equal(t, 4, body.Len(), "[b118:noread]")
	})
	t.Run("b145FailTransport", func(t *testing.T) {
		errT6 := errors.New("b118 t6")
		rt := &b145FailTransport{err: errT6}
		body := strings.NewReader("b118")
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), rt, "http://h/x", body)
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, errT6, "[b118:err]")
		assert.Equal(t, int32(1), rt.n.Load(), "[b118:count]")
		assert.Equal(t, 4, body.Len(), "[b118:noread]")
	})
}
