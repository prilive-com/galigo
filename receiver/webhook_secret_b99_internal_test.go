package receiver

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// TestB99_HandlerSecretField: WebhookHandler keeps the secret as a tg.SecretToken, copied unchanged from the Config
// for every value class (AC3). reflect and string(), so the file builds while the field is still a string.
func TestB99_HandlerSecretField(t *testing.T) {
	t.Run("type", func(t *testing.T) {
		f, ok := reflect.TypeFor[WebhookHandler]().FieldByName("webhookSecret")
		require.True(t, ok)
		assert.Equal(t, "tg.SecretToken", f.Type.String())
	})
	for _, v := range append(testutil.WebhookSecretValues("B99-FAKE-webhook-secret-internal"), testutil.WebhookSecretOutside()...) {
		t.Run(v.Name, func(t *testing.T) {
			cfg := DefaultConfig()
			reflect.ValueOf(&cfg).Elem().FieldByName("WebhookSecret").SetString(v.Secret)
			h := NewWebhookHandler(nil, make(chan tg.Update, 1), cfg)
			assert.Equal(t, v.Secret, string(h.webhookSecret))
		})
	}
}
