package tg

import (
	"fmt"
	"log/slog"
)

// SecretToken wraps a bot token to prevent accidental logging.
// Implements fmt.Formatter, fmt.Stringer, fmt.GoStringer, slog.LogValuer, and encoding.TextMarshaler.
//
// fmt (for the verbs it passes to these methods), slog, and encoders that call MarshalText (encoding/json for
// values, and encoding/xml for elements, attributes, chardata and cdata) write [REDACTED] in place of a SecretToken
// value. These paths call none of those methods, so they print the real value:
//   - a value reached through an unexported struct field, at any depth (fmt, and slog's text handler, which
//     formats such a struct with fmt);
//   - fmt's bad-verb text %!verb(type=value), which prints its value without methods at any depth: %w of a value
//     that holds the token (an error too, in every function but fmt.Errorf), %p of such a value that is not a
//     pointer, map, slice, channel or func, and a pointer below the top level (a struct field, a slice, array or
//     map element, an interface value) printed with a verb other than v, p, b, o, d, x and X, when the pointed-to
//     type has no method fmt uses for that verb (for example %s of a []*T where T holds the token);
//   - encoder paths that read the string itself: a SecretToken map key in encoding/json (and slog's JSON handler),
//     and a *SecretToken map key depending on the Go version; in encoding/xml, a ",comment" field, a ",chardata"
//     or ",cdata" field of type **SecretToken, and, depending on the Go version, a chardata, cdata or attribute
//     field of interface type; encodings that never call MarshalText, such as encoding/gob;
//   - other code that reads the value by reflection without calling these methods, such as go-spew with methods
//     disabled (testify's failure diff).
//
// go vet does not report %p (or any verb but %w) on a SecretToken, because it implements fmt.Formatter.
// Call Value() to use or store the token.
type SecretToken string

// Value returns the actual token value.
// Only use this when sending to Telegram API.
func (s SecretToken) Value() string { return string(s) }

// String returns a redacted placeholder (fmt.Stringer).
func (s SecretToken) String() string { return "[REDACTED]" }

// GoString returns redacted for %#v (fmt.GoStringer).
func (s SecretToken) GoString() string { return `tg.SecretToken("[REDACTED]")` }

// LogValue returns a redacted value for slog (slog.LogValuer). The cases where slog prints the real value are in
// the SecretToken doc.
func (s SecretToken) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

// MarshalText returns redacted bytes (encoding.TextMarshaler), so encoders that call it (encoding/json for values,
// and encoding/xml for elements, attributes, chardata and cdata) write [REDACTED] for a SecretToken value. The cases
// they do not cover are in the SecretToken doc.
func (s SecretToken) MarshalText() ([]byte, error) {
	return []byte("[REDACTED]"), nil
}

// Format writes the redacted text for the verbs fmt passes to it (fmt.Formatter), so a verb that does not fit a
// string, such as %d, does not print the token. %#v writes GoString; every other verb formats "[REDACTED]" with
// the same flags, width and precision as fmt would format that string. An argument index just before the verb is
// not passed on, so a verb that is directive syntax there (a flag character with no width or precision before the
// index, a digit after a width or precision, or '[') prints fmt's %!(NOVERB) text instead. A type that embeds
// SecretToken gets this method by Go's promotion rules, and fmt uses it before that type's own String, Error or
// GoString; an embedded nil *SecretToken prints fmt's PANIC text. This Format and another promoted field or method
// named Format at the same depth cancel each other, so such a type does not implement fmt.Formatter and its Format
// selector does not compile; a field or method named Format at a greater depth is hidden by this one. The paths fmt
// never sends here are in the SecretToken doc.
func (s SecretToken) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		fmt.Fprintf(f, fmt.FormatString(f, 's'), s.GoString())
		return
	}
	fmt.Fprintf(f, fmt.FormatString(f, verb), "[REDACTED]")
}

// IsEmpty returns true if the token is empty.
func (s SecretToken) IsEmpty() bool {
	return s == ""
}
