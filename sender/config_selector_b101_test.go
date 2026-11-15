package sender

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The selector-table cells of PLAN §9 that compile and change or keep the output, for *sender.Config (the same method
// set rules apply to the receiver and tg configs). Cell 0 and the embedded value are in TestB101_SenderNested and
// TestB101_SenderEmbeddedValue. The calls and assignments that stop compiling (cells 3, 3b, 4, 6, 7, and by value 3, 3b,
// 4, 5, 6, 7) are probes: E36/lookups/selector-table.txt, selector-byvalue.txt and selector-byvalue2.txt.

type b101Custom struct{}

func (b101Custom) Format(f fmt.State, _ rune) { fmt.Fprint(f, "CUSTOM") }

type b101OwnString struct{ *Config }

func (b101OwnString) String() string { return "OWN-STRING" }

type b101OwnError struct{ *Config }

func (b101OwnError) Error() string { return "OWN-ERROR" }

type b101OwnGoString struct{ *Config }

func (b101OwnGoString) GoString() string { return "OWN-GOSTRING" }

type b101OwnFormat struct{ *Config }

func (b101OwnFormat) Format(f fmt.State, _ rune) { fmt.Fprint(f, "OUTER") }

type b101OwnOtherSig struct{ *Config }

func (b101OwnOtherSig) Format(string) string { return "X" }

// b101FieldF has a field named Format (cells 4, 4b, 7).
type b101FieldF struct{ Format int }

type b101SameDepthField struct {
	*Config
	b101FieldF
}

type b101ShallowField struct {
	*Config
	Format int
}

type b101WrapField struct{ b101FieldF }

type b101DeeperField struct {
	*Config
	b101WrapField
}

type b101SameDepth struct {
	*Config
	b101Custom
}

type b101SameDepthTime struct {
	*Config
	time.Time
}

type b101Mid struct{ b101Custom }

type b101Deeper struct {
	*Config
	b101Mid
}

type b101TimeMid struct{ time.Time }

type b101DeeperTime struct {
	*Config
	b101TimeMid
}

// The same cells with the config embedded BY VALUE (PLAN §9 by-value list, round p2 C-P2-02).
type b101OwnStringV struct{ Config }

func (b101OwnStringV) String() string { return "OWN-STRING" }

// b101OwnErrorV is the usual error type: a config value with a pointer-receiver Error.
type b101OwnErrorV struct{ Config }

func (*b101OwnErrorV) Error() string { return "OWN-ERROR" }

type b101OwnGoStringV struct{ Config }

func (*b101OwnGoStringV) GoString() string { return "OWN-GOSTRING" }

type b101DeeperFieldV struct {
	Config
	b101WrapField
}

type b101SameDepthV struct {
	Config
	b101Custom
}

type b101DeeperV struct {
	Config
	b101Mid
}

type b101DeeperTimeV struct {
	Config
	b101TimeMid
}

var errB101Sentinel = errors.New("sentinel")

func (b101OwnError) Is(target error) bool { return target == errB101Sentinel }

func TestB101_SenderSelector(t *testing.T) {
	c := b101Configs()[0].c
	in := fmt.Sprintf("%v", b101Plain(c))
	t.Run("cell1_own_string", func(t *testing.T) {
		assert.Equal(t, in, fmt.Sprintf("%v", b101OwnString{c}), "the promoted Format wins over the outer String")
	})
	t.Run("cell1b_own_error", func(t *testing.T) {
		e := b101OwnError{c}
		assert.Equal(t, in, fmt.Sprintf("%v", e), "the promoted Format wins over the outer Error")
		assert.Equal(t, "wrap: "+in, fmt.Errorf("wrap: %w", e).Error(), "so a wrapped message changes")
		assert.Equal(t, "OWN-ERROR", e.Error(), "Error itself is unchanged")
		assert.ErrorIs(t, fmt.Errorf("wrap: %w", e), errB101Sentinel, "errors.Is is unchanged")
	})
	t.Run("cell1c_own_gostring", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%#v", b101Plain(c)), fmt.Sprintf("%#v", b101OwnGoString{c}),
			"the promoted Format wins over the outer GoString")
	})
	t.Run("cell2_own_format", func(t *testing.T) {
		assert.Equal(t, "OUTER", fmt.Sprintf("%v", b101OwnFormat{c}), "the outer Format is unchanged")
	})
	t.Run("cell2b_own_other_sig", func(t *testing.T) {
		v := b101OwnOtherSig{c}
		assert.Equal(t, "{"+in+"}", fmt.Sprintf("%v", v), "the outer Format of another signature hides the promoted one")
		assert.Equal(t, "X", v.Format(""), "control: the outer method is still called by name")
	})
	t.Run("cell4_same_depth_field", func(t *testing.T) {
		v := b101SameDepthField{Config: c}
		assert.Equal(t, "{"+in+" {0}}", fmt.Sprintf("%v", v), "the field and the method cancel; the config part prints its fields")
		assert.Equal(t, 0, v.b101FieldF.Format, "control: the field exists")
	})
	t.Run("cell4b_shallow_field", func(t *testing.T) {
		v := b101ShallowField{Config: c}
		assert.Equal(t, "{"+in+" 0}", fmt.Sprintf("%v", v), "the field hides the method; the config part prints its fields")
		assert.Equal(t, 0, v.Format, "control: the field is reached")
	})
	t.Run("cell7_deeper_field", func(t *testing.T) {
		v := b101DeeperField{Config: c}
		assert.Equal(t, in, fmt.Sprintf("%v", v), "a deeper field is hidden: only the config's fields print")
		assert.Equal(t, 0, v.b101WrapField.Format, "control: the field exists")
	})
	t.Run("cell3_same_depth", func(t *testing.T) {
		v := b101SameDepth{Config: c}
		_, isFormatter := any(v).(fmt.Formatter)
		assert.False(t, isFormatter, "two Format methods at the same depth cancel")
		assert.Equal(t, "{"+in+" {}}", fmt.Sprintf("%v", v), "the fields print, the config part as fields")
		assert.Equal(t, "CUSTOM", fmt.Sprintf("%v", v.b101Custom), "control: the cancelled Format exists")
	})
	t.Run("cell3b_same_depth_time", func(t *testing.T) {
		v := b101SameDepthTime{Config: c}
		assert.Equal(t, v.Time.String(), fmt.Sprintf("%v", v), "output unchanged: time's String is still promoted")
	})
	t.Run("cell5_deeper", func(t *testing.T) {
		assert.Equal(t, in, fmt.Sprintf("%v", b101Deeper{Config: c}), "a deeper Format is hidden")
		assert.Equal(t, "CUSTOM", fmt.Sprintf("%v", b101Deeper{}.b101Mid), "control: the hidden Format exists")
	})
	t.Run("cell6_deeper_time", func(t *testing.T) {
		v := b101DeeperTime{Config: c}
		assert.Equal(t, in, fmt.Sprintf("%v", v), "the config's fields print instead of time's text")
		assert.Equal(t, v.Time.String(), fmt.Sprintf("%v", v.b101TimeMid), "control: time's String exists")
	})
	inV := fmt.Sprintf("%v", b101PlainValue(*c))
	t.Run("byvalue_cell1_own_string", func(t *testing.T) {
		assert.Equal(t, "OWN-STRING", fmt.Sprintf("%v", b101OwnStringV{*c}), "as a value: unchanged")
		assert.Equal(t, in, fmt.Sprintf("%v", &b101OwnStringV{*c}), "through a pointer: the config's fields")
	})
	t.Run("byvalue_cell1b_own_error", func(t *testing.T) {
		e := &b101OwnErrorV{*c}
		assert.Equal(t, "OWN-ERROR", e.Error(), "Error itself is unchanged")
		assert.Equal(t, "{"+inV+"}", fmt.Sprintf("%v", b101OwnErrorV{*c}), "as a value: unchanged")
		assert.Equal(t, in, fmt.Sprintf("%v", e), "through a pointer the promoted Format wins over Error")
		assert.Equal(t, "wrap: "+in, fmt.Errorf("wrap: %w", e).Error(), "so a wrapped message changes")
	})
	t.Run("byvalue_cell1c_own_gostring", func(t *testing.T) {
		v := b101OwnGoStringV{*c}
		assert.Equal(t, fmt.Sprintf("%#v", b101Plain(c)), fmt.Sprintf("%#v", &v), "through a pointer the promoted Format wins over GoString")
		assert.Equal(t, "OWN-GOSTRING", v.GoString(), "GoString itself is unchanged")
	})
	t.Run("byvalue_cell7_deeper_field", func(t *testing.T) {
		v := b101DeeperFieldV{Config: *c}
		assert.Equal(t, "{"+inV+" {{0}}}", fmt.Sprintf("%v", v), "as a value: unchanged")
		assert.Equal(t, in, fmt.Sprintf("%v", &v), "through a pointer: the config's fields")
		assert.Equal(t, 0, v.b101WrapField.Format, "control: the field exists")
	})
	t.Run("byvalue_cell3_same_depth", func(t *testing.T) {
		v := b101SameDepthV{Config: *c}
		_, isFormatter := any(v).(fmt.Formatter)
		assert.False(t, isFormatter, "the pointer method cancels the value's Format too")
		assert.Equal(t, "{"+inV+" {}}", fmt.Sprintf("%v", v), "as a value: the fields")
		assert.Equal(t, "&{"+inV+" {}}", fmt.Sprintf("%v", &v), "through a pointer: the fields")
		assert.Equal(t, "CUSTOM", fmt.Sprintf("%v", v.b101Custom), "control: the cancelled Format exists")
	})
	t.Run("byvalue_cell5_deeper", func(t *testing.T) {
		v := b101DeeperV{Config: *c}
		_, isFormatter := any(v).(fmt.Formatter)
		assert.False(t, isFormatter, "the pointer method hides the value's deeper Format")
		assert.Equal(t, "{"+inV+" {{}}}", fmt.Sprintf("%v", v), "as a value: the fields")
		assert.Equal(t, in, fmt.Sprintf("%v", &v), "through a pointer: the config's fields")
		assert.Equal(t, "CUSTOM", fmt.Sprintf("%v", v.b101Mid), "control: the hidden Format exists")
	})
	t.Run("byvalue_cell6_deeper_time", func(t *testing.T) {
		v := b101DeeperTimeV{Config: *c}
		assert.Equal(t, v.Time.String(), fmt.Sprintf("%v", v), "as a value: unchanged")
		assert.Equal(t, in, fmt.Sprintf("%v", &v), "through a pointer: the config's fields")
	})
}
