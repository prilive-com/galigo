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
// the set of groups in each "all" case must be a subset of the set in each status function.
//
// [C2-01 Codex, C2-03 Claude] A group counts only when its call is an argument of the one supported shape,
// `scenarios := append(suites.AllXScenarios(), suites.AllYScenarios()...)` (`=` in the "all" cases) or
// `scenarios = append(scenarios, suites.AllXScenarios()...)`. Any other write to `scenarios`, and any
// suites.All*Scenarios call outside that shape (a discarded result, a loop), fails the test; in an "all" case every
// statement must have that shape, so a group reached through a helper fails too. A new shape needs this test changed.
// [C3-01, C3-02 Claude, C3-01 Codex] Each supported append must be a top-level statement of the list's own block (the
// function body, or the "all" case body), so an append inside an `if` or in a nested block with a second `scenarios`
// fails; and the two-group append must be the first one, and only the first, so starting the list again fails.
func TestB48C1_StatusListsCoverAll(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	funcs := map[string]*ast.FuncDecl{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			funcs[fd.Name.Name] = fd
		}
	}
	isScenarios := func(e ast.Expr) bool { id, ok := e.(*ast.Ident); return ok && id.Name == "scenarios" }
	group := func(e ast.Expr) (string, bool) { // the name of a call suites.All*Scenarios()
		call, ok := e.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return "", false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != "suites" || !strings.HasPrefix(sel.Sel.Name, "All") || !strings.HasSuffix(sel.Sel.Name, "Scenarios") {
			return "", false
		}
		return sel.Sel.Name, true
	}
	// appended returns the groups a statement appends to scenarios, and false if it is not the supported shape.
	appended := func(st ast.Stmt) ([]string, bool) {
		as, ok := st.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 || !isScenarios(as.Lhs[0]) {
			return nil, false
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok || !call.Ellipsis.IsValid() || len(call.Args) != 2 {
			return nil, false
		}
		if fn, isIdent := call.Fun.(*ast.Ident); !isIdent || fn.Name != "append" {
			return nil, false
		}
		last, ok := group(call.Args[1])
		if !ok {
			return nil, false
		}
		if as.Tok == token.ASSIGN && isScenarios(call.Args[0]) {
			return []string{last}, true
		}
		if first, ok := group(call.Args[0]); ok {
			return []string{first, last}, true
		}
		return nil, false
	}
	// groups returns the groups appended under n. Every write to scenarios and every group call under n must be
	// part of a supported append that is one of stmts (the list's own block); with strict, every statement in stmts
	// must be one.
	groups := func(where string, n ast.Node, stmts []ast.Stmt, strict bool) map[string]bool {
		got, inAppend, top := map[string]bool{}, map[ast.Node]bool{}, map[ast.Stmt]bool{}
		for _, st := range stmts {
			top[st] = true
		}
		ast.Inspect(n, func(n ast.Node) bool {
			if st, ok := n.(ast.Stmt); ok {
				if gs, ok := appended(st); ok {
					assert.True(t, top[st], "%s: an append that is not a statement of the list itself", where)
					assert.Equal(t, len(gs) == 2, len(got) == 0, "%s: the two-group append must be the first, and only the first", where)
					for _, g := range gs {
						got[g] = true
					}
					for _, a := range st.(*ast.AssignStmt).Rhs[0].(*ast.CallExpr).Args {
						inAppend[a] = true
					}
				} else if as, ok := st.(*ast.AssignStmt); ok {
					for _, l := range as.Lhs {
						assert.False(t, isScenarios(l), "%s: a write to scenarios that is not the supported append", where)
					}
				}
			}
			if e, ok := n.(ast.Expr); ok {
				if g, ok := group(e); ok {
					assert.True(t, inAppend[e], "%s: %s is not appended to scenarios", where, g)
				}
			}
			return true
		})
		if strict {
			for _, st := range stmts {
				_, ok := appended(st)
				assert.True(t, ok, "%s: a statement that is not the supported append", where)
			}
		}
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
					got = groups(fd.Name.Name+" all", &ast.BlockStmt{List: cc.Body}, cc.Body, true)
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
			counted := groups(status, funcs[status], funcs[status].Body.List, false)
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
