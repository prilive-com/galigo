package sender

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The user types of PLAN §9's selector table: each embeds a SendInvoiceRequest by value, unless named. The rules do not
// depend on which of the three types: they have the same method set (TestB100_SenderFormatter, tg's twin). Each embeds
// the type by its own name: through an alias such as `type r = SendInvoiceRequest` the field name is the alias, and fmt
// calls no method on an unexported field it walks, which it does when the outer type does not promote Format (PLAN AC6;
// a promoting alias embedding redacts, b100AliasPromotes).

type b100C1 struct{ SendInvoiceRequest }

func (b100C1) String() string { return "OWN-STRING" }

type b100C1b struct{ SendInvoiceRequest }

func (*b100C1b) Error() string { return "OWN-ERROR" }

type b100C1c struct{ SendInvoiceRequest }

func (b100C1c) GoString() string { return "OWN-GOSTRING" }

type b100C2 struct{ SendInvoiceRequest }

func (b100C2) Format(f fmt.State, _ rune) { fmt.Fprint(f, "OUTER") }

type b100Custom struct{} //nolint:unused // cells c3 and c5: the request's Format cancels or hides this one

func (b100Custom) Format(f fmt.State, _ rune) { fmt.Fprint(f, "CUSTOM") } //nolint:unused // see b100Custom

type b100C3 struct { // a same-depth Format method: they cancel
	SendInvoiceRequest
	b100Custom //nolint:unused // see b100Custom
}

type b100C3b struct { // a same-depth Format(string) string: they cancel, time.Time's String is promoted
	SendInvoiceRequest
	time.Time
}

type b100C4 struct { // a shallower field Format: it hides the method
	SendInvoiceRequest
	Format int
}

type b100Deep struct{ b100Custom } //nolint:unused // see b100Custom

type b100C5 struct { // a deeper Format method: the request's wins
	SendInvoiceRequest
	b100Deep //nolint:unused // see b100Custom
}

type b100DeepT struct{ time.Time }

type b100C6 struct { // a deeper time.Time: the request's Format wins
	SendInvoiceRequest
	b100DeepT
}

type b100Leaf struct{ Format int }

type b100C7 struct { // a same-depth field Format: they cancel
	SendInvoiceRequest
	b100Leaf
}

type b100Deep2 struct{ b100Leaf }

type b100C7b struct { // a field Format two levels down: hidden by the request's method
	SendInvoiceRequest
	b100Deep2
}

type b100C8 struct{ Inner SendInvoiceRequest } // a named field

type b100Alias = SendInvoiceRequest

type b100C4Alias struct { // c4, with the request embedded through an alias: the field name b100Alias is unexported
	b100Alias
	Format int
}

type b100AliasPromotes struct{ b100Alias } // c0 through an alias: Format is promoted, the field name does not matter

// TestB100_SenderSelector: PLAN §9's cells, exact texts from the oracle. Compile cells are probes only (TEST-PLAN §7).
func TestB100_SenderSelector(t *testing.T) {
	r := b100Values(b100Send)[0].v
	p := b100PlainSend(r)
	in := func(verb string) string { return fmt.Sprintf(verb, p) }
	formatter := reflect.TypeFor[fmt.Formatter]()
	only := func(t *testing.T, x any) { // the outer type prints only the request
		t.Helper()
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%d"} {
			got := fmt.Sprintf(verb, x)
			assert.Equal(t, in(verb), got, verb)
			b100NoToken(t, got)
		}
	}
	t.Run("c0_value", func(t *testing.T) { only(t, b100SendEmbedV{r, "app"}) })
	t.Run("c0_value_ptr", func(t *testing.T) { only(t, &b100SendEmbedV{r, "app"}) })
	t.Run("c0_pointer", func(t *testing.T) { only(t, b100SendEmbedP{&r, "app"}) })
	t.Run("c0_nil_pointer", func(t *testing.T) {
		assert.Equal(t, "%!v(PANIC=Format method: runtime error: invalid memory address or nil pointer dereference)",
			fmt.Sprintf("%v", b100SendEmbedP{nil, "app"}))
	})
	t.Run("c1_own_string", func(t *testing.T) {
		only(t, b100C1{r})
		assert.Equal(t, "OWN-STRING", b100C1{r}.String())
	})
	t.Run("c1b_own_error", func(t *testing.T) {
		e := &b100C1b{r}
		only(t, e)
		assert.Equal(t, "wrap: "+in("%v"), fmt.Errorf("wrap: %w", e).Error())
		assert.Equal(t, "OWN-ERROR", e.Error())
		assert.True(t, errors.Is(fmt.Errorf("w: %w", e), e))
		// fmt.Errorf's %w of an error is %v, so it redacts; any other function's %w is the bad-verb path (PLAN AC6).
		b100NoToken(t, fmt.Errorf(b100W, e).Error())
		assert.Contains(t, fmt.Sprintf(b100W, e), b100Token)
	})
	t.Run("c1c_own_gostring", func(t *testing.T) { only(t, b100C1c{r}) })
	t.Run("c0_alias_promotes", func(t *testing.T) { only(t, b100AliasPromotes{r}) })
	t.Run("c2_own_format", func(t *testing.T) {
		assert.Equal(t, "OUTER OUTER", fmt.Sprintf("%v %#v", b100C2{r}, &b100C2{r}))
	})
	t.Run("c3_same_depth_format", func(t *testing.T) {
		assert.False(t, reflect.TypeFor[b100C3]().Implements(formatter))
		assert.Equal(t, "{"+in("%v")+" {}}", fmt.Sprintf("%v", b100C3{SendInvoiceRequest: r}))
		assert.Equal(t, "&{"+in("%v")+" {}}", fmt.Sprintf("%v", &b100C3{SendInvoiceRequest: r}))
	})
	t.Run("c3b_same_depth_time", func(t *testing.T) {
		assert.Equal(t, "0001-01-01 00:00:00 +0000 UTC", fmt.Sprintf("%v", b100C3b{SendInvoiceRequest: r}))
	})
	t.Run("c4_shallow_field", func(t *testing.T) {
		assert.Equal(t, "{"+in("%v")+" 0}", fmt.Sprintf("%v", b100C4{SendInvoiceRequest: r}))
		assert.Equal(t, "&{"+in("%v")+" 0}", fmt.Sprintf("%v", &b100C4{SendInvoiceRequest: r}))
	})
	t.Run("c5_deeper_format", func(t *testing.T) { only(t, b100C5{SendInvoiceRequest: r}) })
	t.Run("c6_deeper_time", func(t *testing.T) { only(t, b100C6{SendInvoiceRequest: r}) })
	t.Run("c7_same_depth_field", func(t *testing.T) {
		assert.Equal(t, "{"+in("%v")+" {0}}", fmt.Sprintf("%v", b100C7{SendInvoiceRequest: r}))
		assert.Equal(t, "&{"+in("%v")+" {0}}", fmt.Sprintf("%v", &b100C7{SendInvoiceRequest: r}))
	})
	t.Run("c7b_deeper_field", func(t *testing.T) { only(t, b100C7b{SendInvoiceRequest: r}) })
	t.Run("c8_named_field", func(t *testing.T) {
		assert.Equal(t, "{"+in("%v")+"}", fmt.Sprintf("%v", b100C8{r}))
		assert.Equal(t, "&{"+in("%v")+"}", fmt.Sprintf("%v", &b100C8{r}))
	})
	// PLAN AC6: through a pointer to a type that does not promote Format, below the top level, a bad verb prints the
	// token (fmt turns methods off). c3 printed CUSTOM there before B-100: the one new leak, named in the docs.
	t.Run("limit_hidden_c4", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(b100S, []*b100C4{{SendInvoiceRequest: r}}), b100Token)
	})
	t.Run("limit_cancelled_c3", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(b100S, []*b100C3{{SendInvoiceRequest: r}}), b100Token)
	})
	t.Run("limit_c7", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf(b100S, []*b100C7{{SendInvoiceRequest: r}}), b100Token)
	})
	// c4 through an alias: Format is hidden, fmt walks the fields, and on the unexported field name b100Alias it calls
	// no method (PLAN AC6). The promoting alias redacts: c0_alias_promotes.
	t.Run("limit_alias_unexported", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf("%v", b100C4Alias{b100Alias: r}), b100Token)
	})
}
