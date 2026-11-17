//go:build race

package sender

// The race build: b100ValueDirectives returns the named list (B-100 test file payments_format_b100_test.go).
func init() { b100Race = true }
