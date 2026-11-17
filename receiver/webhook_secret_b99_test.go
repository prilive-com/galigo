package receiver_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

const (
	b99Token  = "123456789:B99-FAKE-token-receiver-AAAAAAAAAA"
	b99Secret = "B99-FAKE-webhook-secret-receiver"
)

// TestB99_ConfigFieldType: receiver.Config.WebhookSecret is a tg.SecretToken (AC1). It reads the type by reflection,
// so the file builds while the field is still a string.
func TestB99_ConfigFieldType(t *testing.T) {
	f, ok := reflect.TypeFor[receiver.Config]().FieldByName("WebhookSecret")
	require.True(t, ok)
	assert.Equal(t, "tg.SecretToken", f.Type.String())
}

// TestB99_LoadConfig: LoadConfig stores WEBHOOK_SECRET unchanged (every value Telegram accepts, empty, and "" when the variable
// is absent), and the field prints [REDACTED] (AC1, AC2). The other variables LoadConfig validates are set to valid
// values, so the caller's environment cannot fail the test.
func TestB99_LoadConfig(t *testing.T) {
	fixture := func(t *testing.T) {
		for k, v := range map[string]string{"RECEIVER_MODE": "webhook", "TELEGRAM_BOT_TOKEN": b99Token, "WEBHOOK_URL": "",
			"POLLING_TIMEOUT": "30", "POLLING_LIMIT": "100", "UPDATE_DELIVERY_POLICY": "block"} {
			t.Setenv(k, v)
		}
	}
	load := func(t *testing.T) *receiver.Config {
		cfg, err := receiver.LoadConfig()
		require.NoError(t, err)
		return cfg
	}
	rows := append(testutil.WebhookSecretValues(b99Secret), testutil.WebhookSecretValue{Name: "empty"})
	for _, r := range rows {
		t.Run(r.Name, func(t *testing.T) {
			fixture(t)
			t.Setenv("WEBHOOK_SECRET", r.Secret)
			cfg := load(t)
			assert.Equal(t, r.Secret, string(cfg.WebhookSecret))
			assert.Equal(t, "[REDACTED]", fmt.Sprintf("%v", cfg.WebhookSecret))
			if r.Name == "normal" {
				assert.NotContains(t, fmt.Sprintf("%+v", cfg), b99Secret)
			}
		})
	}
	t.Run("absent", func(t *testing.T) {
		fixture(t)
		t.Setenv("WEBHOOK_SECRET", "") // restores the caller's value after the test
		require.NoError(t, os.Unsetenv("WEBHOOK_SECRET"))
		cfg := load(t)
		assert.Equal(t, "", string(cfg.WebhookSecret))
	})
}

// b99Config is a Config with only the two secrets set, so its whole output is fixed text.
func b99Config() receiver.Config {
	return receiver.Config{Token: tg.SecretToken(b99Token), WebhookSecret: b99Secret}
}

// The %+v text of b99Config; slog's text handler writes the same text, quoted.
const b99PlusV = "{Mode: Token:[REDACTED] BaseURL: WebhookPort:0 TLSCertPath: TLSKeyPath: WebhookSecret:[REDACTED] AllowedDomain: WebhookURL: PollingTimeout:0 PollingLimit:0 PollingMaxErrors:0 DeleteWebhookFirst:false AllowedUpdates:[] RetryInitialDelay:0s RetryMaxDelay:0s RetryBackoffFactor:0 UpdateBufferSize:0 RateLimitRequests:0 RateLimitBurst:0 MaxBodySize:0 UpdateDeliveryPolicy:0 UpdateDeliveryTimeout:0s OnUpdateDropped:<nil> BreakerMaxRequests:0 BreakerInterval:0s BreakerTimeout:0s ReadTimeout:0s ReadHeaderTimeout:0s WriteTimeout:0s IdleTimeout:0s DrainDelay:0s ShutdownTimeout:0s}"

// TestB99_ConfigOutput compares the whole output of a Config, a *Config and a struct field holding one with exact
// text, so no form of the secret (plain, quoted, hex, a prefix) can be in it (AC2).
func TestB99_ConfigOutput(t *testing.T) {
	cfg := b99Config()
	rows := []struct {
		name, format string
		arg          any
		want         string
	}{
		{"v", "%v", cfg, "{ [REDACTED]  0   [REDACTED]   0 0 0 false [] 0s 0s 0 0 0 0 0 0 0s <nil> 0 0s 0s 0s 0s 0s 0s 0s 0s}"},
		{"plus_v", "%+v", cfg, b99PlusV},
		{"pointer_plus_v", "%+v", &cfg, "&" + b99PlusV},
		{"field_plus_v", "%+v", struct{ C receiver.Config }{cfg}, "{C:" + b99PlusV + "}"},
		{"sharp_v", "%#v", cfg, `receiver.Config{Mode:"", Token:tg.SecretToken("[REDACTED]"), BaseURL:"", WebhookPort:0, TLSCertPath:"", TLSKeyPath:"", WebhookSecret:tg.SecretToken("[REDACTED]"), AllowedDomain:"", WebhookURL:"", PollingTimeout:0, PollingLimit:0, PollingMaxErrors:0, DeleteWebhookFirst:false, AllowedUpdates:[]string(nil), RetryInitialDelay:0, RetryMaxDelay:0, RetryBackoffFactor:0, UpdateBufferSize:0, RateLimitRequests:0, RateLimitBurst:0, MaxBodySize:0, UpdateDeliveryPolicy:0, UpdateDeliveryTimeout:0, OnUpdateDropped:(func(int, string))(nil), BreakerMaxRequests:0x0, BreakerInterval:0, BreakerTimeout:0, ReadTimeout:0, ReadHeaderTimeout:0, WriteTimeout:0, IdleTimeout:0, DrainDelay:0, ShutdownTimeout:0}`},
		{"s", "%s", cfg, "{ [REDACTED]  %!s(int=0)   [REDACTED]   %!s(int=0) %!s(int=0) %!s(int=0) %!s(bool=false) [] 0s 0s %!s(float64=0) %!s(int=0) %!s(float64=0) %!s(int=0) %!s(int64=0) %!s(receiver.UpdateDeliveryPolicy=0) 0s %!s(func(int, string)=<nil>) %!s(uint32=0) 0s 0s 0s 0s 0s 0s 0s 0s}"},
		{"q", "%q", cfg, `{"" "[REDACTED]" "" '\x00' "" "" "[REDACTED]" "" "" '\x00' '\x00' '\x00' %!q(bool=false) [] "0s" "0s" %!q(float64=0) '\x00' %!q(float64=0) '\x00' '\x00' '\x00' "0s" %!q(func(int, string)=<nil>) '\x00' "0s" "0s" "0s" "0s" "0s" "0s" "0s" "0s"}`},
		{"d", "%d", cfg, "{%!d(receiver.Mode=) %!d(string=[REDACTED]) %!d(string=) 0 %!d(string=) %!d(string=) %!d(string=[REDACTED]) %!d(string=) %!d(string=) 0 0 0 %!d(bool=false) [] 0 0 %!d(float64=0) 0 %!d(float64=0) 0 0 0 0 0 0 0 0 0 0 0 0 0 0}"},
		{"x", "%x", cfg, "{ 5b52454441435445445d  0   5b52454441435445445d   0 0 0 %!x(bool=false) [] 3073 3073 0x0p+00 0 0x0p+00 0 0 0 3073 0 0 3073 3073 3073 3073 3073 3073 3073 3073}"},
		{"X", "%X", cfg, "{ 5B52454441435445445D  0   5B52454441435445445D   0 0 0 %!X(bool=false) [] 3073 3073 0X0P+00 0 0X0P+00 0 0 0 3073 0 0 3073 3073 3073 3073 3073 3073 3073 3073}"},
		{"width_prec", "%12.5v", cfg, "{                    [REDA                     00000                                  [REDA                                  00000        00000        00000        false []           0s           0s            0        00000            0        00000        00000        00000           0s        <nil>        00000           0s           0s           0s           0s           0s           0s           0s           0s}"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			assert.Equal(t, r.want, fmt.Sprintf(r.format, r.arg))
		})
	}
	t.Run("slog", func(t *testing.T) {
		const js = `{"level":"INFO","msg":"m","c":"!ERROR:json: unsupported type: func(int, string)"}` + "\n"
		text, gotJS := testutil.SlogLines("c", cfg)
		assert.Equal(t, `level=INFO msg=m c="`+b99PlusV+`"`+"\n", text)
		assert.Equal(t, js, gotJS, "the JSON handler cannot encode a Config (func field); unchanged")
		text, gotJS = testutil.SlogLines("c", &cfg)
		assert.Equal(t, `level=INFO msg=m c="&`+b99PlusV+`"`+"\n", text)
		assert.Equal(t, js, gotJS)
	})
}

// TestB99_SaveAndReload: a config saved with encoding/json holds [REDACTED], so after a reload the handler rejects
// the real secret and accepts the public text [REDACTED] as the header (PLAN §9, the upgrade note). With the field
// still a string, the reload would keep the secret. The channel shows that the refused update is not delivered
// (a 401 alone does not prove it).
func TestB99_SaveAndReload(t *testing.T) {
	saved, err := json.Marshal(map[string]any{"WebhookSecret": b99Config().WebhookSecret})
	require.NoError(t, err)
	assert.JSONEq(t, `{"WebhookSecret":"[REDACTED]"}`, string(saved))
	var reloaded receiver.Config
	require.NoError(t, json.Unmarshal(saved, &reloaded))
	assert.Equal(t, "[REDACTED]", string(reloaded.WebhookSecret))
	cfg := receiver.DefaultConfig()
	cfg.WebhookSecret = reloaded.WebhookSecret
	updates := make(chan tg.Update, 2)
	h := receiver.NewWebhookHandler(nil, updates, cfg)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"update_id":1}`))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", b99Secret)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "the real secret after the reload")
	assert.Equal(t, 0, len(updates), "the refused update is not delivered")
	forged := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"update_id":2}`))
	forged.Header.Set("X-Telegram-Bot-Api-Secret-Token", "[REDACTED]")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, forged)
	// A pin, not a property (no wrong version can fail only this line): the upgrade-note hazard. Since B-103,
	// LoadConfig and galigo.New refuse the secret [REDACTED] and NewWebhookHandler logs a warning for it, but the
	// handler still keeps and compares it, so this request is still accepted.
	assert.Equal(t, http.StatusOK, rec.Code, "pin of the upgrade-note hazard: a reloaded [REDACTED] secret accepts the header [REDACTED]")
	assert.Equal(t, 1, len(updates), "pin of the upgrade-note hazard: the forged update is delivered")
}

// b99Delivered is the number of updates a request with the status want delivers: one when it is accepted, none when
// it is refused.
func b99Delivered(want int) int {
	if want == http.StatusOK {
		return 1
	}
	return 0
}

// b99SetSecret sets cfg.WebhookSecret from a string variable by reflection (its kind is string before and after the
// change), so the file builds on the baseline.
func b99SetSecret(cfg *receiver.Config, secret string) {
	reflect.ValueOf(cfg).Elem().FieldByName("WebhookSecret").SetString(secret)
}

// TestB99_HandlerAuth: a handler built from a Config accepts only its secret and rejects every other header, for every
// value class; with an empty secret it accepts every request (AC4). A new handler per request. An accepted request
// delivers its update and a refused one delivers nothing.
func TestB99_HandlerAuth(t *testing.T) {
	serve := func(secret string, header *string) (int, int) {
		cfg := receiver.DefaultConfig()
		b99SetSecret(&cfg, secret)
		updates := make(chan tg.Update, 1)
		h := receiver.NewWebhookHandler(nil, updates, cfg)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"update_id":1}`))
		if header != nil {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", *header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, len(updates)
	}
	values := append(testutil.WebhookSecretValues(b99Secret), testutil.WebhookSecretOutside()...)
	for _, v := range append(values, testutil.WebhookSecretValue{Name: "empty"}) {
		t.Run(v.Name, func(t *testing.T) {
			for _, c := range testutil.WebhookAuthCases(v.Secret) {
				code, delivered := serve(v.Secret, c.Header)
				assert.Equal(t, c.Want, code, c.Name)
				assert.Equal(t, b99Delivered(c.Want), delivered, c.Name+": updates delivered")
			}
		})
	}
}

// TestB99_DocumentedLimit pins the limit the docs name (PLAN non-goal 4): a Config VALUE in an unexported field of the
// user's own struct is printed by reflection, so its secret (and token) are in the output. It passes before and after
// the change; a change to this path shows here.
func TestB99_DocumentedLimit(t *testing.T) {
	type app struct {
		name string
		cfg  receiver.Config
	}
	want := "{name:a cfg:{Mode: Token:" + b99Token + " BaseURL: WebhookPort:0 TLSCertPath: TLSKeyPath: WebhookSecret:" + b99Secret +
		" AllowedDomain: WebhookURL: PollingTimeout:0 PollingLimit:0 PollingMaxErrors:0 DeleteWebhookFirst:false AllowedUpdates:[] RetryInitialDelay:0 RetryMaxDelay:0 RetryBackoffFactor:0 UpdateBufferSize:0 RateLimitRequests:0 RateLimitBurst:0 MaxBodySize:0 UpdateDeliveryPolicy:0 UpdateDeliveryTimeout:0 OnUpdateDropped:<nil> BreakerMaxRequests:0 BreakerInterval:0 BreakerTimeout:0 ReadTimeout:0 ReadHeaderTimeout:0 WriteTimeout:0 IdleTimeout:0 DrainDelay:0 ShutdownTimeout:0}}"
	assert.Equal(t, want, fmt.Sprintf("%+v", app{"a", b99Config()}))
}
