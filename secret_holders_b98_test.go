package galigo_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

const (
	b98Token  = "123456789:B98-FAKE-token-bot-AAAAAAAAAAAAAAA"
	b98Secret = "B98-FAKE-webhook-secret-bot"
	b98Text   = "galigo.Bot{[REDACTED]}"
)

func b98Bot(t *testing.T) *galigo.Bot {
	t.Helper()
	b, err := galigo.New(b98Token, galigo.WithWebhook(8443, b98Secret))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// TestB98_BotMatrix: a webhook-mode bot and a polling-mode bot made by New (the matrix also checks that printing
// leaves their state in place: token, webhook secret, handler or receiver, updates channel), a zero-value bot and a
// nil pointer.
func TestB98_BotMatrix(t *testing.T) {
	t.Run("live", func(t *testing.T) { testutil.AssertFormatsAs(t, b98Bot(t), b98Text, b98Token, b98Secret) })
	t.Run("live_polling", func(t *testing.T) {
		b, err := galigo.New(b98Token) // polling mode; New makes no request
		require.NoError(t, err)
		t.Cleanup(func() { _ = b.Close() })
		testutil.AssertFormatsAs(t, b, b98Text, b98Token)
	})
	t.Run("zero", func(t *testing.T) { testutil.AssertFormatsAs(t, &galigo.Bot{}, b98Text) })
	t.Run("nil", func(t *testing.T) {
		var nilb *galigo.Bot
		testutil.AssertFormatsAs(t, nilb, "<nil>")
	})
}

func TestB98_BotRows(t *testing.T) {
	b := b98Bot(t)
	type pub struct{ B *galigo.Bot }
	cases := []struct{ name, got, want string }{
		{"plus_v", fmt.Sprintf("%+v", b), b98Text},
		{"sharp_v", fmt.Sprintf("%#v", b), b98Text},
		{"T", fmt.Sprintf("%T", b), "*galigo.Bot"},
		{"struct_field_plus_v", fmt.Sprintf("%+v", pub{b}), "{B:" + b98Text + "}"},
		{"struct_field_s", fmt.Sprintf("%s", any(pub{b})), "{" + b98Text + "}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, tc.got) })
	}
	t.Run("slog_text", func(t *testing.T) {
		text, _ := testutil.SlogLines("b", b)
		assert.Equal(t, "level=INFO msg=m b="+b98Text+"\n", text)
	})
	t.Run("slog_json", func(t *testing.T) {
		_, js := testutil.SlogLines("b", b)
		assert.Equal(t, `{"level":"INFO","msg":"m","b":{}}`+"\n", js)
	})
	t.Run("L-d_text_template", func(t *testing.T) {
		var out bytes.Buffer
		require.NoError(t, template.Must(template.New("t").Parse("{{.}}")).Execute(&out, b))
		assert.Contains(t, out.String(), b98Token, "text/template dereferences the pointer (token)")
		assert.Contains(t, out.String(), b98Secret, "and the webhook secret")
	})
}

// b98App embeds a holder and has no Format or String member of its own (PLAN §9 cell 0).
type b98App struct {
	*galigo.Bot
	Name string
}

func TestB98_EmbeddingTypePrintsOnlyHolder(t *testing.T) {
	a := b98App{b98Bot(t), "myapp"}
	t.Run("cell0_plus_v", func(t *testing.T) {
		assert.Equal(t, b98Text, fmt.Sprintf("%+v", a), "the promoted Format prints only the holder's text")
	})
	t.Run("cell0_slog_text", func(t *testing.T) {
		text, _ := testutil.SlogLines("app", a)
		assert.Equal(t, "level=INFO msg=m app="+b98Text+"\n", text)
	})
	t.Run("cell0_by_value", func(t *testing.T) {
		// A holder embedded by value: the pointer to the outer value gets the promoted Format.
		assert.Equal(t, "sender.Client{[REDACTED]}", fmt.Sprintf("%+v", &struct {
			sender.Client
			Name string
		}{Name: "n"}))
	})
	t.Run("cell0_nil_holder", func(t *testing.T) {
		// The embedded pointer is nil, so the promoted Format prints <nil> for the whole value.
		assert.Equal(t, "<nil>", fmt.Sprintf("%+v", b98App{nil, "myapp"}))
	})
}

// b98TwoHolders embeds two galigo holders at the same depth (PLAN §9 cell 9).
type b98TwoHolders struct {
	*sender.Client
	*receiver.PollingClient
}

func TestB98_TwoHoldersEmbedded(t *testing.T) {
	c, err := sender.New(b98Token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	pc := receiver.NewPollingClient(tg.SecretToken(b98Token), make(chan tg.Update, 1),
		slog.New(slog.DiscardHandler), receiver.DefaultConfig())
	both := b98TwoHolders{c, pc}
	t.Run("cell9_not_formatter", func(t *testing.T) {
		_, isFormatter := any(both).(fmt.Formatter)
		assert.False(t, isFormatter, "the two promoted Format methods cancel")
	})
	t.Run("cell9_fields", func(t *testing.T) {
		assert.Equal(t, "{Client:sender.Client{[REDACTED]} PollingClient:receiver.PollingClient{[REDACTED]}}",
			fmt.Sprintf("%+v", both))
	})
}
