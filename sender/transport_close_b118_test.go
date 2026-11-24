package sender_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/internal/testutil"
)

// B-118: the sender's failing test transports close the request body (net/http.RoundTripper: "RoundTrip must
// always close the body, including on errors") without reading it, keep their error, and count one call.
func TestB118_SenderTransports(t *testing.T) {
	t.Run("api101bFailTransport", func(t *testing.T) {
		rt := &api101bFailTransport{}
		body := strings.NewReader("b118")
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), rt, "http://h/x", body)
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, api101bTimeout{}, "[b118:err]")
		assert.Equal(t, int32(1), rt.n.Load(), "[b118:count]")
		assert.Equal(t, 4, body.Len(), "[b118:noread]")
	})
	t.Run("api101dFailTransport", func(t *testing.T) {
		rt := &api101dFailTransport{}
		body := strings.NewReader("b118")
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), rt, "http://h/x", body)
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, api101dTimeout{}, "[b118:err]")
		assert.Equal(t, int32(1), rt.n.Load(), "[b118:count]")
		assert.Equal(t, 4, body.Len(), "[b118:noread]")
	})
}
