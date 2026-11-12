package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B-48c1 [C1-01]: every scenario group that `--run all` runs is also counted by `--status`. main.go builds the lists
// by hand in four places (the "all" case of runSuiteCommand and of handleRun, showCoverageStatus, handleStatus), so a
// new group added to "all" and not to the two status lists is never counted. The test reads main.go's syntax tree:
// the set of suites.All*Scenarios calls in each "all" case must be a subset of the set in each status function.
func TestB48C1_StatusListsCoverAll(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	funcs := map[string]*ast.FuncDecl{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			funcs[fd.Name.Name] = fd
		}
	}
	groups := func(n ast.Node) map[string]bool { // the suites.All*Scenarios calls under n
		got := map[string]bool{}
		ast.Inspect(n, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "suites" &&
					strings.HasPrefix(sel.Sel.Name, "All") && strings.HasSuffix(sel.Sel.Name, "Scenarios") {
					got[sel.Sel.Name] = true
				}
			}
			return true
		})
		return got
	}
	allCase := func(fd *ast.FuncDecl) map[string]bool { // the groups of the `case "all":` clause in fd
		var got map[string]bool
		ast.Inspect(fd, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if lit, ok := e.(*ast.BasicLit); ok && lit.Value == `"all"` {
					require.Nil(t, got, "%s has one \"all\" case", fd.Name.Name)
					got = groups(&ast.BlockStmt{List: cc.Body})
				}
			}
			return true
		})
		require.NotEmpty(t, got, "%s: the \"all\" case", fd.Name.Name)
		return got
	}
	for _, run := range []string{"runSuiteCommand", "handleRun"} {
		require.Contains(t, funcs, run)
		all := allCase(funcs[run])
		assert.True(t, all["AllSuggestedPostScenarios"], "%s: S71 runs in all", run)
		for _, status := range []string{"showCoverageStatus", "handleStatus"} {
			require.Contains(t, funcs, status)
			counted := groups(funcs[status])
			var missing []string
			for g := range all {
				if !counted[g] {
					missing = append(missing, g)
				}
			}
			slices.Sort(missing)
			assert.Empty(t, missing, "%s counts every group of %s's \"all\" case; have %v",
				status, run, slices.Sorted(maps.Keys(counted)))
		}
	}
}
