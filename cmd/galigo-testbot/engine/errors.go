package engine

import "errors"

// SkipError indicates a scenario was skipped due to missing prerequisites.
type SkipError struct {
	Reason string
}

func (e SkipError) Error() string {
	return "skipped: " + e.Reason
}

// Skip returns a SkipError with the given reason.
func Skip(reason string) error {
	return SkipError{Reason: reason}
}

// IsSkip returns true if err is a SkipError.
func IsSkip(err error) bool {
	var skipErr SkipError
	return errors.As(err, &skipErr)
}
