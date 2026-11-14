package scrub

import "fmt"

// FormatHolder is the body of the Format method of a public type that holds a secret (the bot token or the
// webhook secret) in an unexported field, where tg.SecretToken's own methods are never called. It writes
// name+"{[REDACTED]}", or "<nil>" when isNil is true, with the flags, width and precision of the directive;
// %#v writes the same text without quotes. As in tg.SecretToken's Format, an argument index just before the verb
// is not passed on, so a verb that is directive syntax there prints fmt's %!(NOVERB) text. fmt handles %T and %p
// before Format.
func FormatHolder(f fmt.State, verb rune, name string, isNil bool) {
	text := name + "{[REDACTED]}"
	if isNil {
		text = "<nil>"
	}
	if verb == 'v' && f.Flag('#') {
		verb = 's'
	}
	fmt.Fprintf(f, fmt.FormatString(f, verb), text)
}
