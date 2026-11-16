package receiver_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

const (
	b103Token   = "123456789:B103-FAKE-token-receiver-AAAAAAAAA"
	b103LoadErr = "galigo: validation: WEBHOOK_SECRET - must be 1-256 characters, only A-Z, a-z, 0-9, _ and -"
)

// b103Env sets every variable LoadConfig validates to a valid value, so the caller's environment cannot change a row.
func b103Env(t *testing.T, mode string) {
	t.Helper()
	for k, v := range map[string]string{"RECEIVER_MODE": mode, "TELEGRAM_BOT_TOKEN": b103Token, "WEBHOOK_URL": "",
		"POLLING_TIMEOUT": "30", "POLLING_LIMIT": "100", "UPDATE_DELIVERY_POLICY": "block"} {
		t.Setenv(k, v)
	}
}

// TestB103_LoadConfig: in both receiver modes, LoadConfig refuses a non-empty WEBHOOK_SECRET outside Telegram's rule
// with a nil config and exactly the AC1 error, and loads every other value (empty, absent) unchanged (AC1, AC4). A
// NUL byte cannot be set in the environment (os.Setenv: "setenv: invalid argument"), so those rows are skipped.
func TestB103_LoadConfig(t *testing.T) {
	rows := append(testutil.WebhookSecretMatrix(), testutil.WebhookSecretCase{Name: "empty", Secret: "", OK: true})
	for _, mode := range []string{"webhook", "longpolling"} {
		t.Run(mode, func(t *testing.T) {
			for _, c := range rows {
				t.Run(c.Name, func(t *testing.T) {
					if strings.IndexByte(c.Secret, 0) >= 0 {
						t.Skip("a NUL byte cannot be set in the environment")
					}
					b103Env(t, mode)
					t.Setenv("WEBHOOK_SECRET", c.Secret)
					cfg, err := receiver.LoadConfig()
					if c.OK {
						require.NoError(t, err)
						assert.Equal(t, c.Secret, cfg.WebhookSecret.Value())
						return
					}
					assert.Nil(t, cfg)
					var ve *tg.ValidationError
					require.True(t, errors.As(err, &ve), "error %v is not a *tg.ValidationError", err)
					assert.Equal(t, b103LoadErr, err.Error())
				})
			}
			t.Run("absent", func(t *testing.T) {
				b103Env(t, mode)
				t.Setenv("WEBHOOK_SECRET", "") // restores the caller's value after the test
				require.NoError(t, os.Unsetenv("WEBHOOK_SECRET"))
				cfg, err := receiver.LoadConfig()
				require.NoError(t, err)
				assert.Equal(t, "", cfg.WebhookSecret.Value())
			})
		})
	}
}

// b103Handlers returns the constructor AssertWebhookHandlerLog needs: a handler for secret made with logger (nil:
// slog.Default()), with DefaultConfig or, for other, with the AssertWebhookOther settings.
func b103Handlers(logger *slog.Logger, secret string) func(bool, chan tg.Update) http.Handler {
	return func(other bool, updates chan tg.Update) http.Handler {
		cfg := receiver.DefaultConfig()
		cfg.WebhookSecret = tg.SecretToken(secret)
		if other {
			cfg.AllowedDomain = testutil.WebhookOtherHost
			cfg.MaxBodySize = testutil.WebhookOtherMaxBody
			cfg.RateLimitRequests = 0
			cfg.RateLimitBurst = testutil.WebhookOtherBurst
		}
		return receiver.NewWebhookHandler(logger, updates, cfg)
	}
}

// TestB103_Handler: for every secret, NewWebhookHandler logs exactly one WARN record with the fixed message and no
// attributes when the secret is non-empty and outside the rule, and nothing at any level otherwise; the handler keeps
// the secret and accepts only it (AC3: it warns, it does not refuse), and no later record or response body carries
// it (AC4; testutil.AssertWebhookHandlerLog).
func TestB103_Handler(t *testing.T) {
	rows := append(testutil.WebhookSecretMatrix(), testutil.WebhookSecretCase{Name: "empty", Secret: "", OK: true})
	for _, c := range rows {
		t.Run(c.Name, func(t *testing.T) {
			rec := testutil.NewLogRecorder()
			testutil.AssertWebhookHandlerLog(t, rec, b103Handlers(slog.New(rec), c.Secret), c)
		})
	}
}

// TestB103_HandlerDefaultLogger: with a nil logger, NewWebhookHandler writes the same records to slog.Default(), for
// every secret (AC3). It replaces the default logger, so it must not run in parallel.
func TestB103_HandlerDefaultLogger(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	rows := append(testutil.WebhookSecretMatrix(), testutil.WebhookSecretCase{Name: "empty", Secret: "", OK: true})
	for _, c := range rows {
		t.Run(c.Name, func(t *testing.T) {
			rec := testutil.NewLogRecorder()
			slog.SetDefault(slog.New(rec))
			testutil.AssertWebhookHandlerLog(t, rec, b103Handlers(nil, c.Secret), c)
		})
	}
}

// TestB103_SetWebhookUnchanged: SetWebhook still sends every secret of the matrix unchanged (as JSON can carry it), and
// no secret_token for an empty one (PLAN non-goal 1: Telegram refuses a secret outside the rule; galigo does not check it here). Offline:
// a local test server.
func TestB103_SetWebhookUnchanged(t *testing.T) {
	rows := append(testutil.WebhookSecretMatrix(), testutil.WebhookSecretCase{Name: "empty", Secret: "", OK: true})
	for _, c := range rows {
		secret := c.Secret
		t.Run(c.Name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true}))
			}))
			defer server.Close()
			client := server.Client()
			client.Transport = &urlRewriteTransport{base: client.Transport, targetURL: server.URL}
			err := receiver.SetWebhook(context.Background(), client, tg.SecretToken(b103Token), "https://example.com/hook", secret)
			require.NoError(t, err)
			got, sent := body["secret_token"]
			if secret == "" {
				assert.False(t, sent, "no secret_token for an empty secret")
				return
			}
			// encoding/json writes an invalid UTF-8 byte as U+FFFD, so the oracle is the secret as JSON carries it.
			raw, err := json.Marshal(secret)
			require.NoError(t, err)
			var want string
			require.NoError(t, json.Unmarshal(raw, &want))
			assert.Equal(t, want, got)
		})
	}
}
