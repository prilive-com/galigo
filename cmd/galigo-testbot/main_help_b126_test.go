package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// b126InteractiveNames reads the names isInteractiveSuite accepts from main.go. It knows only the form the function
// has today, `func isInteractiveSuite(suite string) bool` with `s := strings.ToLower(suite)` (the standard library
// "strings", imported without a name) then `return s == "a" || s == "b" ...`, and fails on any other form, so a new
// form (a constant, a map, a switch, another strings package) makes the test fail until the reader learns it (B-126,
// registers O-P2-01, O-C1-03).
func b126InteractiveNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)
	var fd *ast.FuncDecl
	for _, d := range f.Decls {
		if x, ok := d.(*ast.FuncDecl); ok && x.Recv == nil && x.Name.Name == "isInteractiveSuite" {
			require.Nil(t, fd, "isInteractiveSuite declared twice")
			fd = x
		}
	}
	require.NotNil(t, fd, "isInteractiveSuite not found in main.go")
	var stringsImports []string
	for _, im := range f.Imports {
		path, err := strconv.Unquote(im.Path.Value)
		require.NoError(t, err)
		if path == "strings" || (im.Name != nil && im.Name.Name == "strings") {
			stringsImports = append(stringsImports, im.Path.Value)
			require.Nil(t, im.Name, "main.go imports %s with a name: update the reader", im.Path.Value)
		}
	}
	require.Equal(t, []string{`"strings"`}, stringsImports, "main.go must import the standard library strings once")
	// cannot is the failure message for a node this reader does not know: its type and position.
	cannot := func(n ast.Node) string {
		return fmt.Sprintf("isInteractiveSuite has a form this test cannot read (%T at %s): update the reader",
			n, fset.Position(n.Pos()))
	}
	params := fd.Type.Params.List
	require.True(t, len(params) == 1 && len(params[0].Names) == 1 && params[0].Names[0].Name == "suite",
		cannot(fd.Type))
	require.Len(t, fd.Body.List, 2, cannot(fd.Body))
	as, ok := fd.Body.List[0].(*ast.AssignStmt)
	require.True(t, ok && as.Tok == token.DEFINE && len(as.Lhs) == 1 && len(as.Rhs) == 1, cannot(fd.Body.List[0]))
	lhs, ok := as.Lhs[0].(*ast.Ident)
	require.True(t, ok && lhs.Name == "s", cannot(as))
	call, ok := as.Rhs[0].(*ast.CallExpr)
	require.True(t, ok && len(call.Args) == 1, cannot(as.Rhs[0]))
	sel, ok := call.Fun.(*ast.SelectorExpr)
	require.True(t, ok && sel.Sel.Name == "ToLower", cannot(call))
	pkg, ok := sel.X.(*ast.Ident)
	require.True(t, ok && pkg.Name == "strings", cannot(call))
	arg, ok := call.Args[0].(*ast.Ident)
	require.True(t, ok && arg.Name == "suite", cannot(call))
	ret, ok := fd.Body.List[1].(*ast.ReturnStmt)
	require.True(t, ok && len(ret.Results) == 1, cannot(fd.Body.List[1]))

	var names []string
	var walk func(e ast.Expr)
	walk = func(e ast.Expr) {
		b, ok := e.(*ast.BinaryExpr)
		require.True(t, ok, cannot(e))
		if b.Op == token.LOR {
			walk(b.X)
			walk(b.Y)
			return
		}
		require.Equal(t, token.EQL, b.Op, cannot(b))
		x, ok := b.X.(*ast.Ident)
		require.True(t, ok && x.Name == "s", cannot(b))
		lit, ok := b.Y.(*ast.BasicLit)
		require.True(t, ok && lit.Kind == token.STRING, cannot(b))
		name, err := strconv.Unquote(lit.Value)
		require.NoError(t, err)
		// the predicate lower-cases its input first, so a name with an upper-case letter can never match (O-C2-02)
		require.Equal(t, strings.ToLower(name), name, "isInteractiveSuite compares with %q, which no input can match", name)
		names = append(names, name)
	}
	walk(ret.Results[0])
	require.NotEmpty(t, names, cannot(ret))
	return names
}

// T1 (AC1, AC4): availableSuites lists every name the CLI and the bot accept, each once, and nothing else. The
// accepted names are the switch labels of suiteScenarios (b49Routes, pinned to the switch by
// TestB49_SuiteLabelsComplete) and the names isInteractiveSuite accepts (read from main.go).
func TestB126_AvailableSuitesIsEveryRoute(t *testing.T) {
	listed := strings.Split(availableSuites, ", ")
	seen := map[string]bool{}
	var dups []string
	for _, n := range listed {
		require.NotEmpty(t, n, "empty name in availableSuites")
		// the bot splits a /run message on any white space (strings.Fields), so a listed name may hold none
		require.False(t, strings.ContainsRune(n, ',') || strings.IndexFunc(n, unicode.IsSpace) >= 0,
			"malformed name %q in availableSuites", n)
		if seen[n] {
			dups = append(dups, n)
		}
		seen[n] = true
	}
	assert.Empty(t, dups, "names listed more than once")

	var accepted []string
	for _, r := range b49Routes {
		accepted = append(accepted, r.label)
	}
	accepted = append(accepted, b126InteractiveNames(t)...)
	sort.Strings(accepted) // list A in alphabetical order in the failure message
	assert.ElementsMatch(t, accepted, listed,
		"list A = accepted names (switch labels + isInteractiveSuite), list B = availableSuites")
}

// T2 (AC2): the -run flag help prints the same list.
func TestB126_RunFlagUsage(t *testing.T) {
	f := flag.Lookup("run")
	require.NotNil(t, f)
	assert.Equal(t, "Run a test suite: "+availableSuites, f.Usage)
}

// T3 (AC3): the bot's /run with no suite name sends the same list, in one message within Telegram's 4096 characters.
func TestB126_BotRunUsage(t *testing.T) {
	_, texts := b49RunBot(t, "")
	require.Len(t, texts, 1)
	assert.Equal(t, "Usage: /run <suite>\nSuites: "+availableSuites, texts[0])
	assert.LessOrEqual(t, utf8.RuneCountInString(texts[0]), 4096)
}
