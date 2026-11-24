package receiver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/internal/testutil"
)

// B-118: the receiver's internal test transports close the request body (net/http.RoundTripper: "RoundTrip must
// always close the body, including on errors") and keep their error and behaviour.
func TestB118_ReceiverInternalTransports(t *testing.T) {
	t.Run("scrubCountingTransport", func(t *testing.T) {
		rt := &scrubCountingTransport{}
		body := strings.NewReader("b118")
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), rt, "http://h/x", body)
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, errScrubBoom, "[b118:err]")
		assert.Equal(t, int32(1), rt.calls.Load(), "[b118:count]")
		assert.Equal(t, 4, body.Len(), "[b118:noread]")
	})
	t.Run("nilClientStub/read_error", func(t *testing.T) {
		errT3 := errors.New("b118 t3 read")
		stub := &nilClientStub{}
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), stub, "http://h/x", iotest.ErrReader(errT3))
		assert.True(t, closed, "[b118:close]")
		assert.ErrorIs(t, err, errT3, "[b118:err]")
		assert.Empty(t, stub.reqs, "[b118:reqs]")
	})
	// The good path guards the moved close: it passes at the baseline too.
	t.Run("nilClientStub/ok", func(t *testing.T) {
		stub := &nilClientStub{status: 200, body: `{"ok":true,"result":true}`}
		closed, err := testutil.RoundTripClosesBody(t, context.Background(), stub, "http://h/x", strings.NewReader("b118"))
		assert.True(t, closed, "[b118:close]")
		assert.NoError(t, err, "[b118:err]")
		if assert.Len(t, stub.reqs, 1, "[b118:reqs]") {
			assert.Equal(t, "b118", string(stub.reqs[0].body), "[b118:body]")
		}
	})
}
