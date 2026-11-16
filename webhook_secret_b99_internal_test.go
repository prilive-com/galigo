package galigo

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
)

const (
	b99Token  = "123456789:B99-FAKE-token-root-AAAAAAAAAAAAAA"
	b99Secret = "B99-FAKE-webhook-secret-root"
)

func b99NewBot(t *testing.T, secret string) *Bot {
	b, err := New(b99Token, WithWebhook(8443, secret))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// TestB99_BotSecretFields: botConfig keeps the secret as a tg.SecretToken; WithWebhook stores it and New copies it
// to the receiver Config unchanged, for every value Telegram accepts (AC3; New refuses the others, B-103). reflect and string(), so the file builds while the
// fields are strings.
func TestB99_BotSecretFields(t *testing.T) {
	t.Run("type", func(t *testing.T) {
		f, ok := reflect.TypeFor[botConfig]().FieldByName("webhookSecret")
		require.True(t, ok)
		assert.Equal(t, "tg.SecretToken", f.Type.String())
	})
	for _, v := range testutil.WebhookSecretValues(b99Secret) {
		t.Run(v.Name, func(t *testing.T) {
			b := b99NewBot(t, v.Secret)
			assert.Equal(t, v.Secret, string(b.config.webhookSecret), "botConfig")
			assert.Equal(t, v.Secret, string(b.config.receiverConfig.WebhookSecret), "receiver Config")
		})
	}
}

// TestB99_BotWebhookAuth: the handler of a Bot made with WithWebhook(port, secret) accepts only the secret and rejects
// every other header, for every value Telegram accepts; with WithWebhook(port, "") it accepts every request (AC4). A new Bot per
// request. It passes on the baseline too: behaviour kept. An accepted request delivers its update and a refused one
// delivers nothing (round t3 O-T3-01).
func TestB99_BotWebhookAuth(t *testing.T) {
	serve := func(t *testing.T, secret string, header *string) (int, int) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"update_id":1}`))
		if header != nil {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", *header)
		}
		rec := httptest.NewRecorder()
		b := b99NewBot(t, secret)
		b.WebhookHandler().ServeHTTP(rec, req)
		return rec.Code, len(b.Updates())
	}
	for _, v := range append(testutil.WebhookSecretValues(b99Secret), testutil.WebhookSecretValue{Name: "no_secret"}) {
		t.Run(v.Name, func(t *testing.T) {
			for _, c := range testutil.WebhookAuthCases(v.Secret) {
				code, delivered := serve(t, v.Secret, c.Header)
				assert.Equal(t, c.Want, code, c.Name)
				want := 0
				if c.Want == http.StatusOK {
					want = 1
				}
				assert.Equal(t, want, delivered, c.Name+": updates delivered")
			}
		})
	}
}
