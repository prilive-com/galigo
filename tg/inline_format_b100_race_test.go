//go:build race

package tg

// The race build: b100ValueDirectives returns the named list (B-100 test file inline_format_b100_test.go).
func init() { b100Race = true }
