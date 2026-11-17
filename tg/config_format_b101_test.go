package tg

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The B-101 tests are internal (package tg): internal/testutil imports sender and tg, so these files cannot use it.

// b101Token and b101Secret are fake values; they are never sent anywhere.
const (
	b101Token  = "123456789:B101-FAKE-token-tg-AAAAAAAAAA"
	b101Secret = "B101-FAKE-webhook-secret-tg"
)

// b101Outer names Config inside b101Plain, where a local type of the same name hides it.
type b101Outer = Config

// b101Plain returns c as a pointer to a test-local type with the same name, package and fields as Config and no
// methods. fmt prints it as it printed a *Config before B-101 (TEST-PLAN §2: on the baseline the guard rows compare
// the oracle with the real *Config, so a green baseline run proves the oracle equals the old output).
func b101Plain(c *Config) any {
	type Config b101Outer
	return (*Config)(c)
}

// b101PlainValue is b101Plain for a Config value.
func b101PlainValue(c Config) any {
	type Config b101Outer
	return Config(c)
}

type b101Case struct {
	name string
	c    *Config
}

// b101Configs: every exported field set to a distinct non-zero value (both secrets where the type has two), the
// zero value, and a nil pointer.
func b101Configs() []b101Case {
	var full Config
	b101Fill(&full)
	full.Token = SecretToken(b101Token)
	return []b101Case{{"full", &full}, {"zero", &Config{}}, {"nil", nil}}
}

// b101Fill sets every exported field of *c that is a string, number or []string to a value no other field has, and
// every bool to true, so a wrong Format that drops or swaps a field changes the output; a list
// has two items in descending order, so a lost tail or a sort shows; every string field holds an
// invalid UTF-8 byte and every int with an even index is negative, so a re-encoding or a sign change shows;
// every string field has a space at each end and every float a fraction that float32 cannot hold, so a trim or a
// narrowing shows. Two bools can still be swapped unseen here: TestB101_TgBools sets each one alone. The caller sets the secrets and funcs.
// Another kind panics, so a new field type cannot go untested.
func b101Fill(c *Config) {
	v := reflect.ValueOf(c).Elem()
	for i := range v.NumField() {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(fmt.Sprintf(" f%d\xff ", i))
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int64:
			v := int64(i + 1)
			if i%2 == 0 {
				v = -v
			}
			f.SetInt(v)
		case reflect.Uint32:
			f.SetUint(uint64(i + 1))
		case reflect.Float64:
			f.SetFloat(float64(i) + 0.1)
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{fmt.Sprintf("f%d-b", i), fmt.Sprintf("f%d-a", i)}))
		case reflect.Func:
		default:
			panic("b101Fill: field " + v.Type().Field(i).Name + " has kind " + f.Kind().String())
		}
	}
}

// b101Directives: the verbs v s q x X d o e E f F g G t b c U O and an unknown one, every flag, width, precision and argument index (AC2), and after an argument
// index a verb that fmt would otherwise read as directive syntax (a flag character, a digit, a bracket, a multi-byte
// rune). %T, %p and %w never reach Format; their rows show the output is unchanged anyway. The slice is not a
// constant format, so vet does not check these directives.
var b101Directives = []struct{ name, d string }{
	{"v", "%v"}, {"plus_v", "%+v"}, {"sharp_v", "%#v"}, {"s", "%s"}, {"q", "%q"}, {"x", "%x"}, {"X", "%X"},
	{"d", "%d"}, {"o", "%o"}, {"e", "%e"}, {"t", "%t"}, {"unknown_z", "%z"}, {"width_prec", "%10.3v"},
	{"minus_width", "%-12v|"}, {"plus_sharp", "%+#v"}, {"space_x", "% x"}, {"sharp_x", "%#x"}, {"zero_d", "%08d"},
	{"prec_s", "%.2s"}, {"plus_q", "%+q"}, {"sharp_q", "%#q"}, {"T", "%T"}, {"p", "%p"}, {"arg_index", "%[1]v"},
	{"arg_index_twice", "%[1]+v %[1]s"}, {"index_flag", "%[1]+"}, {"index_sharp", "%[1]#"}, {"index_space", "%[1] v"},
	{"index_bracket", "%[1][1]v"}, {"index_close", "%[1]]"}, {"index_star", "%[1]*v"}, {"index_digit", "%5.2[1]3v"},
	{"index_multibyte", "%[1]é"}, {"index_second", "%[1]v %[2]v"}, {"w", "%w"},
	// a present zero precision, the other integer and float verbs, and a verb that is
	// '.' or an invalid rune, also after an index
	{"zero_prec_s", "%.0s"}, {"dot_prec_v", "%.v"}, {"b", "%b"}, {"c", "%c"}, {"U", "%U"}, {"O", "%O"},
	{"g", "%g"}, {"dot_verb", "%.."}, {"invalid_rune", "%\xff"}, {"index_invalid_rune", "%[1]\xff"},
	// the other float verbs
	{"f", "%f"}, {"F", "%F"}, {"E", "%E"}, {"G", "%G"},
	// the space flag with v, and the minus flag with verbs other than v
	{"space_v", "% v"}, {"minus_s", "%-12s|"}, {"minus_d", "%-8d|"},
}

func TestB101_TgFormatter(t *testing.T) {
	formatter := reflect.TypeFor[fmt.Formatter]()
	t.Run("pointer", func(t *testing.T) {
		assert.True(t, reflect.TypeFor[*Config]().Implements(formatter), "*Config implements fmt.Formatter (AC1)")
	})
	t.Run("value", func(t *testing.T) {
		assert.False(t, reflect.TypeFor[Config]().Implements(formatter), "Config does not (AC1, a pointer receiver)")
	})
}

// TestB101_TgTopLevel (AC2): every directive prints a *Config exactly as before. It passes on the baseline (a guard).
func TestB101_TgTopLevel(t *testing.T) {
	for _, cc := range b101Configs() {
		t.Run(cc.name, func(t *testing.T) {
			for _, dd := range b101Directives {
				t.Run(dd.name, func(t *testing.T) {
					assert.Equal(t, fmt.Sprintf(dd.d, b101Plain(cc.c)), fmt.Sprintf(dd.d, cc.c))
				})
			}
			t.Run("star_width", func(t *testing.T) {
				assert.Equal(t, fmt.Sprintf("%*v|", 9, b101Plain(cc.c)), fmt.Sprintf("%*v|", 9, cc.c))
			})
			t.Run("star_prec", func(t *testing.T) {
				assert.Equal(t, fmt.Sprintf("%.*v", 1, b101Plain(cc.c)), fmt.Sprintf("%.*v", 1, cc.c))
			})
			t.Run("index_reorder", func(t *testing.T) {
				assert.Equal(t, fmt.Sprintf("%[2]v %[1]d", 7, b101Plain(cc.c)), fmt.Sprintf("%[2]v %[1]d", 7, cc.c))
			})
			t.Run("sprint", func(t *testing.T) {
				assert.Equal(t, fmt.Sprint(b101Plain(cc.c), 1), fmt.Sprint(cc.c, 1))
			})
			t.Run("sprintln", func(t *testing.T) {
				assert.Equal(t, fmt.Sprintln(b101Plain(cc.c)), fmt.Sprintln(cc.c))
			})
		})
	}
}

// b101Matrix: every verb of b101Directives with each flag, a width, a zero and a non-zero precision, alone and mixed.
var b101Matrix = func() (m []string) {
	for _, verb := range "vsqxXdoObcUteEfFgG" {
		for _, mod := range []string{"", "+", "-", " ", "#", "0", "8", "08", ".0", ".2", "#.0", "-8.2", "+#08.2"} {
			m = append(m, "%"+mod+string(verb)+"|")
		}
	}
	return m
}()

// b101AllDirectives: b101Directives and b101Matrix, for the rows that check printing changes nothing.
func b101AllDirectives() []string {
	all := slices.Clone(b101Matrix)
	for _, dd := range b101Directives {
		all = append(all, dd.d)
	}
	return all
}

// TestB101_TgFlagMatrix: b101Matrix equal to the method-less copy, so a Format that drops a flag, the width or the
// precision for one of these verbs fails.
func TestB101_TgFlagMatrix(t *testing.T) {
	for _, cc := range b101Configs() {
		t.Run(cc.name, func(t *testing.T) {
			for _, d := range b101Matrix {
				assert.Equal(t, fmt.Sprintf(d, b101Plain(cc.c)), fmt.Sprintf(d, cc.c), d)
			}
		})
	}
}

type b101Field struct{ C *Config }

type b101Embed struct {
	*Config
	Name string
}

type b101ByValue struct {
	Config
	DB string
}

// b101Shape is a value that holds the config below the top level, and the text fmt must print for it: in is the
// oracle's top-level text of the config for the same verb, v the verb.
type b101Shape struct {
	name string
	val  func(c *Config) any
	want func(v, in string) string
}

var b101Shapes = []b101Shape{
	{"field", func(c *Config) any { return b101Field{c} }, func(v, in string) string {
		switch v {
		case "#v":
			return "tg.b101Field{C:" + in + "}"
		case "+v":
			return "{C:" + in + "}"
		}
		return "{" + in + "}"
	}},
	{"slice", func(c *Config) any { return []*Config{c} }, func(v, in string) string {
		if v == "#v" {
			return "[]*tg.Config{" + in + "}"
		}
		return "[" + in + "]"
	}},
	{"array", func(c *Config) any { return [1]*Config{c} }, func(v, in string) string {
		if v == "#v" {
			return "[1]*tg.Config{" + in + "}"
		}
		return "[" + in + "]"
	}},
	{"map_value", func(c *Config) any { return map[int]*Config{1: c} }, func(v, in string) string {
		k := fmt.Sprintf("%"+v, 1)
		if v == "#v" {
			return "map[int]*tg.Config{" + k + ":" + in + "}"
		}
		return "map[" + k + ":" + in + "]"
	}},
	{"map_key", func(c *Config) any { return map[*Config]int{c: 1} }, func(v, in string) string {
		e := fmt.Sprintf("%"+v, 1)
		if v == "#v" {
			return "map[*tg.Config]int{" + in + ":" + e + "}"
		}
		return "map[" + in + ":" + e + "]"
	}},
	{"any_slice", func(c *Config) any { return []any{c} }, func(v, in string) string {
		if v == "#v" {
			return "[]interface {}{" + in + "}"
		}
		return "[" + in + "]"
	}},
	// Selector table cell 0: the outer type gets the promoted Format, so only the config prints (also for nil).
	{"embedded_pointer", func(c *Config) any { return b101Embed{c, "myapp"} }, func(_, in string) string { return in }},
}

var b101Verbs = []string{"v", "+v", "#v", "s", "q", "x", "d"}

// TestB101_TgNested (AC3): below the top level the config prints its fields, secrets redacted, as at the top level.
func TestB101_TgNested(t *testing.T) {
	for _, cc := range b101Configs() {
		if cc.name == "zero" {
			continue
		}
		t.Run(cc.name, func(t *testing.T) {
			for _, sh := range b101Shapes {
				t.Run(sh.name, func(t *testing.T) {
					for _, v := range b101Verbs {
						t.Run(v, func(t *testing.T) {
							got := fmt.Sprintf("%"+v, sh.val(cc.c))
							assert.Equal(t, sh.want(v, fmt.Sprintf("%"+v, b101Plain(cc.c))), got)
							b101NoSecret(t, got)
						})
					}
				})
			}
		})
	}
}

// TestB101_TgEmbeddedValue: a config embedded by value prints only the config through a pointer to the outer value
// (the promoted method of *Config, selector table cell 0) and is unchanged as a value (a guard).
func TestB101_TgEmbeddedValue(t *testing.T) {
	full := b101Configs()[0].c
	for _, v := range b101Verbs {
		t.Run("pointer/"+v, func(t *testing.T) {
			bv := &b101ByValue{*full, "db"}
			got := fmt.Sprintf("%"+v, bv)
			assert.Equal(t, fmt.Sprintf("%"+v, b101Plain(&bv.Config)), got)
			b101NoSecret(t, got)
		})
		t.Run("value/"+v, func(t *testing.T) {
			got := fmt.Sprintf("%"+v, b101ByValue{*full, "db"})
			in, db := fmt.Sprintf("%"+v, b101PlainValue(*full)), fmt.Sprintf("%"+v, "db")
			want := "{" + in + " " + db + "}"
			switch v {
			case "#v":
				want = "tg.b101ByValue{Config:" + in + ", DB:" + db + "}"
			case "+v":
				want = "{Config:" + in + " DB:" + db + "}"
			}
			assert.Equal(t, want, got)
			b101NoSecret(t, got)
		})
	}
}

// TestB101_TgSlog: slog's text handler prints a struct that holds the config with its fields (before: the address);
// the *Config itself and both JSON lines are unchanged (guards).
func TestB101_TgSlog(t *testing.T) {
	for _, cc := range b101Configs() {
		if cc.name == "zero" {
			continue
		}
		t.Run(cc.name, func(t *testing.T) {
			t.Run("text_field", func(t *testing.T) {
				text, _ := b101Slog(b101Field{cc.c})
				want, _ := b101Slog("{C:" + fmt.Sprintf("%+v", b101Plain(cc.c)) + "}")
				assert.Equal(t, want, text)
				b101NoSecret(t, text)
			})
			t.Run("text_pointer", func(t *testing.T) {
				text, _ := b101Slog(cc.c)
				want, _ := b101Slog(b101Plain(cc.c))
				assert.Equal(t, want, text)
			})
			t.Run("json_field", func(t *testing.T) {
				type plainField struct{ C any }
				_, js := b101Slog(b101Field{cc.c})
				_, want := b101Slog(plainField{b101Plain(cc.c)})
				assert.Equal(t, want, js)
				b101NoSecret(t, js)
			})
			t.Run("json_pointer", func(t *testing.T) {
				_, js := b101Slog(cc.c)
				_, want := b101Slog(b101Plain(cc.c))
				assert.Equal(t, want, js)
			})
		})
	}
}

// TestB101_TgLiteral: exact texts written out, so the type name does not rest on the oracle alone.
func TestB101_TgLiteral(t *testing.T) {
	full := b101Configs()[0].c
	var nilc *Config
	t.Run("sharp_v", func(t *testing.T) {
		got := fmt.Sprintf("%#v", full)
		assert.True(t, strings.HasPrefix(got, "&tg.Config{"), got)
		assert.Contains(t, got, `Token:tg.SecretToken("[REDACTED]")`)
	})
	t.Run("nil_s", func(t *testing.T) { assert.Equal(t, "%!s(*tg.Config=<nil>)", fmt.Sprintf("%s", any(nilc))) })
	t.Run("T", func(t *testing.T) { assert.Equal(t, "*tg.Config", fmt.Sprintf("%T", full)) })
}

// TestB101_TgReadOnly: printing leaves the config as it was, full and zero; an in-place redaction would pass every
// output row. The %#v snapshot cannot see the secrets (they print
// redacted) or a func; the token, webhook_secret and callback rows read those.
func TestB101_TgReadOnly(t *testing.T) {
	for _, cc := range b101Configs()[:2] {
		t.Run(cc.name, func(t *testing.T) {
			c := cc.c
			tok, before := c.Token.Value(), fmt.Sprintf("%#v", b101Plain(c))
			for _, d := range b101AllDirectives() {
				_ = fmt.Sprintf(d, c)
			}
			for _, sh := range b101Shapes {
				for _, v := range b101Verbs {
					_ = fmt.Sprintf("%"+v, sh.val(c))
				}
			}
			t.Run("token", func(t *testing.T) { assert.Equal(t, tok, c.Token.Value()) })
			t.Run("fields", func(t *testing.T) { assert.Equal(t, before, fmt.Sprintf("%#v", b101Plain(c))) })
		})
	}
	t.Run("print_change_print", func(t *testing.T) {
		for _, d := range b101AllDirectives() { // every directive, so a memo behind one verb shows
			c := b101Configs()[0].c
			_ = fmt.Sprintf(d, c)
			c.BaseURL = "changed"
			assert.Equal(t, fmt.Sprintf(d, b101Plain(c)), fmt.Sprintf(d, c), "Format keeps no state (round t1 C-T1-01): %s", d)
		}
	})
}

// TestB101_TgConcurrent: goroutines that print a shared full and zero config with every directive while others read
// their tokens see the raw values and the same text each time. A Format that writes the config, even one that
// restores it, behind any verb, flag or config value, is a data race the -race gate reports here.
// %p is left out: it prints the address of the new slice. The expected %v text is checked against
// the oracle first (a config in a slice prints its fields).
func TestB101_TgConcurrent(t *testing.T) {
	c, z := b101Configs()[0].c, b101Configs()[1].c
	want := map[string]string{}
	for _, d := range b101AllDirectives() {
		if d != "%p" {
			want[d] = fmt.Sprintf(d, []*Config{c, z})
		}
	}
	assert.Equal(t, "["+fmt.Sprintf("%v", b101Plain(c))+" "+fmt.Sprintf("%v", b101Plain(z))+"]", want["%v"],
		"a config in a slice prints its fields")
	var wg sync.WaitGroup
	var mu sync.Mutex
	var bad []string
	for range 4 {
		wg.Go(func() {
			for range 10 {
				for d, w := range want {
					if got := fmt.Sprintf(d, []*Config{c, z}); got != w {
						mu.Lock()
						bad = append(bad, got)
						mu.Unlock()
					}
				}
			}
		})
		wg.Go(func() {
			for range 500 {
				if got := c.Token.Value() + "|" + z.Token.Value(); got != b101Token+"|" {
					mu.Lock()
					bad = append(bad, got)
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	assert.Empty(t, bad)
}

// TestB101_TgBools: each bool field set alone, printed with every directive. The full config sets every bool to true,
// so a Format that copies or swaps two bools prints the same text there. Only the tg
// Config has two or more bools.
func TestB101_TgBools(t *testing.T) {
	typ := reflect.TypeFor[Config]()
	for i := range typ.NumField() {
		if typ.Field(i).Type.Kind() != reflect.Bool {
			continue
		}
		t.Run(typ.Field(i).Name, func(t *testing.T) {
			var c Config
			reflect.ValueOf(&c).Elem().Field(i).SetBool(true)
			for _, dd := range b101Directives {
				assert.Equal(t, fmt.Sprintf(dd.d, b101Plain(&c)), fmt.Sprintf(dd.d, &c), dd.name)
			}
		})
	}
}

// b101App holds a config VALUE in a named field.
type b101App struct {
	Cfg  Config
	Name string
}

// TestB101_TgLimits pins the two shapes no method on *Config can reach (PLAN AC3, §9): fmt's bad-verb path on the
// user's own type turns methods off before Format can run, so the secret still prints there, as on the baseline.
func TestB101_TgLimits(t *testing.T) {
	full := b101Configs()[0].c
	t.Run("value_in_user_pointer_s", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(b101S, []*b101App{{*full, "a"}}), b101Token)
	})
	t.Run("value_in_struct_p", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(b101P, struct{ C Config }{*full}), b101Token)
	})
}

// TestB101_TgIndexException pins AC2's one named exception (PLAN AC2): a digit or * verb after an
// argument index when fmt holds no width other than 0 and no precision. Format prints %!(NOVERB), after fmt's own
// prefix, where the baseline printed bad-verb text. No secret prints either way.
func TestB101_TgIndexException(t *testing.T) {
	rows := []struct {
		name, d string
		w       any
		want    string
	}{
		{"zero_width_digit", "%*[2]0", 0, "%!(NOVERB)"},
		{"zero_width_star", "%*[2]*", 0, "%!(BADWIDTH)%!(NOVERB)"},
		{"bad_prec_digit", "%.*[2]5", -1, "%!(BADPREC)%!(NOVERB)"},
		{"non_int_prec_digit", "%.*[2]5", "x", "%!(BADPREC)%!(NOVERB)"},
	}
	for _, cc := range b101Configs() {
		t.Run(cc.name, func(t *testing.T) {
			for _, r := range rows {
				t.Run(r.name, func(t *testing.T) {
					got := fmt.Sprintf(r.d, r.w, cc.c)
					assert.Equal(t, r.want, got)
					assert.NotEqual(t, fmt.Sprintf(r.d, r.w, b101Plain(cc.c)), got, "the baseline text differs")
					b101NoSecret(t, got)
				})
			}
		})
	}
}

// b101S and b101P are variable formats: vet and staticcheck reject these directives in a constant one.
var b101S, b101P = "%s", "%p"

func b101Slog(v any) (text, json string) {
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

// b101NoSecret: the output holds neither fake secret, as text or in hex.
func b101NoSecret(t *testing.T, got string) {
	t.Helper()
	for _, s := range []string{b101Token, b101Secret} {
		assert.NotContains(t, got, s)
		assert.NotContains(t, got, fmt.Sprintf("%x", s))
		assert.NotContains(t, got, fmt.Sprintf("%X", s))
	}
}
