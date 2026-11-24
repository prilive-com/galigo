package receiver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/internal/testutil"
)

// B-118: handlerTransport closes the request body on the paths that return before req.Write (which closes it
// itself), also through tagTransport, whose Request.Clone shares the body; each path keeps its error and serves
// nothing.
func TestB118_ReceiverTransports(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	const urlErr = "handlerTransport: unsupported scheme or empty host"
	for _, tc := range []struct {
		name, url string
		ctx       context.Context
		tag       bool
		wantIs    error
		wantText  string
	}{
		{name: "handlerTransport/cancelled", url: "http://h/x", ctx: cancelled, wantIs: context.Canceled},
		{name: "handlerTransport/bad_scheme", url: "ftp://h/x", ctx: context.Background(), wantText: urlErr},
		{name: "handlerTransport/empty_host", url: "http:///x", ctx: context.Background(), wantText: urlErr},
		{name: "tagTransport/cancelled", url: "http://h/x", ctx: cancelled, tag: true, wantIs: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newDeleteWebhookRecorder(b112DeleteOK)
			var rt http.RoundTripper = handlerTransport{rec: rec}
			if tc.tag {
				rt = tagTransport{base: handlerTransport{rec: rec}}
			}
			body := strings.NewReader("b118")
			closed, err := testutil.RoundTripClosesBody(t, tc.ctx, rt, tc.url, body)
			assert.True(t, closed, "[b118:close]")
			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs, "[b118:err]")
			} else {
				assert.EqualError(t, err, tc.wantText, "[b118:err]")
			}
			assert.Empty(t, rec.requests(), "[b118:served]")
			assert.Equal(t, 4, body.Len(), "[b118:noread]")
		})
	}
}
