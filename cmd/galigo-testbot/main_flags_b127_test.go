package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestB127_NoSkipInteractiveFlag pins B-127: the testbot has no
// --skip-interactive flag (it was accepted and never read).
func TestB127_NoSkipInteractiveFlag(t *testing.T) {
	assert.Nil(t, flag.Lookup("skip-interactive"))
}

// TestB127_FlagSet pins the testbot's own flags: exactly run and status.
//
// It sees every flag registered on flag.CommandLine at package init
// (VisitAll visits them in name order). Flags whose names start with "test."
// are left out, because go test adds its own flags with that prefix.
// Limits: a flag registered on another FlagSet, inside main, or named
// "test.…" is not seen here. One registered inside main before flag.Parse,
// or named "test.…", shows in the built binary's -h output, which CI does
// not check. A custom flag.Usage can hide a flag from -h, not from this
// test. A future test import that registers its own flag would make this
// test fail with no product change.
func TestB127_FlagSet(t *testing.T) {
	var names []string
	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		if !strings.HasPrefix(f.Name, "test.") {
			names = append(names, f.Name)
		}
	})
	assert.Equal(t, []string{"run", "status"}, names)
}
