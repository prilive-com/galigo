package scrub_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/internal/testutil"
)

// B-118: countingTransport closes the request body on its error path (net/http.RoundTripper: "RoundTrip must always
// close the body, including on errors") and keeps its error and count.
func TestB118_ScrubTransports(t *testing.T) {
	t.Run("countingTransport", func(t *testing.T) {
		rt := &countingTransport{}
		body := strings.NewReader("b118")
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), rt, "http://h/x", body)
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, errBoom, "[b118:err]")
		assert.Equal(t, int32(1), rt.calls.Load(), "[b118:count]")
		assert.Equal(t, 4, body.Len(), "[b118:noread]")
	})
}
