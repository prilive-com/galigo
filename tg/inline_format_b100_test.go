package tg

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	htmltemplate "html/template"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The B-100 tests are internal (package tg): internal/testutil imports tg, so this file cannot use it. It is the twin
// of sender/payments_format_b100_test.go for InputInvoiceMessageContent (the same harness; read that file's comments).

// b100Token is a fake value; it is never sent anywhere.
const b100Token = "B100-FAKE-provider-token-tg"

// b100OuterInv names InputInvoiceMessageContent inside the oracles, where a local type of the same name hides it.
type b100OuterInv = InputInvoiceMessageContent

// b100PlainInv returns v as a value of a test-local type with the same name, package and fields as
// InputInvoiceMessageContent and no methods, with a non-empty ProviderToken set to [REDACTED] (the oracle).
func b100PlainInv(v InputInvoiceMessageContent) any {
	type InputInvoiceMessageContent b100OuterInv
	if v.ProviderToken != "" {
		v.ProviderToken = "[REDACTED]"
	}
	return InputInvoiceMessageContent(v)
}

// b100RawInv is b100PlainInv without the redaction.
func b100RawInv(v InputInvoiceMessageContent) any {
	type InputInvoiceMessageContent b100OuterInv
	return InputInvoiceMessageContent(v)
}

// b100Kind describes one request type: its oracles, its token field and the user types that embed it (Go cannot embed
// a type parameter, so each type names its own).
type b100Kind[T any] struct {
	name  string
	plain func(T) any
	raw   func(T) any
	token func(*T) *string
	// embedded returns a user value that embeds v by value, a pointer to it, and one that embeds a pointer to v.
	embedded func(v T) []any
	// nilEmbedded returns a user value that embeds a nil pointer to T.
	nilEmbedded func() any
}

type b100InvEmbedV struct {
	InputInvoiceMessageContent
	Name string
}

type b100InvEmbedP struct {
	*InputInvoiceMessageContent
	Name string
}

var b100Inv = b100Kind[InputInvoiceMessageContent]{
	name: "InputInvoiceMessageContent", plain: b100PlainInv, raw: b100RawInv,
	token: func(r *InputInvoiceMessageContent) *string { return &r.ProviderToken },
	embedded: func(v InputInvoiceMessageContent) []any {
		return []any{b100InvEmbedV{v, "app"}, &b100InvEmbedV{v, "app"}, b100InvEmbedP{&v, "app"}}
	},
	nilEmbedded: func() any { return b100InvEmbedP{nil, "app"} },
}

type b100Val[T any] struct {
	name string
	v    T
}

// b100Values: full (every field distinct, the fake token), empty_token (the same fill, no token, no shared slices),
// zero, and empty_lists (full with every list empty but not nil, also lists reached through a pointer: %#v prints {}
// for it, not nil, and the JSON prints []).
func b100Values[T any](k b100Kind[T]) []b100Val[T] {
	var full T
	b100Fill(&full)
	*k.token(&full) = b100Token
	var empty T // its own fill: a copy of full would share full's slices, so one row's write would show in the other
	b100Fill(&empty)
	*k.token(&empty) = ""
	var zero T
	var lists T
	b100Fill(&lists)
	*k.token(&lists) = b100Token
	b100EmptyLists(reflect.ValueOf(&lists).Elem())
	return []b100Val[T]{{"full", full}, {"empty_token", empty}, {"zero", zero}, {"empty_lists", lists}}
}

// b100EmptyLists makes every list in the struct v, and in every struct it reaches through a pointer, empty but not nil.
func b100EmptyLists(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for j := range v.NumField() {
			if v.Type().Field(j).IsExported() {
				b100EmptyLists(v.Field(j))
			}
		}
	case reflect.Pointer:
		if !v.IsNil() {
			b100EmptyLists(v.Elem())
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
}

// b100Fill sets every exported field of the struct *p, and of every struct it reaches through pointers and lists (three
// pointer or list steps down; a struct field is no step, so ReplyMarkup.InlineKeyboard is filled), to a value no other
// field has, so a wrong Format that drops, swaps, re-encodes or writes through a field changes the output or the JSON:
// strings hold an invalid UTF-8 byte and a space at each
// end, every second number is negative, bools are true, a list has two items in descending order, an interface holds an
// int64. Another kind panics, so a new field type cannot go untested. The caller sets the token.
func b100Fill(p any) {
	n := 0
	b100FillValue(reflect.ValueOf(p).Elem(), 0, &n)
}

func b100FillValue(v reflect.Value, depth int, n *int) {
	*n++
	i := int64(*n)
	if i%2 == 0 {
		i = -i
	}
	switch v.Kind() {
	case reflect.Struct:
		for j := range v.NumField() {
			if v.Type().Field(j).IsExported() {
				b100FillValue(v.Field(j), depth, n) // only a pointer or a list step counts as depth
			}
		}
	case reflect.String:
		v.SetString(fmt.Sprintf(" f%d\xff ", *n))
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(*n))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(i) + 0.5)
	case reflect.Interface:
		v.Set(reflect.ValueOf(i * 1000))
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		if depth < 3 {
			b100FillValue(v.Elem(), depth+1, n)
		}
	case reflect.Slice:
		if depth >= 3 {
			return
		}
		l := reflect.MakeSlice(v.Type(), 2, 2)
		for j := 1; j >= 0; j-- { // the later item gets the smaller value: descending
			b100FillValue(l.Index(j), depth+1, n)
		}
		v.Set(l)
	default:
		panic("b100Fill: a field has type " + v.Type().String())
	}
}

// b100Directives: the verbs v s q x X d o e E f F g G t b c U O and an unknown one, flags, width, precision and argument
// index, and after an index a verb that fmt would otherwise read as directive syntax (B-101's list without %T, %p and
// %w, which never reach Format: TestB100_TgLimits). The slice is not a constant format, so vet does not check it.
var b100Directives = []struct{ name, d string }{
	{"v", "%v"}, {"plus_v", "%+v"}, {"sharp_v", "%#v"}, {"s", "%s"}, {"q", "%q"}, {"x", "%x"}, {"X", "%X"},
	{"d", "%d"}, {"o", "%o"}, {"e", "%e"}, {"t", "%t"}, {"unknown_z", "%z"}, {"width_prec", "%10.3v"},
	{"minus_width", "%-12v|"}, {"plus_sharp", "%+#v"}, {"space_x", "% x"}, {"sharp_x", "%#x"}, {"zero_d", "%08d"},
	{"prec_s", "%.2s"}, {"plus_q", "%+q"}, {"sharp_q", "%#q"}, {"arg_index", "%[1]v"},
	{"arg_index_twice", "%[1]+v %[1]s"}, {"index_flag", "%[1]+"}, {"index_sharp", "%[1]#"}, {"index_space", "%[1] v"},
	{"index_bracket", "%[1][1]v"}, {"index_close", "%[1]]"}, {"index_star", "%[1]*v"}, {"index_digit", "%5.2[1]3v"},
	{"index_multibyte", "%[1]é"}, {"index_second", "%[1]v %[2]v"},
	{"zero_prec_s", "%.0s"}, {"dot_prec_v", "%.v"}, {"b", "%b"}, {"c", "%c"}, {"U", "%U"}, {"O", "%O"},
	{"g", "%g"}, {"dot_verb", "%.."}, {"invalid_rune", "%\xff"}, {"index_invalid_rune", "%[1]\xff"},
	{"f", "%f"}, {"F", "%F"}, {"E", "%E"}, {"G", "%G"},
	{"space_v", "% v"}, {"minus_s", "%-12s|"}, {"minus_d", "%-8d|"},
}

// b100Matrix is the one generated cross product of the directive axes: every ASCII letter
// as the verb (known and unknown verbs; not T, p and w, which never reach Format: TestB100_SenderLimits) × every subset
// of the five flags × no width or precision, width 10, precision .0 or .2, 10.2 × no argument index or [1]. TopLevel, Pointer
// and Concurrent use it (with b100Directives: b100AllDirectives), and Bools and ReadOnly except in a race build
// (b100ValueDirectives), for all three types; Nested uses the short verb list b100Verbs over its shapes.
// The other test file holds the same list; the twin gate checks it.
var b100Matrix = func() (m []string) {
	for _, verb := range "abcdefghijklmnoqrstuvxyzABCDEFGHIJKLMNOPQRSUVWXYZ" {
		for flags := range 32 {
			mod := ""
			for b, c := range " +-#0" {
				if flags&(1<<b) != 0 {
					mod += string(c)
				}
			}
			for _, wp := range []string{"", "10", ".0", ".2", "10.2"} { // .0: a zero precision
				for _, idx := range []string{"", "[1]"} {
					m = append(m, "%"+mod+wp+idx+string(verb)+"|")
				}
			}
		}
	}
	return m
}()

// b100AllDirectives: b100Directives and b100Matrix.
func b100AllDirectives() []string {
	all := slices.Clone(b100Matrix)
	for _, dd := range b100Directives {
		all = append(all, dd.d)
	}
	return all
}

// b100Race is true in a race build (set by the race-only test file of this package).
var b100Race bool

// b100ValueDirectives: the directives of the rows that check values, not races (Bools, ReadOnly). In a race build they
// are the named list: the race build adds race checks, not value checks, and the matrix there takes minutes (Go's own
// tests change what they run under the race detector: go1.27.4 fmt/fmt_test.go:1526). Every CI job without -race and
// the mutation engine run the whole list. Concurrent always uses the whole list.
func b100ValueDirectives() []string {
	if !b100Race {
		return b100AllDirectives()
	}
	named := make([]string, 0, len(b100Directives))
	for _, dd := range b100Directives {
		named = append(named, dd.d)
	}
	return named
}

// b100Each checks each directive and stops at the first wrong one: the others repeat it, and one diff per directive
// (12,544) made one red test log about 4.2 GB. Each directive first prints the prime values.
func b100Each(t *testing.T, dirs []string, want, got any, prime ...any) {
	t.Helper()
	for _, d := range dirs {
		for _, q := range prime {
			_ = fmt.Sprintf(d, q)
		}
		if !assert.Equal(t, fmt.Sprintf(d, want), fmt.Sprintf(d, got), d) {
			return
		}
	}
}

func TestB100_TgFormatter(t *testing.T) {
	formatter := reflect.TypeFor[fmt.Formatter]()
	t.Run("value", func(t *testing.T) {
		assert.True(t, reflect.TypeFor[InputInvoiceMessageContent]().Implements(formatter), "AC1")
	})
	t.Run("pointer", func(t *testing.T) {
		assert.True(t, reflect.TypeFor[*InputInvoiceMessageContent]().Implements(formatter), "AC1")
	})
}

// TestB100_TgTopLevel (AC2): a value prints as the method-less copy with the token redacted, for every directive.
func TestB100_TgTopLevel(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100TopLevel(t, b100Inv) })
}

func b100TopLevel[T any](t *testing.T, k b100Kind[T]) {
	for _, c := range b100Values(k) {
		t.Run(c.name, func(t *testing.T) {
			p := k.plain(c.v)
			for _, dd := range b100Directives {
				t.Run(dd.name, func(t *testing.T) { assert.Equal(t, fmt.Sprintf(dd.d, p), fmt.Sprintf(dd.d, c.v)) })
			}
			t.Run("matrix", func(t *testing.T) {
				b100Each(t, b100Matrix, p, c.v)
			})
			t.Run("star_width", func(t *testing.T) { assert.Equal(t, fmt.Sprintf("%*v|", 9, p), fmt.Sprintf("%*v|", 9, c.v)) })
			t.Run("star_prec", func(t *testing.T) { assert.Equal(t, fmt.Sprintf("%.*v", 1, p), fmt.Sprintf("%.*v", 1, c.v)) })
			t.Run("index_reorder", func(t *testing.T) {
				assert.Equal(t, fmt.Sprintf("%[2]v %[1]d", 7, p), fmt.Sprintf("%[2]v %[1]d", 7, c.v))
			})
			t.Run("sprint", func(t *testing.T) { assert.Equal(t, fmt.Sprint(p, 1), fmt.Sprint(c.v, 1)) })
			t.Run("sprintln", func(t *testing.T) { assert.Equal(t, fmt.Sprintln(p), fmt.Sprintln(c.v)) })
		})
	}
}

// TestB100_TgPointer (AC3): a pointer prints as the value (no &); a nil pointer prints <nil> for every directive
// that reaches Format (fmt's catchPanic, go1.27.4 fmt/print.go:580-588), as a Formatter that writes <nil> would.
func TestB100_TgPointer(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Pointer(t, b100Inv) })
}

// b100NilText prints <nil> for every verb: the oracle of a nil pointer.
type b100NilText struct{}

func (b100NilText) Format(f fmt.State, _ rune) { fmt.Fprint(f, "<nil>") }

func b100Pointer[T any](t *testing.T, k b100Kind[T]) {
	for _, c := range b100Values(k)[:2] {
		t.Run(c.name, func(t *testing.T) {
			p, ptr := k.plain(c.v), &c.v
			for _, dd := range b100Directives {
				t.Run(dd.name, func(t *testing.T) { assert.Equal(t, fmt.Sprintf(dd.d, p), fmt.Sprintf(dd.d, ptr)) })
			}
			t.Run("matrix", func(t *testing.T) {
				b100Each(t, b100Matrix, p, ptr)
			})
		})
	}
	t.Run("nil", func(t *testing.T) {
		var np *T
		b100Each(t, b100AllDirectives(), b100NilText{}, np)
	})
}

// TestB100_TgIndexException pins AC2's one named exception: a digit or * verb after an
// argument index when fmt holds no width other than 0 and no precision. Format prints %!(NOVERB), after fmt's own
// prefix, where the baseline printed bad-verb text. No token prints either way.
func TestB100_TgIndexException(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100IndexException(t, b100Inv) })
}

func b100IndexException[T any](t *testing.T, k b100Kind[T]) {
	rows := []struct {
		name, d string
		w       any
		want    string
	}{
		{"zero_width_digit", "%*[2]0", 0, "%!(NOVERB)"},
		{"zero_width_star", "%*[2]*", 0, "%!(BADWIDTH)%!(NOVERB)"},
		{"bad_prec_digit", "%.*[2]5", -1, "%!(BADPREC)%!(NOVERB)"},
		{"non_int_prec_digit", "%.*[2]5", "x", "%!(BADPREC)%!(NOVERB)"},
		// fmt takes any integer that fits an int as a * argument (PLAN AC2).
		{"int64_zero_width_digit", "%*[2]0", int64(0), "%!(NOVERB)"},
		{"uint_zero_width_digit", "%*[2]0", uint(0), "%!(NOVERB)"},
	}
	for _, c := range b100Values(k)[:2] {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range rows {
				t.Run(r.name, func(t *testing.T) {
					for _, x := range []any{c.v, &c.v} {
						got := fmt.Sprintf(r.d, r.w, x)
						assert.Equal(t, r.want, got)
						assert.NotEqual(t, fmt.Sprintf(r.d, r.w, k.plain(c.v)), got, "the baseline text differs")
						b100NoToken(t, got)
					}
				})
			}
			// Not the exception: a width before the index reaches Format, which redacts (PLAN AC2).
			t.Run("width_then_index_digit", func(t *testing.T) {
				for _, x := range []any{c.v, &c.v} {
					assert.Equal(t, fmt.Sprintf(b100WI, k.plain(c.v)), fmt.Sprintf(b100WI, x))
				}
			})
		})
	}
}

// b100Field, b100PtrField, b100AnyField and b100DoublePtr hold a request below the top level in a named field.
type (
	b100Field[T any]     struct{ R T }
	b100PtrField[T any]  struct{ R *T }
	b100AnyField         struct{ R any }
	b100DoublePtr[T any] struct{ R **T }
)

// b100Shape is a value that holds a request below the top level, and the text fmt must print for it: verb is the verb
// with its flags, in the oracle's top-level text of the request for that verb, typ the %T of the shape (%#v names it).
type b100Shape[T any] struct {
	name string
	val  func(v *T) any
	want func(verb, in, typ string) string
}

func b100Wrap(prefix string) func(verb, in, typ string) string {
	return func(verb, in, typ string) string {
		switch verb {
		case "#v":
			return typ + "{" + prefix + in + "}"
		case "+v":
			return "{" + prefix + in + "}"
		}
		return "{" + in + "}"
	}
}

func b100List(verb, in, typ string) string {
	if verb == "#v" {
		return typ + "{" + in + "}"
	}
	return "[" + in + "]"
}

func b100Shapes[T any]() []b100Shape[T] {
	return []b100Shape[T]{
		{"field_value", func(v *T) any { return b100Field[T]{*v} }, b100Wrap("R:")},
		{"field_pointer", func(v *T) any { return b100PtrField[T]{v} }, b100Wrap("R:")},
		{"iface_field", func(v *T) any { return b100AnyField{*v} }, b100Wrap("R:")},
		{"iface_field_pointer", func(v *T) any { return b100AnyField{v} }, b100Wrap("R:")},
		{"slice_value", func(v *T) any { return []T{*v} }, b100List},
		{"slice_pointer", func(v *T) any { return []*T{v} }, b100List},
		{"array_value", func(v *T) any { return [1]T{*v} }, b100List},
		{"array_pointer", func(v *T) any { return [1]*T{v} }, b100List},
		{"anys", func(v *T) any { return []any{*v, v} }, func(verb, in, typ string) string {
			if verb == "#v" {
				return typ + "{" + in + ", " + in + "}"
			}
			return "[" + in + " " + in + "]"
		}},
		{"map_value", func(v *T) any { return map[int]T{1: *v} }, func(verb, in, typ string) string {
			k := fmt.Sprintf("%"+verb, 1)
			if verb == "#v" {
				return typ + "{" + k + ":" + in + "}"
			}
			return "map[" + k + ":" + in + "]"
		}},
		{"map_pointer_key", func(v *T) any { return map[*T]int{v: 1} }, b100KeyWant},
		{"map_any_key", func(v *T) any { return map[any]int{v: 1} }, b100KeyWant},
	}
}

func b100KeyWant(verb, in, typ string) string {
	e := fmt.Sprintf("%"+verb, 1)
	if verb == "#v" {
		return typ + "{" + in + ":" + e + "}"
	}
	return "map[" + in + ":" + e + "]"
}

var b100Verbs = []string{"v", "+v", "#v", "s", "q", "x", "X", "d", "10.3v", "-8s"}

// TestB100_TgNested (AC4): below the top level a request prints its fields with the token redacted, as at the top
// level; the whole text equals the shape around the oracle's text.
func TestB100_TgNested(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Nested(t, b100Inv) })
}

func b100Nested[T any](t *testing.T, k b100Kind[T]) {
	for _, c := range b100Values(k)[:2] {
		t.Run(c.name, func(t *testing.T) {
			for _, sh := range b100Shapes[T]() {
				t.Run(sh.name, func(t *testing.T) {
					for _, verb := range b100Verbs {
						x := sh.val(&c.v)
						got := fmt.Sprintf("%"+verb, x)
						assert.Equal(t, sh.want(verb, fmt.Sprintf("%"+verb, k.plain(c.v)), fmt.Sprintf("%T", x)), got, verb)
						b100NoToken(t, got)
					}
				})
			}
			t.Run("embedded", func(t *testing.T) {
				// Selector table cell 0: the outer type gets the promoted Format, so only the request prints.
				for i, x := range k.embedded(c.v) {
					for _, verb := range b100Verbs {
						got := fmt.Sprintf("%"+verb, x)
						assert.Equal(t, fmt.Sprintf("%"+verb, k.plain(c.v)), got, "%d %s", i, verb)
						b100NoToken(t, got)
					}
				}
			})
			t.Run("double_pointer", func(t *testing.T) {
				// fmt calls no method on a **T: it prints the address, as before (PLAN AC4).
				pp := &[]*T{&c.v}[0]
				x := b100DoublePtr[T]{pp}
				for _, verb := range b100Verbs {
					got := fmt.Sprintf("%"+verb, x)
					assert.Equal(t, b100Wrap("R:")(verb, fmt.Sprintf("%"+verb, pp), fmt.Sprintf("%T", x)), got, verb)
					b100NoToken(t, got)
				}
			})
			t.Run("errorf", func(t *testing.T) {
				for _, verb := range b100Verbs {
					got := fmt.Errorf("e: %"+verb, c.v).Error()
					assert.Equal(t, fmt.Sprintf("e: %"+verb, k.plain(c.v)), got, verb)
					b100NoToken(t, got)
				}
			})
		})
	}
	t.Run("nil_embedded", func(t *testing.T) {
		// Selector table cell 0 with a nil embedded pointer: fmt's panic text (PLAN §9), no token.
		assert.Equal(t, "%!v(PANIC=Format method: runtime error: invalid memory address or nil pointer dereference)",
			fmt.Sprintf("%v", k.nilEmbedded()))
	})
}

// b100ContentField holds the content in the field type the inline results use (an interface with an unexported method).
type b100ContentField struct{ C InputMessageContent }

// TestB100_TgContentField (AC4): the content in an InputMessageContent field, value and pointer, prints redacted; the
// whole text equals the field around the oracle's text. InlineQueryResultArticle.InputMessageContent is such a field.
func TestB100_TgContentField(t *testing.T) {
	for _, c := range b100Values(b100Inv)[:2] {
		t.Run(c.name, func(t *testing.T) {
			v := c.v
			for _, x := range []b100ContentField{{v}, {&v}} {
				for _, verb := range b100Verbs {
					got := fmt.Sprintf("%"+verb, x)
					assert.Equal(t, b100Wrap("C:")(verb, fmt.Sprintf("%"+verb, b100PlainInv(v)), fmt.Sprintf("%T", x)), got, verb)
					b100NoToken(t, got)
				}
			}
		})
	}
}

// TestB100_TgBools: each bool alone, so a copy that swaps two bools fails.
func TestB100_TgBools(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Bools(t, b100Inv) })
}

func b100Bools[T any](t *testing.T, k b100Kind[T]) {
	for _, c := range b100BoolValues(k) {
		t.Run(c.name, func(t *testing.T) { // a swap behind one directive shows
			b100Each(t, b100ValueDirectives(), k.plain(c.v), c.v)
		})
	}
}

// b100BoolValues: each bool field alone true in the zero request, named by the field; the fake token, " \t" every 2nd row.
func b100BoolValues[T any](k b100Kind[T]) (vs []b100Val[T]) {
	typ := reflect.TypeFor[T]()
	for i := range typ.NumField() {
		if typ.Field(i).Type.Kind() == reflect.Bool {
			var v T
			*k.token(&v) = []string{b100Token, " \t"}[len(vs)%2]
			reflect.ValueOf(&v).Elem().Field(i).SetBool(true)
			vs = append(vs, b100Val[T]{typ.Field(i).Name, v})
		}
	}
	return vs
}

// TestB100_TgSlog: slog's text handler prints the request redacted, also as a pointer and inside a group; the JSON
// handler prints it as before, token included (guards: it is the wire form, AC5).
func TestB100_TgSlog(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Slog(t, b100Inv) })
}

func b100Slog[T any](t *testing.T, k b100Kind[T]) {
	for _, c := range b100Values(k)[:2] {
		t.Run(c.name, func(t *testing.T) {
			p := k.plain(c.v)
			t.Run("text_value", func(t *testing.T) {
				got, _ := b100Log(c.v)
				want, _ := b100Log(p)
				assert.Equal(t, want, got)
				b100NoToken(t, got)
			})
			t.Run("text_pointer", func(t *testing.T) {
				got, _ := b100Log(&c.v)
				want, _ := b100Log(p)
				assert.Equal(t, want, got)
				b100NoToken(t, got)
			})
			t.Run("text_group", func(t *testing.T) {
				got, _ := b100Log(slog.GroupValue(slog.Any("r", c.v)))
				want, _ := b100Log(slog.GroupValue(slog.Any("r", p)))
				assert.Equal(t, want, got)
				b100NoToken(t, got)
			})
			t.Run("json_value", func(t *testing.T) {
				_, got := b100Log(c.v)
				_, want := b100Log(k.raw(c.v))
				assert.Equal(t, want, got)
			})
			t.Run("json_pointer", func(t *testing.T) {
				_, got := b100Log(&c.v)
				_, want := b100Log(k.raw(c.v))
				assert.Equal(t, want, got)
			})
		})
	}
}

// TestB100_TgWire (AC5): the JSON sent to Telegram is unchanged and carries the token (guards).
func TestB100_TgWire(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Wire(t, b100Inv) })
}

func b100Wire[T any](t *testing.T, k b100Kind[T]) {
	for _, c := range b100Values(k) {
		t.Run(c.name, func(t *testing.T) {
			for _, x := range []any{c.v, &c.v} {
				got, err := json.Marshal(x)
				require.NoError(t, err)
				want, err := json.Marshal(k.raw(c.v))
				require.NoError(t, err)
				assert.Equal(t, string(want), string(got))
				tok := c.v
				assert.Equal(t, *k.token(&tok) != "", strings.Contains(string(got), `"provider_token":"`+b100Token+`"`))
			}
		})
	}
}

// TestB100_TgTemplate: text/template and html/template {{.}} call Format and print the request redacted.
func TestB100_TgTemplate(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Template(t, b100Inv) })
}

func b100Template[T any](t *testing.T, k b100Kind[T]) {
	full := b100Values(k)[0].v
	for _, src := range []string{"{{.}}", `{{printf "%s" .}}`, `{{printf "%#v" .}}`} {
		t.Run("text/"+src, func(t *testing.T) {
			got, want := b100Exec(t, false, src, full), b100Exec(t, false, src, k.plain(full))
			assert.Equal(t, want, got)
			b100NoToken(t, got)
		})
		t.Run("html/"+src, func(t *testing.T) {
			got, want := b100Exec(t, true, src, &full), b100Exec(t, true, src, k.plain(full))
			assert.Equal(t, want, got)
			b100NoToken(t, got)
		})
	}
}

func b100Exec(t *testing.T, html bool, src string, data any) string {
	t.Helper()
	var b bytes.Buffer
	if html {
		require.NoError(t, htmltemplate.Must(htmltemplate.New("t").Parse(src)).Execute(&b, data))
	} else {
		require.NoError(t, template.Must(template.New("t").Parse(src)).Execute(&b, data))
	}
	return b.String()
}

// TestB100_TgLiteral: exact texts written out, so the type name does not rest on the oracle alone; the field stays a
// string (AC5).
func TestB100_TgLiteral(t *testing.T) {
	full := b100Values(b100Inv)[0].v
	t.Run("sharp_v_value", func(t *testing.T) {
		got := fmt.Sprintf("%#v", full)
		assert.True(t, strings.HasPrefix(got, "tg.InputInvoiceMessageContent{Title:"), got)
		assert.Contains(t, got, `ProviderToken:"[REDACTED]", Currency:`)
	})
	t.Run("sharp_v_pointer", func(t *testing.T) {
		assert.True(t, strings.HasPrefix(fmt.Sprintf("%#v", &full), "tg.InputInvoiceMessageContent{Title:"))
	})
	t.Run("T", func(t *testing.T) {
		assert.Equal(t, "tg.InputInvoiceMessageContent *tg.InputInvoiceMessageContent", fmt.Sprintf("%T %T", full, &full))
	})
	t.Run("field_type", func(t *testing.T) {
		f, ok := reflect.TypeFor[InputInvoiceMessageContent]().FieldByName("ProviderToken")
		require.True(t, ok)
		assert.Equal(t, reflect.TypeFor[string](), f.Type)
	})
}

// TestB100_TgReadOnly: printing changes nothing, full and empty, value and pointer: Format's receiver is a
// copy. The %#v of the raw copy and the JSON see every field, the token row reads the token.
func TestB100_TgReadOnly(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100ReadOnly(t, b100Inv) })
}

func b100ReadOnly[T any](t *testing.T, k b100Kind[T]) {
	for _, c := range b100Values(k) { // every value: empty_lists holds the empty nested lists
		t.Run(c.name, func(t *testing.T) {
			v := c.v
			tok, before := *k.token(&v), fmt.Sprintf("%#v", k.raw(v))
			js, err := json.Marshal(v)
			require.NoError(t, err)
			for _, d := range b100ValueDirectives() { // through the pointer: a value shares the same lists and pointer targets
				_ = fmt.Sprintf(d, &v)
			}
			for _, dd := range b100Directives {
				_ = fmt.Sprintf(dd.d, v)
				_ = fmt.Sprintf(dd.d, &v)
			}
			for _, sh := range b100Shapes[T]() {
				for _, verb := range b100Verbs {
					_ = fmt.Sprintf("%"+verb, sh.val(&v))
				}
			}
			t.Run("token", func(t *testing.T) { assert.Equal(t, tok, *k.token(&v)) })
			t.Run("fields", func(t *testing.T) { assert.Equal(t, before, fmt.Sprintf("%#v", k.raw(v))) })
			t.Run("json", func(t *testing.T) {
				after, err := json.Marshal(v)
				require.NoError(t, err)
				assert.Equal(t, string(js), string(after))
			})
		})
	}
	// For each field but the token, a copy with only that field changed prints its own text with every directive, each
	// right after full with the same directive: a cache keyed by some fields, even with one entry per directive, returns
	// full's stale text. The copy's lists share v's arrays, so a print
	// that writes through them changes the oracle too: the copy's %#v must not change.
	t.Run("print_change_print", func(t *testing.T) {
		all := b100ValueDirectives()
		v := b100Values(k)[0].v
		typ := reflect.TypeFor[T]()
		for i := range typ.NumField() {
			if typ.Field(i).Name == "ProviderToken" {
				continue
			}
			t.Run(typ.Field(i).Name, func(t *testing.T) {
				x := v // b100Change replaces the field; a list keeps v's array
				b100Change(reflect.ValueOf(&x).Elem().Field(i))
				p, raw := k.plain(x), fmt.Sprintf("%#v", k.raw(x))
				b100Each(t, all, p, &x, &v)
				assert.Equal(t, raw, fmt.Sprintf("%#v", k.raw(x)), "a print wrote through a list the copy shares with v")
			})
		}
	})
}

// b100Change gives the field f another value of its kind (a list keeps its first item only; a pointer becomes nil).
func b100Change(f reflect.Value) {
	switch f.Kind() {
	case reflect.String:
		f.SetString(f.String() + "-changed")
	case reflect.Bool:
		f.SetBool(!f.Bool())
	case reflect.Int, reflect.Int64:
		f.SetInt(f.Int() + 100)
	case reflect.Interface:
		f.Set(reflect.ValueOf(int64(-7777)))
	case reflect.Pointer:
		f.Set(reflect.Zero(f.Type()))
	case reflect.Slice:
		f.Set(f.Slice(0, 1))
	default:
		panic("b100Change: kind " + f.Kind().String())
	}
}

// TestB100_TgConcurrent: goroutines print shared full and empty requests with every directive while others read
// their tokens; they see the same text each time and the raw token. A Format that writes the request, even one that
// restores it, is a data race the -race gate reports here. %p is not in the list.
func TestB100_TgConcurrent(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Concurrent(t, b100Inv) })
}

func b100Concurrent[T any](t *testing.T, k b100Kind[T]) {
	var ps []*T // every value class and each bool alone (as in Bools): a write may depend on the value
	want := "["
	for _, c := range append(b100Values(k), b100BoolValues(k)...) {
		ps, want = append(ps, &c.v), want+fmt.Sprintf("%v ", k.plain(c.v))
	}
	assert.Equal(t, want[:len(want)-1]+"]", fmt.Sprintf("%v", ps), "a request pointer in a slice prints its fields")
	wire := func() string { b, _ := json.Marshal(ps); return string(b) } // every field, token included, without Format
	t0, bad, mu, wg := wire(), []string{}, sync.Mutex{}, sync.WaitGroup{}
	check := func(d, got, w string) { // no t call and no lock unless wrong: a lock orders the goroutines
		if got != w {
			mu.Lock()
			bad = append(bad, d+" -> "+got[:min(len(got), 1000)])
			mu.Unlock()
		}
	}
	// The goroutines print one directive at once: fmt's sync.Pool orders prints that do not overlap in time, so the race
	// detector misses a write between them (with one pass per goroutine, the detector caught a writing Format in only 2 runs of 5).
	for _, d := range b100AllDirectives() {
		w, start := fmt.Sprintf(d, ps), make(chan struct{})
		for range 4 {
			wg.Go(func() { <-start; check(d, fmt.Sprintf(d, ps), w) })
		}
		wg.Go(func() { <-start; check("json", wire(), t0) })
		close(start)
		wg.Wait()
	}
	assert.Zero(t, len(bad), "the first wrong results: %q", bad[:min(len(bad), 3)])
}

// b100Named, b100Unexported and b100UnexportedPtr hold a request in a field of a user type that has no Format.
type (
	b100Named[T any]         struct{ Inner T }
	b100Unexported[T any]    struct{ r T }
	b100UnexportedPtr[T any] struct{ r *T }
)

// TestB100_TgLimits pins the paths that still print the token (PLAN AC6), as on the baseline, so a later change
// cannot claim them silently. The sender selector file pins the hidden (c4) and cancelled (c3) cells.
func TestB100_TgLimits(t *testing.T) {
	t.Run(b100Inv.name, func(t *testing.T) { b100Limits(t, b100Inv) })
}

func b100Limits[T any](t *testing.T, k b100Kind[T]) {
	full := b100Values(k)[0].v
	// %w of any value that holds a request, and %p of such a value that is not a pointer, map, slice, chan or func,
	// print the whole value, outer fields included: fmt handles them before methods.
	embedded := k.embedded(full)[0]
	for _, c := range []struct {
		name string
		x    any
	}{{"value", full}, {"pointer", &full}, {"embedding", embedded}, {"embedding_pointer", k.embedded(full)[1]},
		{"array", [1]T{full}}, {"slice", []T{full}}, {"map", map[int]T{1: full}}} {
		t.Run("w_"+c.name, func(t *testing.T) { assert.Contains(t, fmt.Errorf(b100W, c.x).Error(), b100Token) })
	}
	// Below the top level a request pointer is an address on that path: no token (PLAN rev p3a AC6).
	for _, c := range []struct {
		name string
		x    any
	}{{"embedded_pointer", k.embedded(full)[2]}, {"pointer_slice", []*T{&full}}, {"pointer_array", [1]*T{&full}}} {
		t.Run("w_address_"+c.name, func(t *testing.T) { b100NoToken(t, fmt.Errorf(b100W, c.x).Error()) })
	}
	t.Run("p_address_embedded_pointer", func(t *testing.T) { b100NoToken(t, fmt.Sprintf(b100P, k.embedded(full)[2])) })
	t.Run("p_address_pointer_slice", func(t *testing.T) { b100NoToken(t, fmt.Sprintf(b100P, []*T{&full})) })
	t.Run("w_embedding_outer", func(t *testing.T) { assert.Contains(t, fmt.Errorf(b100W, embedded).Error(), "app") })
	t.Run("w_nil_pointer", func(t *testing.T) {
		var np *T
		assert.Equal(t, fmt.Sprintf("%%!w(%T=<nil>)", np), fmt.Errorf(b100W, np).Error())
	})
	for _, c := range []struct {
		name string
		x    any
	}{{"value", full}, {"embedding", embedded}, {"array", [1]T{full}}} {
		t.Run("p_"+c.name, func(t *testing.T) { assert.Contains(t, fmt.Sprintf(b100P, c.x), b100Token) })
	}
	t.Run("p_pointer_address", func(t *testing.T) {
		got := fmt.Sprintf(b100P, &full)
		assert.True(t, strings.HasPrefix(got, "0x"), got)
		b100NoToken(t, got)
	})
	t.Run("named_field_user_pointer_s", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(b100S, []*b100Named[T]{{full}}), b100Token)
	})
	t.Run("unexported_value_v", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf("%v", b100Unexported[T]{full}), b100Token)
	})
	t.Run("unexported_value_x_hex", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(b100X, b100Unexported[T]{full}), fmt.Sprintf("%x", b100Token))
	})
	t.Run("unexported_value_T", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%T", b100Unexported[T]{}), fmt.Sprintf("%T", b100Unexported[T]{full}))
		b100NoToken(t, fmt.Sprintf("%T", b100Unexported[T]{full}))
	})
	t.Run("unexported_pointer", func(t *testing.T) {
		x := b100UnexportedPtr[T]{&full}
		assert.True(t, strings.HasPrefix(fmt.Sprintf("%v", x), "{0x"), "an address")
		assert.Contains(t, fmt.Sprintf(b100S, x), b100Token)
	})
	t.Run("xml", func(t *testing.T) {
		b, err := xml.Marshal(full)
		require.NoError(t, err)
		assert.Contains(t, string(b), b100Token)
	})
	t.Run("field_read", func(t *testing.T) {
		assert.Equal(t, b100Token, b100Exec(t, false, "{{.ProviderToken}}", full))
	})
}

// b100S, b100P, b100W, b100X and b100WI are variable formats: vet and staticcheck reject these directives in a constant one.
var b100S, b100P, b100W, b100X, b100WI = "%s", "%p", "%w", "%x", "%5[1]0"

func b100Log(v any) (text, json string) {
	noTime := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}
	var tb, jb bytes.Buffer
	slog.New(slog.NewTextHandler(&tb, &slog.HandlerOptions{ReplaceAttr: noTime})).Info("m", "c", v)
	slog.New(slog.NewJSONHandler(&jb, &slog.HandlerOptions{ReplaceAttr: noTime})).Info("m", "c", v)
	return tb.String(), jb.String()
}

// b100NoToken: the output holds the fake token neither as text nor in hex.
func b100NoToken(t *testing.T, got string) {
	t.Helper()
	assert.NotContains(t, got, b100Token)
	assert.NotContains(t, got, fmt.Sprintf("%x", b100Token))
	assert.NotContains(t, got, fmt.Sprintf("%X", b100Token))
}
