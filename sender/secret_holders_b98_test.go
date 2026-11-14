package sender_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"reflect"
	"regexp"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
)

// b98Token is a fake token in the bot token shape; it is never sent anywhere.
const b98Token = "123456789:B98-FAKE-token-sender-AAAAAAAAAAAA"

const b98Text = "sender.Client{[REDACTED]}"

func b98Client(t *testing.T) *sender.Client {
	t.Helper()
	c, err := sender.New(b98Token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestB98_ClientMatrix: a client made by New (the matrix also checks that printing leaves its token in place), a
// zero-value client (a non-nil pointer whose fields are empty is not a nil pointer), and a nil pointer.
func TestB98_ClientMatrix(t *testing.T) {
	t.Run("live", func(t *testing.T) { testutil.AssertFormatsAs(t, b98Client(t), b98Text, b98Token) })
	t.Run("zero", func(t *testing.T) { testutil.AssertFormatsAs(t, &sender.Client{}, b98Text) })
	t.Run("nil", func(t *testing.T) {
		var c *sender.Client
		testutil.AssertFormatsAs(t, c, "<nil>")
	})
}

// TestB98_ClientRows passes some values as any(...): on the baseline go vet rejects %s, %q, %x and %d of a type that
// is not a fmt.Formatter, which would make the red a build failure instead of a value failure.
func TestB98_ClientRows(t *testing.T) {
	c := b98Client(t)
	var nilc *sender.Client
	type pub struct{ C *sender.Client }
	cases := []struct{ name, got, want string }{
		{"v", fmt.Sprintf("%v", c), b98Text},
		{"plus_v", fmt.Sprintf("%+v", c), b98Text},
		{"sharp_v", fmt.Sprintf("%#v", c), b98Text},
		{"sharp_v_width", fmt.Sprintf("%#28v", c), "   " + b98Text},
		{"s", fmt.Sprintf("%s", any(c)), b98Text},
		{"q", fmt.Sprintf("%q", any(c)), `"` + b98Text + `"`},
		{"x", fmt.Sprintf("%x", any(c)), fmt.Sprintf("%x", b98Text)},
		{"d", fmt.Sprintf("%d", any(c)), "%!d(string=" + b98Text + ")"},
		{"width_prec", fmt.Sprintf("%10.6v", c), "    sender"},
		{"star_width", fmt.Sprintf("%-*v|", 27, c), b98Text + "  |"},
		{"star_prec", fmt.Sprintf("%.*s", 6, any(c)), "sender"},
		{"arg_index", fmt.Sprintf("%[2]v %[1]d", 7, c), b98Text + " 7"},
		{"sprint", fmt.Sprint(c), b98Text},
		{"sprintln", fmt.Sprintln(c, 1), b98Text + " 1\n"},
		{"append", string(fmt.Appendf(nil, "%v", c)), b98Text},
		{"nil_v", fmt.Sprintf("%v", nilc), "<nil>"},
		{"nil_sharp_v", fmt.Sprintf("%#v", nilc), "<nil>"},
		{"nil_d", fmt.Sprintf("%d", any(nilc)), "%!d(string=<nil>)"},
		{"T", fmt.Sprintf("%T", c), "*sender.Client"},
		{"struct_field_v", fmt.Sprintf("%v", pub{c}), "{" + b98Text + "}"},
		{"struct_field_plus_v", fmt.Sprintf("%+v", pub{c}), "{C:" + b98Text + "}"},
		{"struct_field_s", fmt.Sprintf("%s", any(pub{c})), "{" + b98Text + "}"},
		{"struct_field_q", fmt.Sprintf("%q", any(pub{c})), `{"` + b98Text + `"}`},
		{"slice_s", fmt.Sprintf("%s", any([]*sender.Client{c, nil})), "[" + b98Text + " <nil>]"},
		{"array_v", fmt.Sprintf("%v", [1]*sender.Client{c}), "[" + b98Text + "]"},
		{"map_value_v", fmt.Sprintf("%v", map[string]*sender.Client{"a": c}), "map[a:" + b98Text + "]"},
		{"interface_slice_v", fmt.Sprintf("%v", []any{c}), "[" + b98Text + "]"},
		{"pointer_to_pointer_v", fmt.Sprintf("%v", &c)[:2], "0x"},
		// An argument index is not passed on by fmt.FormatString (PLAN AC1 ii); a variable format, because vet
		// rejects this directive in a constant one.
		{"arg_index_flag_verb", fmt.Sprintf(b98IndexFlag, c), "%!(NOVERB)%!(EXTRA string=" + b98Text + ")"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got)
		})
	}
	t.Run("p", func(t *testing.T) {
		assert.Regexp(t, regexp.MustCompile(`^0x[0-9a-f]+$`), fmt.Sprintf("%p", c))
	})
}

func TestB98_ClientSlogJSON(t *testing.T) {
	c := b98Client(t)
	var nilc *sender.Client
	t.Run("slog_text", func(t *testing.T) {
		text, _ := testutil.SlogLines("c", c)
		assert.Equal(t, "level=INFO msg=m c="+b98Text+"\n", text)
	})
	// Controls (the same on the baseline): JSON is {} for a non-nil pointer and null for a nil one (round p3 O-P3-03).
	t.Run("slog_json", func(t *testing.T) {
		_, js := testutil.SlogLines("c", c)
		assert.Equal(t, `{"level":"INFO","msg":"m","c":{}}`+"\n", js)
	})
	t.Run("json", func(t *testing.T) {
		b, err := json.Marshal(c) //nolint:staticcheck // SA9005 (no exported field) is the point: the row pins {}
		require.NoError(t, err)
		assert.Equal(t, "{}", string(b))
	})
	t.Run("nil_slog_text", func(t *testing.T) {
		text, _ := testutil.SlogLines("c", nilc)
		assert.Equal(t, "level=INFO msg=m c=<nil>\n", text)
	})
	t.Run("nil_slog_json", func(t *testing.T) {
		_, js := testutil.SlogLines("c", nilc)
		assert.Equal(t, `{"level":"INFO","msg":"m","c":null}`+"\n", js)
	})
	t.Run("nil_json", func(t *testing.T) {
		b, err := json.Marshal(nilc) //nolint:staticcheck // SA9005 is the point: the row pins null
		require.NoError(t, err)
		assert.Equal(t, "null", string(b))
	})
}

// b98IndexFlag is a directive whose verb position, after an argument index, holds a flag character.
var b98IndexFlag = "%[1]+"

// b98Custom has its own Format, for the embedding rows.
type b98Custom struct{}

func (b98Custom) Format(f fmt.State, _ rune) { fmt.Fprint(f, "CUSTOM") }

type b98OwnString struct {
	*sender.Client
	Name string
}

func (o b98OwnString) String() string { return "OWN-" + o.Name }

type b98SameDepth struct {
	*sender.Client
	b98Custom
}

type b98Mid struct{ b98Custom }

type b98OwnError struct {
	*sender.Client
	msg string
}

func (e b98OwnError) Error() string { return "myerr:" + e.msg }

type b98OwnGoString struct{ *sender.Client }

func (b98OwnGoString) GoString() string { return "OWN-GOSTRING" }

type b98Deeper struct {
	*sender.Client
	b98Mid
}

// TestB98_ClientEmbedding pins the behaviour changes for types that embed *sender.Client (PLAN §9 cells 1, 1b, 1c,
// 3, 5), one row per subtest; the control rows show that the cancelled or hidden Format exists.
func TestB98_ClientEmbedding(t *testing.T) {
	c := b98Client(t)
	same := b98SameDepth{Client: c}
	e := b98OwnError{c, "boom"}
	t.Run("cell1_own_string", func(t *testing.T) {
		assert.Equal(t, b98Text, fmt.Sprintf("%v", b98OwnString{c, "x"}), "the promoted Format wins over the outer String")
	})
	t.Run("cell3_not_formatter", func(t *testing.T) {
		_, isFormatter := any(same).(fmt.Formatter)
		assert.False(t, isFormatter, "two Format methods at the same depth cancel")
	})
	t.Run("cell3_fields", func(t *testing.T) {
		assert.Equal(t, "{"+b98Text+" {}}", fmt.Sprintf("%v", same))
	})
	t.Run("cell3_control", func(t *testing.T) {
		assert.Equal(t, "CUSTOM", fmt.Sprintf("%v", same.b98Custom), "the cancelled Format exists")
	})
	t.Run("cell5_deeper", func(t *testing.T) {
		assert.Equal(t, b98Text, fmt.Sprintf("%v", b98Deeper{Client: c}), "a deeper Format is hidden")
	})
	t.Run("cell5_control", func(t *testing.T) {
		assert.Equal(t, "CUSTOM", fmt.Sprintf("%v", b98Deeper{}.b98Mid), "the hidden Format exists")
	})
	t.Run("cell1b_own_error", func(t *testing.T) {
		assert.Equal(t, b98Text, fmt.Sprintf("%v", e), "the promoted Format wins over the outer Error")
	})
	t.Run("cell1b_wrap", func(t *testing.T) {
		assert.Equal(t, "wrap: "+b98Text, fmt.Errorf("wrap: %w", e).Error(), "so the wrapped message changes")
	})
	t.Run("cell1b_error_unchanged", func(t *testing.T) {
		assert.Equal(t, "myerr:boom", e.Error(), "Error itself is unchanged")
	})
	t.Run("cell1c_own_gostring", func(t *testing.T) {
		assert.Equal(t, b98Text, fmt.Sprintf("%#v", b98OwnGoString{c}), "the promoted Format wins over the outer GoString")
	})
}

// TestB98_ClientDocumentedLimits pins the paths that still print the token (PLAN §3 L-a to L-d, the tg.SecretToken
// doc), one per subtest. They print it on the baseline too; only the L-d_printf_control row changes.
func TestB98_ClientDocumentedLimits(t *testing.T) {
	c := b98Client(t)
	w := "x %w" // a variable format: vet rejects these directives in a constant one
	type priv struct{ c *sender.Client }
	t.Run("L-a_sprintf", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(w, c), b98Token, "%w outside fmt.Errorf")
	})
	t.Run("L-a_errorf", func(t *testing.T) {
		assert.Contains(t, fmt.Errorf(w, c).Error(), b98Token, "%w of a value that is not an error")
	})
	t.Run("L-b_plus_v_address", func(t *testing.T) {
		assert.Regexp(t, regexp.MustCompile(`^\{c:0x[0-9a-f]+\}$`), fmt.Sprintf("%+v", priv{c}))
	})
	t.Run("L-b_s_bad_verb", func(t *testing.T) {
		out := fmt.Sprintf("%s", any(priv{c}))
		assert.Contains(t, out, "%!s(*sender.Client=&{")
		assert.Contains(t, out, b98Token)
	})
	t.Run("L-c_copied_value", func(t *testing.T) {
		// A copy of the struct, made without a statement that go vet's copylocks check reports.
		copied := reflect.ValueOf(c).Elem().Interface()
		assert.Contains(t, fmt.Sprintf("%v", copied), b98Token)
	})
	t.Run("L-d_text_template", func(t *testing.T) {
		var out bytes.Buffer
		require.NoError(t, template.Must(template.New("t").Parse("{{.}}")).Execute(&out, c))
		assert.Contains(t, out.String(), b98Token, "text/template dereferences the pointer")
	})
	t.Run("L-d_html_template", func(t *testing.T) {
		var out bytes.Buffer
		require.NoError(t, htmltemplate.Must(htmltemplate.New("t").Parse("{{.}}")).Execute(&out, c))
		assert.Contains(t, out.String(), b98Token, "html/template prints the struct value too")
	})
	t.Run("L-d_printf_control", func(t *testing.T) {
		var out bytes.Buffer
		require.NoError(t, template.Must(template.New("t").Parse(`{{printf "%v" .}}`)).Execute(&out, c))
		assert.Equal(t, b98Text, out.String(), "printf receives the pointer")
	})
}
