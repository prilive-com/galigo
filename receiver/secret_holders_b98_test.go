package receiver_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

const (
	b98Token  = "123456789:B98-FAKE-token-receiver-AAAAAAAAAA"
	b98Secret = "B98-FAKE-webhook-secret-receiver"
)

func b98Config() receiver.Config {
	cfg := receiver.DefaultConfig()
	cfg.Token = tg.SecretToken(b98Token)
	cfg.WebhookSecret = b98Secret
	return cfg
}

func TestB98_PollingClient(t *testing.T) {
	const text = "receiver.PollingClient{[REDACTED]}"
	pc := receiver.NewPollingClient(tg.SecretToken(b98Token), make(chan tg.Update, 1), nil, b98Config())
	b98HolderRows(t, pc, &receiver.PollingClient{}, (*receiver.PollingClient)(nil), text, b98Token)
}

func TestB98_WebhookHandler(t *testing.T) {
	const text = "receiver.WebhookHandler{[REDACTED]}"
	h := receiver.NewWebhookHandler(nil, make(chan tg.Update, 1), b98Config())
	b98HolderRows(t, h, &receiver.WebhookHandler{}, (*receiver.WebhookHandler)(nil), text, b98Secret)
}

// b98HolderRows runs the rows both receiver types share, one per subtest: the matrix for a live value (secret is
// the one it holds), a zero value and a nil pointer; %T; an exported struct field; slog; and the L-d pin.
func b98HolderRows(t *testing.T, live, zero, nilp any, text, secret string) {
	t.Helper()
	t.Run("matrix_live", func(t *testing.T) { testutil.AssertFormatsAs(t, live, text, secret) })
	t.Run("matrix_zero", func(t *testing.T) { testutil.AssertFormatsAs(t, zero, text) })
	t.Run("matrix_nil", func(t *testing.T) { testutil.AssertFormatsAs(t, nilp, "<nil>") })
	t.Run("T", func(t *testing.T) {
		assert.Equal(t, "*"+strings.TrimSuffix(text, "{[REDACTED]}"), fmt.Sprintf("%T", live))
	})
	t.Run("struct_field_plus_v", func(t *testing.T) {
		assert.Equal(t, "{F:"+text+"}", fmt.Sprintf("%+v", struct{ F any }{live}))
	})
	t.Run("slog_text", func(t *testing.T) {
		slogText, _ := testutil.SlogLines("h", live)
		assert.Equal(t, "level=INFO msg=m h="+text+"\n", slogText)
	})
	t.Run("slog_json", func(t *testing.T) {
		_, js := testutil.SlogLines("h", live)
		assert.Equal(t, `{"level":"INFO","msg":"m","h":{}}`+"\n", js)
	})
	t.Run("L-d_text_template", func(t *testing.T) {
		var out bytes.Buffer
		require.NoError(t, template.Must(template.New("t").Parse("{{.}}")).Execute(&out, live))
		assert.Contains(t, out.String(), secret, "text/template dereferences the pointer")
	})
}
