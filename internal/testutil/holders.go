package testutil

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// holderVerbs: every ASCII rune that fmt does not read as directive syntax (so fmt reads it as the verb), plus a
// rune below 256 and a rune above 255 that are not ASCII. %T and %p are left out because fmt handles them before
// any method, and %w because it is a documented limit (its bad-verb text prints the struct). Built on first use, so
// a test binary that imports this package and does not call AssertFormatsAs pays nothing.
var holderVerbs = sync.OnceValue(func() []rune {
	var out []rune
	for r := range rune(0x80) {
		if !strings.ContainsRune("%#+- 0123456789.*[Tpw", r) {
			out = append(out, r)
		}
	}
	return append(out, 'é', '界')
})

// holderForm is a width or a precision: its directive text and, for *, the argument fmt reads for it.
type holderForm struct {
	text string
	arg  any
}

// holderWidths: none, shorter and longer than the texts, and from an argument (*): a negative width (it sets the
// '-' flag) and 0. holderPrecisions: none, a bare '.', 0, shorter and longer than the texts, and from an argument:
// 3, 0 and -1 (fmt writes %!(BADPREC) and formats with no precision).
var (
	holderWidths     = []holderForm{{"", nil}, {"7", nil}, {"40", nil}, {"*", -40}, {"*", 0}}
	holderPrecisions = []holderForm{{"", nil}, {".", nil}, {".0", nil}, {".3", nil}, {".40", nil}, {".*", 3}, {".*", 0}, {".*", -1}}
)

// holderDirective is one directive AssertFormatsAs runs: d for the holder, want for the fixed text, and the
// arguments fmt reads for * before the value.
type holderDirective struct {
	d, want string
	pre     []any
}

// holderDirectives: every verb in holderVerbs with every subset of the flags "+-# 0" and every width and precision
// above. For %#v (any flags, width and precision) the text is formatted with %s and the same flags. Built on first
// use.
var holderDirectives = sync.OnceValue(func() []holderDirective {
	const flags = "+-# 0"
	var out []holderDirective
	for _, verb := range holderVerbs() {
		for mask := range 1 << len(flags) {
			var fl strings.Builder
			for i := range len(flags) {
				if mask&(1<<i) != 0 {
					fl.WriteByte(flags[i])
				}
			}
			for _, wd := range holderWidths {
				for _, pr := range holderPrecisions {
					hd := holderDirective{d: "%" + fl.String() + wd.text + pr.text + string(verb)}
					hd.want = hd.d
					if verb == 'v' && strings.Contains(fl.String(), "#") {
						hd.want = "%" + fl.String() + wd.text + pr.text + "s"
					}
					for _, a := range []any{wd.arg, pr.arg} {
						if a != nil {
							hd.pre = append(hd.pre, a)
						}
					}
					out = append(out, hd)
				}
			}
		}
	}
	return out
})

// The number of verbs and directives TEST-PLAN §2 states: 107 ASCII verbs plus 2, and 109 × 32 flag sets × 5 widths ×
// 8 precisions.
const (
	holderVerbCount      = 109
	holderDirectiveCount = 139520
)

// AssertFormatsAs checks that v prints exactly like the string text for every directive in holderDirectives: the
// output of a Format method that hides state behind a fixed text. held are the secrets v holds: each must be in one
// of v's string fields (found by reflection, unexported fields included) before printing. Printing must not change
// v's state (heldFields): it is the same after one %v, and after all the directives, as before. Four goroutines
// also print v at the same time, so a Format that writes v's state and then restores it is a data race under -race.
func AssertFormatsAs(t *testing.T, v any, text string, held ...string) {
	t.Helper()
	_ = time.Local.String() // time.Local fills its fields on first use; a breaker's start time points to it
	before := heldFields(v)
	for _, h := range held {
		if !slices.ContainsFunc(before, func(f heldField) bool { return f.value == "s:"+h }) {
			t.Errorf("no string field of %T holds the secret %q", v, h)
		}
	}
	_ = fmt.Sprintf("%v", v)
	assertSameState(t, v, before, "one %v")
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { _ = fmt.Sprint(v) })
	}
	wg.Wait()
	verbs, directives := holderVerbs(), holderDirectives()
	if len(verbs) != holderVerbCount || len(directives) != holderDirectiveCount {
		t.Errorf("%d verbs and %d directives, want %d and %d", len(verbs), len(directives), holderVerbCount, holderDirectiveCount)
	}
	bad := 0
	for _, hd := range directives {
		got := fmt.Sprintf(hd.d, append(slices.Clone(hd.pre), v)...)
		want := fmt.Sprintf(hd.want, append(slices.Clone(hd.pre), text)...)
		if got != want {
			bad++
			if bad <= 10 {
				t.Errorf("directive %q: got %q, want %q", hd.d, got, want)
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d directives print other text than %q", bad, len(directives), text)
	}
	assertSameState(t, v, before, "all directives")
}

// assertSameState reports the first field whose state differs from before. It prints paths, not values.
func assertSameState(t *testing.T, v any, before []heldField, when string) {
	t.Helper()
	after := heldFields(v)
	if slices.Equal(before, after) {
		return
	}
	for i := range max(len(before), len(after)) {
		var b, a heldField
		if i < len(before) {
			b = before[i]
		}
		if i < len(after) {
			a = after[i]
		}
		if a != b {
			t.Errorf("after %s, the state of %T changed (%d fields before, %d after); first difference at %q (before) / %q (after)",
				when, v, len(before), len(after), b.path, a.path)
			return
		}
	}
}

// heldField is one piece of state reachable from a value, and the path of fields that reached it. value starts
// with its kind: "s:" a string, "p:" the address of a pointer, channel, map, slice or func (or "nil"), "t:" the
// dynamic type in an interface, "n:" a bool or number.
type heldField struct {
	path, value string
}

var (
	mutexType   = reflect.TypeFor[sync.Mutex]()
	rwMutexType = reflect.TypeFor[sync.RWMutex]()
)

// heldFields returns the state reachable from v, in walk order.
func heldFields(v any) []heldField {
	var out []heldField
	walkHeld(reflect.ValueOf(v), "", map[uintptr]bool{}, &out)
	return out
}

// walkHeld appends the state reachable from v through pointers, interfaces and struct fields (exported or not;
// reflect reads an unexported field with String, Int, Uint, Float, Bool and Pointer). A pointer seen before is
// recorded but not followed again. The elements of maps, slices and channels are not followed (their addresses are
// recorded), and a sync.Mutex or sync.RWMutex is skipped (another goroutine may hold it for a moment).
func walkHeld(v reflect.Value, path string, seen map[uintptr]bool, out *[]heldField) {
	add := func(val string) { *out = append(*out, heldField{path, val}) }
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			add("p:nil")
			return
		}
		add("p:" + strconv.FormatUint(uint64(v.Pointer()), 16))
		if seen[v.Pointer()] {
			return
		}
		seen[v.Pointer()] = true
		walkHeld(v.Elem(), path+"*", seen, out)
	case reflect.Chan, reflect.Map, reflect.Slice, reflect.Func, reflect.UnsafePointer:
		if v.IsNil() {
			add("p:nil")
		} else {
			add("p:" + strconv.FormatUint(uint64(v.Pointer()), 16))
		}
	case reflect.Interface:
		if v.IsNil() {
			add("t:nil")
			return
		}
		add("t:" + v.Elem().Type().String())
		walkHeld(v.Elem(), path, seen, out)
	case reflect.Struct:
		if v.Type() == mutexType || v.Type() == rwMutexType {
			return
		}
		for i := range v.NumField() {
			walkHeld(v.Field(i), path+"."+v.Type().Field(i).Name, seen, out)
		}
	case reflect.String:
		add("s:" + v.String())
	case reflect.Bool:
		add("n:" + strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		add("n:" + strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		add("n:" + strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		add("n:" + strconv.FormatFloat(v.Float(), 'g', -1, 64))
	default: // Array, Complex: no galigo holder keeps state there
	}
}

// SlogLines returns what slog's text and JSON handlers write for one record with the attribute key=v, without the
// time attribute.
func SlogLines(key string, v any) (text, json string) {
	noTime := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}
	var tb, jb bytes.Buffer
	slog.New(slog.NewTextHandler(&tb, &slog.HandlerOptions{ReplaceAttr: noTime})).Info("m", key, v)
	slog.New(slog.NewJSONHandler(&jb, &slog.HandlerOptions{ReplaceAttr: noTime})).Info("m", key, v)
	return tb.String(), jb.String()
}
