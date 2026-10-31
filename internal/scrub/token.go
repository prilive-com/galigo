// Package scrub provides security helpers for removing sensitive data from errors.
package scrub

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/prilive-com/galigo/tg"
)

// minPiece is the shortest run of token bytes that is redacted wherever it appears.
const minPiece = 4

const redacted = "[REDACTED]"

// TokenFromError removes the bot token from error messages.
// Go's http.Client.Do() includes the request URL (containing the token) in error strings.
// URL parsing can escape, decode or split the token, so every run of at least minPiece bytes
// that comes from the token is removed, not only the token as written.
// Preserves the error chain for errors.Is/As via Unwrap().
func TokenFromError(err error, token tg.SecretToken) error {
	if err == nil {
		return nil
	}
	tokenVal := token.Value()
	if tokenVal == "" {
		return err
	}
	msg := err.Error()
	if out := redact(msg, tokenVal); out != msg {
		return &scrubbedError{msg: out, err: err}
	}
	return err
}

// redact replaces every run of bytes in s that is covered by windows of n bytes (n = minPiece, or
// the token length if shorter) taken from the token's forms. Time and memory are linear.
func redact(s, tok string) string {
	n := min(minPiece, len(tok))
	if len(s) < n {
		return s
	}
	windows := make(map[string]struct{})
	for _, f := range tokenForms(tok) {
		for i := 0; i+n <= len(f); i++ {
			windows[f[i:i+n]] = struct{}{}
		}
	}
	covered := make([]bool, len(s))
	found := false
	for i := 0; i+n <= len(s); i++ {
		if _, ok := windows[s[i:i+n]]; ok {
			for j := i; j < i+n; j++ {
				covered[j] = true
			}
			found = true
		}
	}
	if !found {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if !covered[i] {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteString(redacted)
		for i < len(s) && covered[i] {
			i++
		}
	}
	return b.String()
}

// tokenForms returns the token as written and as a URL error can print it: decoded, and decoded
// then escaped again as a fragment, a path, userinfo or a host; plus the %q inner text of each.
func tokenForms(tok string) []string {
	var forms []string
	seen := make(map[string]bool)
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		forms = append(forms, s)
		q := strconv.Quote(s)
		if inner := q[1 : len(q)-1]; !seen[inner] {
			seen[inner] = true
			forms = append(forms, inner)
		}
	}
	add(tok)
	dec := unescapeValid(tok)
	add(dec)
	add((&url.URL{Fragment: dec}).EscapedFragment())
	add((&url.URL{Path: dec}).EscapedPath())
	add(url.User(dec).String())
	add(strings.TrimPrefix((&url.URL{Host: dec}).String(), "//"))
	return forms
}

// unescapeValid decodes each valid %XX sequence and keeps every other byte, so one bad escape
// (which url.Parse rejects but a raw query keeps) does not stop the rest from being decoded.
func unescapeValid(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }
