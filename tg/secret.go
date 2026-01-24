package tg

import "log/slog"

// SecretToken wraps a bot token to prevent accidental logging.
// Implements fmt.Stringer, fmt.GoStringer, slog.LogValuer, and encoding.TextMarshaler.
type SecretToken string

// Value returns the actual token value.
// Only use this when sending to Telegram API.
func (s SecretToken) Value() string { return string(s) }

// String returns a redacted placeholder (fmt.Stringer).
func (s SecretToken) String() string { return "[REDACTED]" }

// GoString returns redacted for %#v (fmt.GoStringer).
func (s SecretToken) GoString() string { return `tg.SecretToken("[REDACTED]")` }

// LogValue returns a redacted value for slog (slog.LogValuer).
// This ensures the token is never logged even with %+v.
func (s SecretToken) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

// MarshalText returns redacted bytes (encoding.TextMarshaler).
// Prevents accidental JSON/YAML serialization of the token.
func (s SecretToken) MarshalText() ([]byte, error) {
	return []byte("[REDACTED]"), nil
}

// IsEmpty returns true if the token is empty.
func (s SecretToken) IsEmpty() bool {
	return s == ""
}
