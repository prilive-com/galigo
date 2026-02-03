// Package scrub provides security helpers for removing sensitive data from errors.
package scrub

import (
	"strings"

	"github.com/prilive-com/galigo/tg"
)

// TokenFromError removes the bot token from error messages.
// Go's http.Client.Do() includes the request URL (containing the token) in error strings.
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
	if strings.Contains(msg, tokenVal) {
		return &scrubbedError{
			msg: strings.ReplaceAll(msg, tokenVal, "[REDACTED]"),
			err: err,
		}
	}
	return err
}

type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }
