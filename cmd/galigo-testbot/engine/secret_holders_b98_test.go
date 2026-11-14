package engine

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

func TestB98_SenderAdapter(t *testing.T) {
	const token = "123456789:B98-FAKE-token-adapter-AAAAAAAAAAA"
	const text = "engine.SenderAdapter{[REDACTED]}"
	c, err := sender.New(token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	a := NewSenderAdapter(c).WithToken(tg.SecretToken(token))
	var nila *SenderAdapter
	// The matrix also checks that printing leaves the token in place. A zero value and an adapter without a client
	// are not a nil pointer: they print the same text.
	t.Run("matrix_pointer", func(t *testing.T) { testutil.AssertFormatsAs(t, a, text, token) })
	t.Run("matrix_value", func(t *testing.T) { testutil.AssertFormatsAs(t, *a, text, token) })
	t.Run("matrix_zero_value", func(t *testing.T) { testutil.AssertFormatsAs(t, SenderAdapter{}, text) })
	t.Run("matrix_zero_pointer", func(t *testing.T) { testutil.AssertFormatsAs(t, &SenderAdapter{}, text) })
	t.Run("matrix_nil_client", func(t *testing.T) {
		testutil.AssertFormatsAs(t, NewSenderAdapter(nil).WithToken(tg.SecretToken(token)), text, token)
	})
	// PLAN AC1 i: for a nil pointer the value method panics in the pointer wrapper, and fmt writes a bare <nil>
	// without flags, width or precision.
	cases := []struct{ name, got, want string }{
		{"nil_v", fmt.Sprintf("%v", nila), "<nil>"},
		{"nil_d", fmt.Sprintf("%d", any(nila)), "<nil>"},
		{"nil_q", fmt.Sprintf("%q", any(nila)), "<nil>"},
		{"nil_width_prec", fmt.Sprintf("%20.3v", nila), "<nil>"},
		{"T", fmt.Sprintf("%T", a), "*engine.SenderAdapter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, tc.got) })
	}
	for _, v := range []struct {
		name string
		v    any
	}{{"pointer", a}, {"value", *a}} {
		t.Run("slog_text_"+v.name, func(t *testing.T) {
			slogText, _ := testutil.SlogLines("a", v.v)
			assert.Equal(t, "level=INFO msg=m a="+text+"\n", slogText)
		})
		// Controls (the same on the baseline): no exported field, so JSON is {}.
		t.Run("slog_json_"+v.name, func(t *testing.T) {
			_, js := testutil.SlogLines("a", v.v)
			assert.Equal(t, `{"level":"INFO","msg":"m","a":{}}`+"\n", js)
		})
		t.Run("json_"+v.name, func(t *testing.T) {
			b, err := json.Marshal(v.v)
			require.NoError(t, err)
			assert.Equal(t, "{}", string(b))
		})
	}
	t.Run("nil_slog_text", func(t *testing.T) {
		slogText, _ := testutil.SlogLines("a", nila)
		assert.Equal(t, "level=INFO msg=m a=<nil>\n", slogText)
	})
	t.Run("nil_json", func(t *testing.T) {
		b, err := json.Marshal(nila) //nolint:staticcheck // SA9005 is the point: the row pins null
		require.NoError(t, err)
		assert.Equal(t, "null", string(b))
	})
}
