package receiver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 part a through the real receiver decode paths (webhook.go and polling.go). Ids are fake.
func api102aSubscriptionBody(state string) string {
	return `{"update_id":7,"subscription":{"user":{"id":123456789,"is_bot":false,"first_name":"Test"},` +
		`"invoice_payload":"sub-42","state":"` + state + `"}}`
}

func api102aCheckSub(t *testing.T, u tg.Update, body, state string) {
	t.Helper()
	assert.Equal(t, 7, u.UpdateID)
	require.NotNil(t, u.Subscription)
	require.NotNil(t, u.Subscription.User)
	assert.Equal(t, int64(123456789), u.Subscription.User.ID)
	assert.Equal(t, "Test", u.Subscription.User.FirstName)
	assert.Equal(t, "sub-42", u.Subscription.InvoicePayload)
	assert.Equal(t, state, u.Subscription.State)
	b, err := json.Marshal(u)
	require.NoError(t, err)
	assert.JSONEq(t, body, string(b))
}

func api102aWebhook(t *testing.T, body string) tg.Update {
	t.Helper()
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
	require.Len(t, updates, 1)
	return <-updates
}

// T-wh-1: a subscription update posted to the webhook.
func TestAPI102A_WebhookSubscription(t *testing.T) {
	body := api102aSubscriptionBody("canceled")
	api102aCheckSub(t, api102aWebhook(t, body), body, "canceled")
}

// T-wh-2: a community_chat_removed service message ({}) posted to the webhook.
func TestAPI102A_WebhookChatRemoved(t *testing.T) {
	u := api102aWebhook(t, `{"update_id":8,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},`+
		`"community_chat_removed":{}}}`)
	require.NotNil(t, u.Message)
	assert.NotNil(t, u.Message.CommunityChatRemoved)
	assert.Nil(t, u.Message.CommunityChatAdded)
}

// T-poll-1: a subscription update returned by getUpdates.
func TestAPI102A_PollingSubscription(t *testing.T) {
	body := api102aSubscriptionBody("active")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:abc/getUpdates" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[` + body + `]}`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL + "/bot"
	c := NewPollingClient(tg.SecretToken("123:abc"), make(chan tg.Update, 1), nil, cfg)
	got, err := c.fetchUpdates(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	api102aCheckSub(t, got[0], body, "active")
}
