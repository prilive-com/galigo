package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/config"
	"github.com/prilive-com/galigo/internal/testutil"
)

// b72Child is the child process of TestB72_CLIInterrupt: runSuiteCommand with a real sender client against a mock
// Bot API server in this process. sendMessage replies with message ids 101, 102, ...; editMessageText replies with the
// id it was sent. Each deleteMessage call first prints "b72: deleteMessage <n> <body>" on stderr.
// Calls before B72_CLI_BLOCK succeed; call B72_CLI_BLOCK blocks until its request is cancelled (the parent sends the
// signal at its line); later calls (the after-stop cleanup) succeed in mode "cleanup" and block in mode "hang".
func b72Child(t *testing.T) {
	suite, mode, dir := os.Getenv("B72_CLI_SUITE"), os.Getenv("B72_CLI_MODE"), os.Getenv("B72_CLI_DIR")
	block, err := strconv.Atoi(os.Getenv("B72_CLI_BLOCK"))
	if suite == "" || err != nil || block < 1 || !filepath.IsAbs(dir) || (mode != "cleanup" && mode != "hang") {
		fmt.Fprintln(os.Stderr, "b72: child needs B72_CLI_SUITE, B72_CLI_BLOCK >= 1, B72_CLI_MODE cleanup|hang and an absolute B72_CLI_DIR")
		os.Exit(3)
	}
	// Only the local mock server is called; the resolver refuses DNS lookups as a backup (B-49 child).
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("b72: no network in the CLI child")
	}}
	server := testutil.NewMockServer(t)
	api := "/bot" + testutil.TestToken + "/"
	server.On(api+"getMe", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyUser(w) })
	var sends atomic.Int32
	server.On(api+"sendMessage", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 100+int(sends.Add(1))) })
	server.On(api+"editMessageText", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MessageID int `json:"message_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MessageID == 0 {
			testutil.ReplyBadRequest(w, "b72: no message_id")
			return
		}
		testutil.ReplyMessage(w, req.MessageID)
	})
	var deletes atomic.Int32
	server.On(api+"deleteMessage", func(w http.ResponseWriter, r *http.Request) {
		n := int(deletes.Add(1))
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(os.Stderr, "b72: deleteMessage %d %s\n", n, body)
		if n == block || (n > block && mode == "hang") {
			<-r.Context().Done()
			return
		}
		testutil.ReplyBool(w, true)
	})
	client := testutil.NewTestClient(t, server.BaseURL())
	cfg := &config.Config{Admins: []int64{1}, ChatID: testutil.TestChatID, StorageDir: dir, MaxMessagesPerRun: 100}
	runSuiteCommand(cfg, client, slog.New(slog.NewTextHandler(os.Stderr, nil)), suite, false)
	os.Exit(0)
}

type b72Run struct {
	status syscall.WaitStatus
	stdout string
	lines  []string
	dir    string
}

// b72Start runs the child. It sends sig when the child prints its block-th deleteMessage line. With again, it then
// sends sig every 100 ms from the next deleteMessage line on, until the child ends: a signal sent before the first
// signal's goroutine restored the default handling is caught, and a later one is not (PLAN AC3).
func b72Start(t *testing.T, suite, mode string, block int, sig os.Signal, again bool) b72Run {
	t.Helper()
	dir, err := filepath.Abs(t.TempDir())
	require.NoError(t, err)
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestB72_CLIInterrupt$", "--", "b72-cli-child")
	cmd.Env = append(os.Environ(), "B72_CLI_SUITE="+suite, "B72_CLI_MODE="+mode, "B72_CLI_DIR="+dir,
		"B72_CLI_BLOCK="+strconv.Itoa(block))
	cmd.Dir = dir
	var stdout strings.Builder
	cmd.Stdout = &stdout
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	lines := make(chan string, 1000)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	var got []string
	sent, repeating := 0, false
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for open := true; open; {
		select {
		case line, ok := <-lines:
			if !ok {
				open = false
				break
			}
			got = append(got, line)
			if sent == 0 && strings.HasPrefix(line, fmt.Sprintf("b72: deleteMessage %d ", block)) {
				if serr := cmd.Process.Signal(sig); serr != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					require.NoError(t, serr, "sending %v", sig)
				}
				sent++
			} else if again && sent == 1 && strings.HasPrefix(line, fmt.Sprintf("b72: deleteMessage %d ", block+1)) {
				repeating = true
			}
		case <-tick.C:
			if repeating {
				_ = cmd.Process.Signal(sig) // the child may already have ended
			}
		}
	}
	err = cmd.Wait()
	require.Equal(t, 1, sent, "first signal not sent; child stderr:\n%s", strings.Join(got, "\n"))
	if again {
		require.True(t, repeating, "the cleanup's deleteMessage never came; child stderr:\n%s", strings.Join(got, "\n"))
	}
	var ws syscall.WaitStatus
	if err != nil {
		var ee *exec.ExitError
		require.ErrorAs(t, err, &ee)
		ws = ee.Sys().(syscall.WaitStatus)
	}
	return b72Run{status: ws, stdout: stdout.String(), lines: got, dir: dir}
}

type b72Scenario struct {
	Name    string `json:"scenario_name"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Steps   []struct {
		Name    string `json:"step_name"`
		Success bool   `json:"success"`
	} `json:"steps"`
}

type b72Saved struct {
	Success   *bool         `json:"success"`
	Scenarios []b72Scenario `json:"scenarios"`
	Summary   struct {
		TotalScenarios int `json:"total_scenarios"`
	} `json:"summary"`
}

func b72Report(t *testing.T, dir string) b72Saved {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "reports", "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	var rep b72Saved
	require.NoError(t, json.Unmarshal(data, &rep))
	require.NotNil(t, rep.Success, "the saved report has a success field")
	return rep
}

type b72Delete struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}

// b72Deletes decodes the body of each "b72: deleteMessage <n> <body>" line.
func b72Deletes(t *testing.T, lines []string) []b72Delete {
	t.Helper()
	var d []b72Delete
	for _, l := range lines {
		rest, ok := strings.CutPrefix(l, "b72: deleteMessage ")
		if !ok {
			continue
		}
		_, body, ok := strings.Cut(rest, " ")
		require.True(t, ok, "delete line %q", l)
		var x b72Delete
		require.NoError(t, json.Unmarshal([]byte(body), &x), "delete body %q", body)
		d = append(d, x)
	}
	return d
}

func TestB72_CLIInterrupt(t *testing.T) {
	if a := flag.Args(); len(a) == 1 && a[0] == "b72-cli-child" {
		b72Child(t)
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal(os.Interrupt) is not implemented on Windows")
	}

	type want struct {
		name string
		ok   bool
	}
	for _, c := range []struct {
		suite   string
		block   int
		sig     os.Signal
		want    []want
		deleted []int // message ids of the deletes: before the signal, the interrupted one, the cleanup's
	}{
		// core: S0 (message 101) and S1 pass, the signal lands in S2's own delete of message 102; S3 and later never
		// start.
		{"core", 2, os.Interrupt, []want{{"S0-Smoke", true}, {"S1-Identity", true}, {"S2-MessageLifecycle", false}}, []int{101, 102, 102}},
		{"smoke", 1, syscall.SIGTERM, []want{{"S0-Smoke", false}}, []int{101, 101}},
	} {
		t.Run(c.suite+"/"+c.sig.String(), func(t *testing.T) {
			r := b72Start(t, c.suite, "cleanup", c.block, c.sig, false)
			require.False(t, r.status.Signaled(), "child ended by %v; stderr:\n%s", r.status.Signal(), strings.Join(r.lines, "\n"))
			assert.Equal(t, 1, r.status.ExitStatus())
			var ids []int
			for _, d := range b72Deletes(t, r.lines) {
				assert.Equal(t, testutil.TestChatID, d.ChatID)
				ids = append(ids, d.MessageID)
			}
			require.Equal(t, c.deleted, ids, "the deletes before the signal, the interrupted one, the cleanup's")
			saved := b72Report(t, r.dir)
			rep := saved.Scenarios
			assert.False(t, *saved.Success, "the saved report says failed")
			assert.Equal(t, len(c.want), saved.Summary.TotalScenarios, "the report was finalized")
			got := make([]want, len(rep))
			for i, s := range rep {
				got[i] = want{s.Name, s.Success}
			}
			require.Equal(t, c.want, got, "exactly the scenarios that started, in order")
			last := rep[len(rep)-1]
			assert.Contains(t, last.Error, `step "deleteMessage" failed`)
			// The signal ended the delete, not the client's 10 s header timeout: the error ends in the cancel cause,
			// "context canceled" on Go 1.25 (NotifyContext sets no cause there) and "<signal> signal received" later.
			suffix := ": " + c.sig.String() + " signal received"
			if strings.HasPrefix(runtime.Version(), "go1.25") {
				suffix = ": context canceled"
			}
			assert.True(t, strings.HasSuffix(last.Error, suffix), "error %q does not end in %q", last.Error, suffix)
			require.NotEmpty(t, last.Steps)
			assert.Equal(t, "cleanup after stop", last.Steps[len(last.Steps)-1].Name)
			assert.True(t, last.Steps[len(last.Steps)-1].Success)
			assert.Contains(t, r.stdout, "Test Run: ")
			assert.Contains(t, r.stdout, "Status: FAILED")
		})
	}

	// A further signal, after the first restored the default handling, ends the child during its blocked cleanup
	// delete (the client's own header timeout would end that delete only after 10 s).
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run("further signal/"+sig.String(), func(t *testing.T) {
			if sig == syscall.SIGINT {
				// A SIGINT ignored when the test process started stays ignored after Stop (os/signal doc): the child
				// would then survive the further SIGINT. go test starts the test binary with the default action.
				require.False(t, signal.Ignored(os.Interrupt), "SIGINT is ignored in this test process; this row needs the default action")
			}
			r := b72Start(t, "smoke", "hang", 1, sig, true)
			require.True(t, r.status.Signaled(), "exit status %d; stderr:\n%s", r.status.ExitStatus(), strings.Join(r.lines, "\n"))
			assert.Equal(t, sig, r.status.Signal())
			_, err := os.Stat(filepath.Join(r.dir, "reports"))
			assert.True(t, os.IsNotExist(err), "no report: the further signal ended the cleanup")
		})
	}
}

// T-seam72: the interactive branch cannot run offline (its poller uses the real API URL), so the context wiring is
// read from the package's syntax tree (E-0112, E-0172). Package rules: interruptContext is exactly
// `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`, `context.AfterFunc(ctx,
// stop)`, `return ctx, stop`; it is called only by runSuiteCommand and runInteractiveSuite, once each; the signal
// package is named only in interruptContext and in bot mode's runInteractive (one signal.Notify). In
// runSuiteCommand and runInteractiveSuite:
//   - neither names context.Background or context.TODO;
//   - ctx is assigned once, by a direct `ctx, <stop> := interruptContext()`, never declared, assigned, shadowed or
//     addressed again; <stop> is named only there and in one direct `defer <stop>()`;
//   - after that definition: no return statement; no panic, log.Fatal*, log.Panic* or runtime.Goexit; os.Exit is
//     named only in the body of the direct `if err := pollingClient.Start(ctx); err != nil` (runInteractiveSuite) and
//     in the last statement;
//   - exactly one runScenarios call, its first argument the bare ctx, as a direct expression statement; in
//     runInteractiveSuite exactly one pollingClient.Start call, its argument the bare ctx, in that direct if;
//   - report is named exactly five times: `report := evidence.NewReport()` (direct), runScenarios's fourth argument,
//     `report.Save(…)`, `report.FormatSummary()`, and the last statement, which is `if !report.Success { os.Exit(1) }`
//     (so no alias, method value, second Finalize or write to report.Success);
//   - the last statement is `if !report.Success { os.Exit(1) }`;
//   - no name defined in the function body (a := or var name, a parameter or result of a func literal,
//     a range variable, a type, a label) equals a top-level name of the package or an import name, so runScenarios,
//     interruptContext, os, signal, ... always mean the package's own (E-0172: a local shadow); neither names
//     syscall (syscall.Exit is an early exit);
//   - os is named only as os.Exit (so no os.FindProcess(os.Getpid()).Kill());
//   - the package-level functions, values and types each refers to (called, as a value, instantiated, or a type
//     whose method is called) are exactly the baseline's (a new helper, a func variable or a method could end the
//     process);
//   - no label and no goto (a goto could jump over runScenarios).
//
// Package: no non-test file imports os, os/signal, syscall, log, runtime or context under another name or as a
// dot import (the rules above match those names).
//
// Limits (the change review's diff read covers them, TEST-PLAN §2): a caller that hangs or blocks forever; a process
// end that names no listed function (a library exit such as flag.CommandLine.Parse with ExitOnError, a runtime panic);
// a scenario run directly by the caller (runner.Run) on a derived context; report.Save before runScenarios.
func TestB72_InterruptSeam(t *testing.T) {
	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)
	funcs := map[string]*ast.FuncDecl{}
	global := map[string]bool{} // top-level names of the package and import names of its non-test files
	var renamed []string        // imports of os, os/signal, syscall, log, runtime or context under another name
	values := map[string]bool{} // package-level vars, consts and types
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, im := range f.Imports {
			switch strings.Trim(im.Path.Value, `"`) {
			case "os", "os/signal", "syscall", "log", "runtime", "context":
				if im.Name != nil {
					renamed = append(renamed, p+": "+im.Name.Name+" "+im.Path.Value)
				}
			}
			name := path.Base(strings.Trim(im.Path.Value, `"`))
			if im.Name != nil {
				name = im.Name.Name
			}
			global[name] = true
		}
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				if x.Recv == nil {
					funcs[x.Name.Name] = x
					global[x.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, sp := range x.Specs {
					switch y := sp.(type) {
					case *ast.ValueSpec:
						for _, nm := range y.Names {
							global[nm.Name] = true
							values[nm.Name] = true
						}
					case *ast.TypeSpec:
						global[y.Name.Name] = true
						values[y.Name.Name] = true
					}
				}
			}
		}
	}
	delete(global, "_")
	require.Contains(t, funcs, "main", "main.go parsed")
	isIdent := func(e ast.Expr, name string) bool {
		id, ok := ast.Unparen(e).(*ast.Ident)
		return ok && id.Name == name
	}
	isCall := func(e ast.Expr, x, sel string) (*ast.CallExpr, bool) {
		c, ok := ast.Unparen(e).(*ast.CallExpr)
		if !ok {
			return nil, false
		}
		if x == "" {
			return c, isIdent(c.Fun, sel)
		}
		s, ok := ast.Unparen(c.Fun).(*ast.SelectorExpr)
		return c, ok && isIdent(s.X, x) && s.Sel.Name == sel
	}
	isSel := func(e ast.Expr, x, sel string) bool {
		s, ok := ast.Unparen(e).(*ast.SelectorExpr)
		return ok && isIdent(s.X, x) && s.Sel.Name == sel
	}
	names := func(root ast.Node, name string) int {
		n := 0
		ast.Inspect(root, func(x ast.Node) bool {
			if id, ok := x.(*ast.Ident); ok && id.Name == name {
				n++
			}
			return true
		})
		return n
	}
	src := func(n ast.Node) string {
		var b strings.Builder
		require.NoError(t, printer.Fprint(&b, fset, n))
		return b.String()
	}

	t.Run("package", func(t *testing.T) {
		assert.Empty(t, renamed, "imports under another name or dot imports (the rules match these names)")
		ic := funcs["interruptContext"]
		require.NotNil(t, ic)
		var body []string
		for _, st := range ic.Body.List {
			body = append(body, src(st))
		}
		assert.Equal(t, []string{
			"ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)",
			"context.AfterFunc(ctx, stop)",
			"return ctx, stop",
		}, body, "interruptContext's body")
		calls, signals := map[string]int{}, map[string]int{}
		for name, fd := range funcs {
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok && isIdent(c.Fun, "interruptContext") {
					calls[name]++
				}
				if s, ok := n.(*ast.SelectorExpr); ok && isIdent(s.X, "signal") {
					signals[name]++
				}
				return true
			})
		}
		assert.Equal(t, map[string]int{"runSuiteCommand": 1, "runInteractiveSuite": 1}, calls, "callers of interruptContext")
		assert.Equal(t, map[string]int{"interruptContext": 1, "runInteractive": 1}, signals, "functions that name the signal package")
	})

	for _, c := range []struct {
		fn    string
		start bool
		calls map[string]bool // the package-level functions, values and types it refers to
	}{
		{"runSuiteCommand", false, map[string]bool{"isInteractiveSuite": true, "runInteractiveSuite": true,
			"suiteScenarios": true, "interruptContext": true, "runScenarios": true, "availableSuites": true}},
		{"runInteractiveSuite", true, map[string]bool{"interruptContext": true, "runScenarios": true}},
	} {
		t.Run(c.fn, func(t *testing.T) {
			fd := funcs[c.fn]
			require.NotNil(t, fd)
			list := fd.Body.List

			// The ctx definition: a direct body statement `ctx, <stop> := interruptContext()`.
			def, stop := -1, ""
			for i, st := range list {
				as, ok := st.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 2 || !isIdent(as.Lhs[0], "ctx") {
					continue
				}
				call, ok := isCall(as.Rhs[0], "", "interruptContext")
				require.True(t, ok && as.Tok == token.DEFINE && len(as.Rhs) == 1 && len(call.Args) == 0,
					"ctx is not `ctx, _ := interruptContext()` at %v", fset.Position(as.Pos()))
				id, ok := as.Lhs[1].(*ast.Ident)
				require.True(t, ok && id.Name != "_", "the stop function is not named")
				def, stop = i, id.Name
			}
			require.GreaterOrEqual(t, def, 0, "no direct `ctx, <stop> := interruptContext()`")

			// No local name shadows a package-level name or an import (E-0172); no syscall.
			defined := func(id *ast.Ident) {
				if id != nil {
					assert.False(t, global[id.Name], "%s defines %q, which shadows a package-level name or an import, at %v", c.fn, id.Name, fset.Position(id.Pos()))
				}
			}
			osExit := map[*ast.Ident]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncType: // parameters and results of a func literal (not struct fields or interface methods)
					for _, fl := range []*ast.FieldList{x.Params, x.Results} {
						if fl != nil {
							for _, fi := range fl.List {
								for _, nm := range fi.Names {
									defined(nm)
								}
							}
						}
					}
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && id.Name == "os" && x.Sel.Name == "Exit" {
						osExit[id] = true
					}
				case *ast.AssignStmt:
					if x.Tok == token.DEFINE {
						for _, l := range x.Lhs {
							if id, ok := l.(*ast.Ident); ok {
								defined(id)
							}
						}
					}
				case *ast.RangeStmt:
					if x.Tok == token.DEFINE {
						for _, e := range []ast.Expr{x.Key, x.Value} {
							if id, ok := e.(*ast.Ident); ok {
								defined(id)
							}
						}
					}
				case *ast.ValueSpec:
					for _, nm := range x.Names {
						defined(nm)
					}
				case *ast.TypeSpec:
					defined(x.Name)
				case *ast.LabeledStmt:
					defined(x.Label)
					assert.Fail(t, "label", "%s has a label at %v", c.fn, fset.Position(x.Pos()))
				case *ast.BranchStmt:
					assert.False(t, x.Tok == token.GOTO, "%s has a goto at %v", c.fn, fset.Position(x.Pos()))
				case *ast.Ident:
					assert.NotEqual(t, "syscall", x.Name, "%s names syscall at %v", c.fn, fset.Position(x.Pos()))
					assert.False(t, x.Name == "os" && !osExit[x], "%s names os other than as os.Exit at %v", c.fn, fset.Position(x.Pos()))
				}
				return true
			})
			// Package-level functions, values and types the body refers to, in any form (a field or method name after a dot,
			// and a composite literal's key, are not references).
			notRef := map[*ast.Ident]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					notRef[x.Sel] = true
				case *ast.KeyValueExpr:
					if id, ok := x.Key.(*ast.Ident); ok {
						notRef[id] = true
					}
				}
				return true
			})
			pkgRefs := map[string]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && !notRef[id] && (funcs[id.Name] != nil || values[id.Name]) {
					pkgRefs[id.Name] = true
				}
				return true
			})
			assert.Equal(t, c.calls, pkgRefs, "%s refers to exactly the baseline's package-level functions, values and types", c.fn)

			defs, runs, starts := 0, 0, 0
			var runCall *ast.CallExpr
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					assert.False(t, isIdent(x.X, "context") && (x.Sel.Name == "Background" || x.Sel.Name == "TODO"),
						"context.%s", x.Sel.Name)
				case *ast.AssignStmt:
					for _, l := range x.Lhs {
						if isIdent(l, "ctx") {
							defs++
						}
					}
				case *ast.ValueSpec:
					for _, nm := range x.Names {
						assert.NotContains(t, []string{"ctx", stop}, nm.Name, "var %s", nm.Name)
					}
				case *ast.Field:
					for _, nm := range x.Names {
						assert.NotContains(t, []string{"ctx", stop}, nm.Name, "parameter %s", nm.Name)
					}
				case *ast.RangeStmt:
					assert.False(t, isIdent(x.Key, "ctx") || isIdent(x.Value, "ctx"), "range ctx")
				case *ast.UnaryExpr:
					assert.False(t, x.Op == token.AND && isIdent(x.X, "ctx"), "&ctx")
				case *ast.CallExpr:
					if call, ok := isCall(x, "", "runScenarios"); ok {
						runs++
						runCall = call
						require.NotEmpty(t, call.Args)
						assert.True(t, isIdent(call.Args[0], "ctx"), "runScenarios's first argument is not ctx")
					}
					if call, ok := isCall(x, "pollingClient", "Start"); ok {
						starts++
						assert.True(t, len(call.Args) == 1 && isIdent(call.Args[0], "ctx"), "pollingClient.Start's argument is not ctx")
					}
				}
				return true
			})
			assert.Equal(t, 1, defs, "ctx assigned once")
			require.Equal(t, 1, runs, "one runScenarios call")

			// <stop> is named only in its definition and in `defer <stop>()`.
			defers := 0
			for _, st := range list[def+1:] {
				if d, ok := st.(*ast.DeferStmt); ok && isIdent(d.Call.Fun, stop) && len(d.Call.Args) == 0 {
					defers++
				}
			}
			assert.Equal(t, 1, defers, "one direct `defer %s()`", stop)
			assert.Equal(t, 2, names(fd.Body, stop), "%s named only in its definition and its defer", stop)

			// Direct body statements: runScenarios(…) as an expression statement; the Start if (interactive), with the
			// condition `err != nil`.
			directRun, startIf := false, (*ast.IfStmt)(nil)
			for _, st := range list {
				if es, ok := st.(*ast.ExprStmt); ok {
					if _, ok := isCall(es.X, "", "runScenarios"); ok {
						directRun = true
					}
				}
				if is, ok := st.(*ast.IfStmt); ok && is.Init != nil {
					if as, ok := is.Init.(*ast.AssignStmt); ok && len(as.Rhs) == 1 {
						if _, ok := isCall(as.Rhs[0], "pollingClient", "Start"); ok {
							startIf = is
						}
					}
				}
			}
			assert.True(t, directRun, "runScenarios(…) is not a direct expression statement")
			if c.start {
				assert.Equal(t, 1, starts, "one pollingClient.Start call")
				require.NotNil(t, startIf, "pollingClient.Start is not in a direct `if err := …; …`")
				assert.Equal(t, "err := pollingClient.Start(ctx)", src(startIf.Init))
				assert.Equal(t, "err != nil", src(startIf.Cond), "the Start if's condition")
				assert.Nil(t, startIf.Else)
			}

			// After the ctx definition: no return; no panic, log.Fatal*, log.Panic* or runtime.Goexit; os.Exit only in
			// the Start if's body (interactive) and in the last statement.
			allowed := map[ast.Node]bool{list[len(list)-1]: true}
			if startIf != nil {
				allowed[startIf.Body] = true
			}
			for _, st := range list[def+1:] {
				var within []ast.Node
				ast.Inspect(st, func(n ast.Node) bool {
					if n == nil {
						within = within[:len(within)-1]
						return false
					}
					within = append(within, n)
					switch x := n.(type) {
					case *ast.ReturnStmt:
						assert.Fail(t, "return after the ctx definition", "at %v", x.Pos())
					case *ast.Ident:
						assert.NotEqual(t, "panic", x.Name, "panic after the ctx definition at %v", x.Pos())
					case *ast.SelectorExpr:
						assert.False(t, isIdent(x.X, "log") && (strings.HasPrefix(x.Sel.Name, "Fatal") || strings.HasPrefix(x.Sel.Name, "Panic")),
							"log.%s after the ctx definition", x.Sel.Name)
						assert.False(t, isSel(x, "runtime", "Goexit"), "runtime.Goexit after the ctx definition")
						if isSel(x, "os", "Exit") {
							ok := false
							for _, w := range within {
								ok = ok || allowed[w]
							}
							assert.True(t, ok, "os.Exit after the ctx definition at %v", x.Pos())
						}
					}
					return true
				})
			}

			// report: defined once, then named only as runScenarios's fourth argument, in report.Save(…),
			// report.FormatSummary() and the last statement.
			assert.Equal(t, 5, names(fd.Body, "report"), "report named only in its five places")
			reportDef, saves, summaries := 0, 0, 0
			for _, st := range list {
				if as, ok := st.(*ast.AssignStmt); ok && len(as.Lhs) == 1 && isIdent(as.Lhs[0], "report") {
					_, ok := isCall(as.Rhs[0], "evidence", "NewReport")
					assert.True(t, ok && as.Tok == token.DEFINE, "report := evidence.NewReport()")
					reportDef++
				}
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if _, ok := isCall0(n, isCall, "report", "Save"); ok {
					saves++
				}
				if call, ok := isCall0(n, isCall, "report", "FormatSummary"); ok && len(call.Args) == 0 {
					summaries++
				}
				return true
			})
			assert.Equal(t, 1, reportDef, "one direct report := evidence.NewReport()")
			assert.Equal(t, 1, saves, "one report.Save call")
			assert.Equal(t, 1, summaries, "one report.FormatSummary() call")
			require.Len(t, runCall.Args, 5)
			assert.True(t, isIdent(runCall.Args[3], "report"), "runScenarios's fourth argument is not report")

			// The last statement: if !report.Success { os.Exit(1) }, no init, no else.
			is, ok := list[len(list)-1].(*ast.IfStmt)
			require.True(t, ok, "the last statement is not an if")
			require.True(t, is.Init == nil && is.Else == nil && len(is.Body.List) == 1)
			not, ok := ast.Unparen(is.Cond).(*ast.UnaryExpr)
			require.True(t, ok && not.Op == token.NOT && isSel(not.X, "report", "Success"), "condition is not !report.Success")
			es, ok := is.Body.List[0].(*ast.ExprStmt)
			require.True(t, ok)
			call, ok := isCall(es.X, "os", "Exit")
			require.True(t, ok && len(call.Args) == 1)
			lit, ok := ast.Unparen(call.Args[0]).(*ast.BasicLit)
			require.True(t, ok && lit.Value == "1", "os.Exit(1)")
		})
	}
}

// isCall0 applies isCall to a node that may not be an expression.
func isCall0(n ast.Node, isCall func(ast.Expr, string, string) (*ast.CallExpr, bool), x, sel string) (*ast.CallExpr, bool) {
	e, ok := n.(ast.Expr)
	if !ok {
		return nil, false
	}
	return isCall(e, x, sel)
}
