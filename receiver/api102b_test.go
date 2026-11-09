package receiver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// T-wh-1 (Bot API 10.2 part b): the webhook decodes an ephemeral message with receiver_user and an
// ephemeral_message_id above 2^32. Ids are fake.
func TestAPI102B_WebhookEphemeralMessage(t *testing.T) {
	const body = `{"update_id":9,"message":{"message_id":0,"date":1700000000,` +
		`"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"},` +
		`"receiver_user":{"id":987654321,"is_bot":false,"first_name":"Recv"},"ephemeral_message_id":5000000001}}`
	cfg := DefaultConfig()
	cfg.WebhookSecret = "test-secret-token"
	cfg.RateLimitRequests = 1000
	cfg.RateLimitBurst = 100
	updates := make(chan tg.Update, 1)
	h := NewWebhookHandler(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), updates, cfg)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", cfg.WebhookSecret)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	u := <-updates
	require.NotNil(t, u.Message)
	assert.Equal(t, 0, u.Message.MessageID)
	assert.Equal(t, 5000000001, u.Message.EphemeralMessageID)
	require.NotNil(t, u.Message.ReceiverUser)
	assert.Equal(t, int64(987654321), u.Message.ReceiverUser.ID)
	b, err := json.Marshal(u)
	require.NoError(t, err)
	assert.JSONEq(t, body, string(b))
}
