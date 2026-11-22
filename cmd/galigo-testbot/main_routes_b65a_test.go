package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/internal/testutil"
)

// B-65 65a T-routes (register O1-03): S73's group is run by `--run ephemeral` and `--run all` (suiteScenarios, used
// by runSuiteCommand and the bot's handleRun) and counted by both status lists (showCoverageStatus, handleStatus). The
// B-48c1 test checks only that `all` is a subset of the status lists, so a group missing everywhere passes it. This
// test reads main.go's syntax tree. It proves the case bodies, not which input reaches a case: the B-49 tests
// (main_suites_b49_test.go) run both callers, and `--run ephemeral` is proved live. A discarded call in an "all" case
// passes this count; the B-48c1 test (append shape) refuses it, so the two tests together prove the "all" lists
// (E-0172, register A2-05).
func TestB65A_MainRoutes(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	funcs := map[string]*ast.FuncDecl{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			funcs[fd.Name.Name] = fd
		}
	}
	isGroup := func(n ast.Node) bool { // the call suites.AllEphemeralScenarios()
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && id.Name == "suites" && sel.Sel.Name == "AllEphemeralScenarios"
	}
	calls := func(n ast.Node) int {
		c := 0
		ast.Inspect(n, func(m ast.Node) bool {
			if isGroup(m) {
				c++
			}
			return true
		})
		return c
	}
	// caseBody returns the body of the switch case whose list has the string label.
	caseBody := func(fd *ast.FuncDecl, label string) []ast.Stmt {
		var body []ast.Stmt
		found := 0
		ast.Inspect(fd, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING && lit.Value == `"`+label+`"` {
					body = cc.Body
					found++
				}
			}
			return true
		})
		require.Equal(t, 1, found, "%s: case %q", fd.Name.Name, label)
		return body
	}
	for _, name := range []string{"suiteScenarios"} {
		fd := funcs[name]
		require.NotNil(t, fd, name)
		// `case "ephemeral":` is exactly `scenarios = suites.AllEphemeralScenarios()`.
		body := caseBody(fd, "ephemeral")
		require.Len(t, body, 1, "%s: case \"ephemeral\"", name)
		as, ok := body[0].(*ast.AssignStmt)
		require.True(t, ok, "%s: case \"ephemeral\" is not an assignment", name)
		require.Len(t, as.Lhs, 1)
		require.Len(t, as.Rhs, 1)
		lhs, ok := as.Lhs[0].(*ast.Ident)
		assert.True(t, ok && lhs.Name == "scenarios" && as.Tok == token.ASSIGN, "%s: case \"ephemeral\" assigns scenarios", name)
		assert.True(t, isGroup(as.Rhs[0]), "%s: case \"ephemeral\" runs suites.AllEphemeralScenarios()", name)
		// `case "all":` appends the group once (the B-48c1 test checks the append's shape).
		all := &ast.BlockStmt{List: caseBody(fd, "all")}
		assert.Equal(t, 1, calls(all), "%s: case \"all\"", name)
	}
	for _, name := range []string{"showCoverageStatus", "handleStatus"} {
		fd := funcs[name]
		require.NotNil(t, fd, name)
		assert.Equal(t, 1, calls(fd.Body), "%s counts the group", name)
	}
}

// b65aEdits are the four ephemeral edit methods; S73 covers them since B-65 65b.
var b65aEdits = []string{"editEphemeralMessageText", "editEphemeralMessageMedia", "editEphemeralMessageCaption",
	"editEphemeralMessageReplyMarkup"}

// B-65 65a T-status-out (register O2-02): the real status output. `--status` (showCoverageStatus, on stdout) lists
// deleteEphemeralMessage as covered and, since B-65 65b, the four edits as covered too; the bot's /status text
// (handleStatus, sent to a mock server) lists neither the delete nor the edits as missing. A status function that drops S73's group, or passes
// another list to registry.CheckCoverage, changes this output.
// b65aCLIStatus returns the lines that showCoverageStatus prints.
func b65aCLIStatus(t *testing.T) []string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	out := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); out <- b }()
	old := os.Stdout
	os.Stdout = w
	func() {
		defer func() { os.Stdout = old }()
		showCoverageStatus(slog.New(slog.DiscardHandler))
	}()
	require.NoError(t, w.Close())
	return strings.Split(string(<-out), "\n")
}

// b65aCount returns N of the first line "<prefix>N<suffix>".
func b65aCount(t *testing.T, lines []string, prefix, suffix string) string {
	t.Helper()
	for _, l := range lines {
		if n, ok := strings.CutPrefix(l, prefix); ok {
			if n, ok = strings.CutSuffix(n, suffix); ok {
				return n
			}
		}
	}
	t.Fatalf("no line %q...%q", prefix, suffix)
	return ""
}

func TestB65A_StatusOutput(t *testing.T) {
	t.Run("cli", func(t *testing.T) {
		lines := b65aCLIStatus(t)
		assert.Contains(t, lines, "  + deleteEphemeralMessage")
		assert.NotContains(t, lines, "  - deleteEphemeralMessage")
		for _, m := range b65aEdits { // B-65 65b: covered by S73
			assert.Contains(t, lines, "  + "+m)
			assert.NotContains(t, lines, "  - "+m)
		}
		assert.Equal(t, "97", b65aCount(t, lines, "Covered: ", " methods")) // B-65 65b: 92; B-65 65f: the five widget methods
		assert.Equal(t, "10", b65aCount(t, lines, "Missing: ", " methods"))
	})
	t.Run("bot", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyMessage(w, 1)
		})
		handleStatus(context.Background(), engine.NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), testutil.TestChatID)
		require.Equal(t, 1, server.CaptureCount())
		var body struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.Unmarshal(server.LastCapture().Body, &body))
		lines := strings.Split(body.Text, "\n")
		cli := b65aCLIStatus(t) // the same counts as the CLI (register O3-07)
		assert.Contains(t, lines, "Covered: "+b65aCount(t, cli, "Covered: ", " methods"))
		assert.Contains(t, lines, "Missing: "+b65aCount(t, cli, "Missing: ", " methods"))
		assert.NotContains(t, lines, "  - deleteEphemeralMessage")
		for _, m := range b65aEdits { // B-65 65b: covered by S73
			assert.NotContains(t, lines, "  - "+m)
		}
		assert.Contains(t, lines, "Missing methods:")       // the list the edits would be in
		assert.Contains(t, lines, "  - getManagedBotToken") // a positive control in the same format (A-T3-05)
	})
}
