package receiver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// rich101cWebhookBody is an update whose blocks value is no array, so it decodes as one RichBlockUnknown that keeps
// the bytes of the request (elements of an array are copied by json.RawMessage already). Bodies built with different
// letters have the same length.
func rich101cWebhookBody(letter string) string {
	return fmt.Sprintf(`{"update_id":1,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},`+
		`"rich_message":{"blocks":{"type":"future_%[1]s","v":"%[1]s"}}}}`, letter)
}

// T-copy-wh: the webhook decodes from a pooled buffer, so the Raw values kept from one update must not change when
// the next request reuses that buffer. sync.Pool reuse is not guaranteed, so the pool here has one buffer only.
func TestAPI101C_WebhookPooledBuffer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WebhookSecret = "test-secret-token"
	cfg.RateLimitRequests = 1000
	cfg.RateLimitBurst = 100
	cfg.MaxBodySize = 1 << 16
	updates := make(chan tg.Update, 2)
	h := NewWebhookHandler(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), updates, cfg)
	shared := make([]byte, cfg.MaxBodySize)
	h.bufferPool = sync.Pool{New: func() any { return &shared }}

	send := func(body string) tg.Update {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", cfg.WebhookSecret)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.True(t, bytes.HasPrefix(shared, []byte(body)), "the request did not use the shared buffer")
		return <-updates
	}
	body1, body2 := rich101cWebhookBody("a"), rich101cWebhookBody("b")
	require.Len(t, body2, len(body1))

	u1 := send(body1)
	send(body2)

	require.NotNil(t, u1.Message)
	require.NotNil(t, u1.Message.RichMessage)
	blocks := u1.Message.RichMessage.Blocks
	assert.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Raw: json.RawMessage(`{"type":"future_a","v":"a"}`)}}, blocks)
}
