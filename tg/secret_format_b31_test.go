package tg_test

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// b31Secret is a fake token value used by the B-31 tests.
const b31Secret = "123456789:B31-FAKE-secret"

// b31Before has the method set SecretToken had before B-31 (String, GoString). For the verbs where fmt used those
// methods, its output is the output SecretToken must still give.
type b31Before string

func (b31Before) String() string   { return "[REDACTED]" }
func (b31Before) GoString() string { return `tg.SecretToken("[REDACTED]")` }

// b31Flags returns the 32 subsets of the five fmt flags, each in one fixed order.
func b31Flags() []string {
	const all = "+-# 0"
	var out []string
	for mask := range 1 << len(all) {
		var b strings.Builder
		for i := range len(all) {
			if mask&(1<<i) != 0 {
				b.WriteByte(all[i])
			}
		}
		out = append(out, b.String())
	}
	return out
}

// b31Verbs: every ASCII rune that fmt does not read as directive syntax (so fmt reads it as the verb), except p
// and w, plus a rune below 256 and a rune above 255 that are not ASCII. %p and %w are limits
// (TestB31_DocumentedLimits). The string verbs are v s q x X; %T prints the type; every other rune is a verb a
// string does not fit, one fmt knows (such as d, O, U) or one it does not know (such as k, z, !, a control rune).
var b31Verbs = func() []rune {
	var out []rune
	for r := range rune(0x80) {
		if !strings.ContainsRune("%#+- 0123456789.*[pw", r) {
			out = append(out, r)
		}
	}
	return append(out, 'é', '界')
}()

// b31Widths: none, 5 (shorter than "[REDACTED]" and longer than its first 3 bytes), 14, and from an argument (*):
// -13 (a negative width sets the '-' flag) and 0.
var b31Widths = []struct {
	text string
	arg  any // the * argument, or nil
}{{"", nil}, {"5", nil}, {"14", nil}, {"*", -13}, {"*", 0}}

// b31Precisions: none, a bare '.', 0, 3, and from an argument (*): 2, 0 and -1 (for a negative precision from an
// argument, fmt writes %!(BADPREC) and then formats with no precision).
var b31Precisions = []struct {
	text string
	arg  any // the * argument, or nil
}{{"", nil}, {".", nil}, {".0", nil}, {".3", nil}, {".*", 2}, {".*", 0}, {".*", -1}}

// TestB31_FormatMatrix: every directive gives an exact expected output, for the fake token and for an empty token.
// The string verbs (v s q x X, %#v) keep the output the String/GoString methods gave before B-31; every other verb
// formats "[REDACTED]" (no form of the token). Width and precision come from the directive or from arguments (*), so
// the arguments are built per directive. Only the first 20 wrong outputs are shown; the last assertion counts all.
func TestB31_FormatMatrix(t *testing.T) {
	n, bad := 0, 0
	for _, tc := range []struct {
		name string
		tok  tg.SecretToken
	}{{"fake", b31Secret}, {"empty", ""}} {
		for _, flags := range b31Flags() {
			for _, width := range b31Widths {
				for _, prec := range b31Precisions {
					for _, verb := range b31Verbs {
						d := "%" + flags + width.text + prec.text + string(verb)
						var pre []any
						if width.arg != nil {
							pre = append(pre, width.arg)
						}
						if prec.arg != nil {
							pre = append(pre, prec.arg)
						}
						args := func(v any) []any { return append(append([]any{}, pre...), v) }
						var want string
						switch {
						case verb == 'T':
							want = fmt.Sprintf(d, args(tg.SecretToken(""))...) // the type name only: the same for any value
						case strings.ContainsRune("vsqxX", verb):
							want = fmt.Sprintf(d, args(b31Before(b31Secret))...)
						default:
							want = fmt.Sprintf(d, args("[REDACTED]")...)
						}
						if got := fmt.Sprintf(d, args(tc.tok)...); got != want {
							bad++
							if bad <= 20 {
								assert.Equal(t, want, got, "%q (%s token)", d, tc.name)
							}
						}
						n++
					}
				}
			}
		}
	}
	assert.Zero(t, bad, "directives with a wrong output (the first 20 are shown above)")
	require.Equal(t, 2*32*len(b31Widths)*len(b31Precisions)*len(b31Verbs), n)
}

// TestB31_IndexedDirectives: an explicit argument index. With a letter verb, or with a parser character that the
// replayed directive still reads as the verb, the output equals the same directive on the string "[REDACTED]". An
// index just before the verb is not passed on to the directive Format replays, so a verb that is directive syntax
// there (a flag character with no width or precision before the index, a digit after a width or precision, '[')
// prints fmt's %!(NOVERB) text; it is pinned here and holds no form of the token (PLAN AC3, D4).
func TestB31_IndexedDirectives(t *testing.T) {
	tok := tg.SecretToken(b31Secret)
	for _, r := range []struct {
		d   string
		arg int // the second argument: a width or precision for the directives that read one
	}{
		{"%[1]d", 9}, {"%[1]v", 9}, {"%[1]q", 9}, {"%[2]*[1]d", 9}, {"%#[1]v", 9}, {"%5[1]#", 9}, {"%[1].", 9},
		{"%.[2]*[1]d", 0}, {"%.[2]*[1]s", 0}, {"%.[2]*[1]q", 0}, {"%#.[2]*[1]v", 0}, // precision 0 from an indexed argument
	} {
		want := fmt.Sprintf(r.d, "[REDACTED]", r.arg)
		if strings.HasSuffix(r.d, "v") || strings.HasSuffix(r.d, "q") || strings.HasSuffix(r.d, "s") {
			want = fmt.Sprintf(r.d, b31Before(b31Secret), r.arg)
		}
		assert.Equal(t, want, fmt.Sprintf(r.d, tok, r.arg), r.d)
	}
	for _, r := range []struct{ d, want string }{
		{"%[1]#", "%!(NOVERB)%!(EXTRA string=[REDACTED])"},
		{"%[1]+", "%!(NOVERB)%!(EXTRA string=[REDACTED])"},
		{"%[1]-", "%!(NOVERB)%!(EXTRA string=[REDACTED])"},
		{"%[1] ", "%!(NOVERB)%!(EXTRA string=[REDACTED])"},
		{"%[1][", "%!(NOVERB)"},
		{"%1[1]0", "%!(NOVERB)%!(EXTRA string=[REDACTED])"},
		{"%.1[1]2", "%!(NOVERB)%!(EXTRA string=[REDACTED])"},
	} {
		assert.Equal(t, r.want, fmt.Sprintf(r.d, tok), r.d)
	}
}

type b31Exported struct{ Token tg.SecretToken }

// b31Creds and b31AuthErr embed SecretToken, so they get its Format, which fmt uses before their own methods.
type b31Creds struct {
	tg.SecretToken
	User string
}

func (c b31Creds) String() string { return "user=" + c.User }

type b31AuthErr struct {
	tg.SecretToken
	Msg string
}

func (e b31AuthErr) Error() string { return "auth failed: " + e.Msg }

// b31Custom has its own Format; b31Conflict embeds it and SecretToken at the same depth, so the two Format methods
// cancel and b31Conflict has none (it had b31Custom's before B-31). b31Deeper has b31Custom one level deeper than
// SecretToken, so the promoted SecretToken.Format hides it (b31Deeper printed CUSTOM before B-31).
type b31Custom struct{}

func (b31Custom) Format(f fmt.State, _ rune) { fmt.Fprint(f, "CUSTOM") }

type b31Conflict struct {
	tg.SecretToken
	b31Custom
}

type b31Mid struct{ b31Custom }

type b31Deeper struct {
	tg.SecretToken
	b31Mid
}

type b31EmbedPtr struct {
	*tg.SecretToken
	User string
}

func (e b31EmbedPtr) String() string { return "user=" + e.User }

type b31EmbedGo struct{ tg.SecretToken }

func (b31EmbedGo) GoString() string { return "EMBGO" }

// TestB31_Containers: the token in an exported field, a pointer, a slice, a map, an error, slog, and an embedding type.
func TestB31_Containers(t *testing.T) {
	tok := tg.SecretToken(b31Secret)
	var nilTok *tg.SecretToken
	d, tv := "%d", "%t" // in variables: the format is the point, not what go vet thinks of it
	ae := b31AuthErr{tok, "bad"}
	wrapped := fmt.Errorf("wrap: %w", ae)
	rows := []struct{ name, got, want string }{
		{"field_d", fmt.Sprintf(d, b31Exported{tok}), "{%!d(string=[REDACTED])}"},
		{"field_v", fmt.Sprintf("%v", b31Exported{tok}), "{[REDACTED]}"},
		{"field_plus_v", fmt.Sprintf("%+v", b31Exported{tok}), "{Token:[REDACTED]}"},
		{"field_sharp_v", fmt.Sprintf("%#v", b31Exported{tok}), `tg_test.b31Exported{Token:tg.SecretToken("[REDACTED]")}`},
		{"pointer_d", fmt.Sprintf(d, &b31Exported{tok}), "&{%!d(string=[REDACTED])}"},
		{"token_pointer_d", fmt.Sprintf(d, &tok), "%!d(string=[REDACTED])"},
		{"token_pointer_v", fmt.Sprintf("%v", &tok), "[REDACTED]"},
		{"nil_token_pointer_d", fmt.Sprintf(d, nilTok), "<nil>"},
		{"nil_token_pointer_v", fmt.Sprintf("%v", nilTok), "<nil>"},
		{"nil_token_pointer_t", fmt.Sprintf(tv, nilTok), "<nil>"},
		{"slice_d", fmt.Sprintf(d, []tg.SecretToken{tok, tok}), "[%!d(string=[REDACTED]) %!d(string=[REDACTED])]"},
		{"map_d", fmt.Sprintf(d, map[string]tg.SecretToken{"k": tok}), "map[%!d(string=k):%!d(string=[REDACTED])]"}, //nolint:staticcheck // SA5009: the wrong verb for the key is the point of the row
		{"map_key_d", fmt.Sprintf(d, map[tg.SecretToken]int{tok: 1}), "map[%!d(string=[REDACTED]):1]"},
		{"errorf_d", fmt.Errorf("token "+d, tok).Error(), "token %!d(string=[REDACTED])"},
		{"println", fmt.Sprintln(tok, b31Exported{tok}), "[REDACTED] {[REDACTED]}\n"},
		{"embed_string_v", fmt.Sprintf("%v", b31Creds{tok, "bob"}), "[REDACTED]"},
		{"embed_error_v", fmt.Sprintf("%v", ae), "[REDACTED]"},
		{"embed_error_wrap", wrapped.Error(), "wrap: [REDACTED]"},
		{"embed_error_method", ae.Error(), "auth failed: bad"},
		{"embed_pointer_v", fmt.Sprintf("%v", b31EmbedPtr{&tok, "bob"}), "[REDACTED]"},
		{"embed_nil_pointer_v", fmt.Sprintf("%v", b31EmbedPtr{nil, "bob"}), "%!v(PANIC=Format method: runtime error: invalid memory address or nil pointer dereference)"},
		{"embed_gostring", fmt.Sprintf("%#v", b31EmbedGo{tok}), `tg.SecretToken("[REDACTED]")`},
		{"embed_deeper_format", fmt.Sprintf("%v", b31Deeper{tok, b31Mid{}}), "[REDACTED]"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) { assert.Equal(t, r.want, r.got) })
	}
	t.Run("embed_error_is", func(t *testing.T) { assert.True(t, errors.Is(wrapped, ae)) })
	t.Run("embed_conflict", func(t *testing.T) {
		var v any = b31Conflict{tok, b31Custom{}}
		_, isFormatter := v.(fmt.Formatter)
		assert.False(t, isFormatter)
		assert.Equal(t, "[REDACTED]", fmt.Sprintf("%v", v))
	})
	t.Run("xml_element_attr", func(t *testing.T) {
		b, err := xml.Marshal(b31XMLElem{tok, tok})
		require.NoError(t, err)
		assert.Equal(t, `<b31XMLElem a="[REDACTED]"><Token>[REDACTED]</Token></b31XMLElem>`, string(b))
	})
	t.Run("xml_chardata_cdata", func(t *testing.T) {
		b, err := xml.Marshal(b31XMLCharData{tok, tok})
		require.NoError(t, err)
		assert.Equal(t, "<b31XMLCharData>[REDACTED]<![CDATA[[REDACTED]]]></b31XMLCharData>", string(b))
	})
	t.Run("slog_text", func(t *testing.T) {
		var b bytes.Buffer
		slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{ReplaceAttr: b31NoTime})).Info("m", "tok", tok, "field", b31Exported{tok})
		assert.Equal(t, "level=INFO msg=m tok=[REDACTED] field={Token:[REDACTED]}\n", b.String())
	})
	t.Run("slog_json", func(t *testing.T) {
		var b bytes.Buffer
		slog.New(slog.NewJSONHandler(&b, &slog.HandlerOptions{ReplaceAttr: b31NoTime})).Info("m", "tok", tok, "field", b31Exported{tok})
		assert.Equal(t, `{"level":"INFO","msg":"m","tok":"[REDACTED]","field":{"Token":"[REDACTED]"}}`+"\n", b.String())
	})
}

func b31NoTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}

type b31Unexported struct{ token tg.SecretToken }

type b31HasPtr struct{ P *b31Exported }

// TestB31_DocumentedLimits pins the paths the SecretToken doc names as limits, with their exact output: a value
// reached through an unexported field; fmt's bad-verb text (%w of a non-error value, and of an error outside
// fmt.Errorf; %p; a pointer below the top level with a verb a pointer does not support: in a struct field, a slice,
// an array, a map element and an interface value); encoder paths that read the string itself (a JSON map key, an
// XML comment field, an XML chardata or cdata field of type **SecretToken, gob). The doc's Go-version-dependent
// paths (a *SecretToken JSON map key, an XML field of interface type) have no row: their output differs between the
// CI toolchains. If a Go release changes one of the pinned paths, this fails and the doc comment must change with
// it.
func TestB31_DocumentedLimits(t *testing.T) {
	tok := tg.SecretToken(b31Secret)
	w, p, s := "x %w", "%p", "%s" // in variables: go vet rejects %w for this argument in a constant format
	t.Run("unexported_field", func(t *testing.T) {
		assert.Equal(t, "{"+b31Secret+"}", fmt.Sprintf("%v", b31Unexported{tok}))
	})
	t.Run("unexported_then_exported", func(t *testing.T) {
		assert.Equal(t, "{{"+b31Secret+"}}", fmt.Sprintf("%v", struct{ in b31Exported }{b31Exported{tok}}))
	})
	t.Run("slog_text_unexported", func(t *testing.T) {
		var b bytes.Buffer
		slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{ReplaceAttr: b31NoTime})).Info("m", "priv", b31Unexported{tok})
		assert.Equal(t, "level=INFO msg=m priv={token:"+b31Secret+"}\n", b.String())
	})
	t.Run("w_token", func(t *testing.T) {
		err := fmt.Errorf(w, tok)
		assert.Equal(t, "x %!w(tg.SecretToken="+b31Secret+")", err.Error())
		assert.Nil(t, errors.Unwrap(err))
	})
	t.Run("w_sprintf_error", func(t *testing.T) {
		assert.Equal(t, "x %!w(tg_test.b31AuthErr={"+b31Secret+" bad})", fmt.Sprintf(w, b31AuthErr{tok, "bad"}))
	})
	t.Run("w_struct", func(t *testing.T) {
		assert.Equal(t, "x %!w(tg_test.b31Exported={"+b31Secret+"})", fmt.Errorf(w, b31Exported{tok}).Error())
	})
	t.Run("p_token", func(t *testing.T) {
		assert.Equal(t, "%!p(tg.SecretToken="+b31Secret+")", fmt.Sprintf(p, tok))
	})
	t.Run("p_struct", func(t *testing.T) {
		assert.Equal(t, "%!p(tg_test.b31Exported={"+b31Secret+"})", fmt.Sprintf(p, b31Exported{tok}))
	})
	t.Run("s_pointer_field", func(t *testing.T) {
		assert.Equal(t, "{%!s(*tg_test.b31Exported=&{"+b31Secret+"})}", fmt.Sprintf(s, b31HasPtr{&b31Exported{tok}}))
	})
	t.Run("s_pointer_slice", func(t *testing.T) {
		assert.Equal(t, "[%!s(*tg_test.b31Exported=&{"+b31Secret+"})]", fmt.Sprintf(s, []*b31Exported{{tok}}))
	})
	t.Run("s_pointer_array", func(t *testing.T) {
		assert.Equal(t, "[%!s(*tg_test.b31Exported=&{"+b31Secret+"})]", fmt.Sprintf(s, [1]*b31Exported{{tok}}))
	})
	t.Run("s_pointer_map", func(t *testing.T) {
		assert.Equal(t, "map[k:%!s(*tg_test.b31Exported=&{"+b31Secret+"})]", fmt.Sprintf(s, map[string]*b31Exported{"k": {tok}}))
	})
	t.Run("s_pointer_iface", func(t *testing.T) {
		assert.Equal(t, "{%!s(*tg_test.b31Exported=&{"+b31Secret+"})}", fmt.Sprintf(s, struct{ A any }{&b31Exported{tok}}))
	})
	t.Run("json_map_key", func(t *testing.T) {
		b, err := json.Marshal(map[tg.SecretToken]string{tok: "x"})
		require.NoError(t, err)
		assert.Equal(t, `{"`+b31Secret+`":"x"}`, string(b))
	})
	t.Run("slog_json_map_key", func(t *testing.T) {
		var b bytes.Buffer
		slog.New(slog.NewJSONHandler(&b, &slog.HandlerOptions{ReplaceAttr: b31NoTime})).Info("m", "m", map[tg.SecretToken]string{tok: "x"})
		assert.Equal(t, `{"level":"INFO","msg":"m","m":{"`+b31Secret+`":"x"}}`+"\n", b.String())
	})
	t.Run("xml_comment", func(t *testing.T) {
		b, err := xml.Marshal(b31XMLComment{tok})
		require.NoError(t, err)
		assert.Equal(t, "<b31XMLComment><!--"+b31Secret+"--></b31XMLComment>", string(b))
	})
	t.Run("xml_chardata_cdata_ptrptr", func(t *testing.T) {
		pt := &tok
		b, err := xml.Marshal(b31XMLPtrPtr{&pt, &pt})
		require.NoError(t, err)
		assert.Equal(t, "<b31XMLPtrPtr>"+b31Secret+"<![CDATA["+b31Secret+"]]></b31XMLPtrPtr>", string(b))
	})
	t.Run("gob", func(t *testing.T) {
		var b bytes.Buffer
		require.NoError(t, gob.NewEncoder(&b).Encode(tok))
		var back string
		require.NoError(t, gob.NewDecoder(&b).Decode(&back))
		assert.Equal(t, b31Secret, back)
	})
}

type b31XMLElem struct {
	Token tg.SecretToken
	A     tg.SecretToken `xml:"a,attr"`
}

type b31XMLCharData struct {
	C  tg.SecretToken `xml:",chardata"`
	CD tg.SecretToken `xml:",cdata"`
}

type b31XMLComment struct {
	Token tg.SecretToken `xml:",comment"`
}

type b31XMLPtrPtr struct {
	C  **tg.SecretToken `xml:",chardata"`
	CD **tg.SecretToken `xml:",cdata"`
}
