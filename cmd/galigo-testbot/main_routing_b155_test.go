package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/config"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/suites"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-155: the names /run and --run accept are pinned by the switch labels (B-49) and isInteractiveSuite (B-126). These
// source checks pin the four routing functions around them, so a route for an extra name added there fails a test.

// b155Func is one function declaration with its file and the import names of that file.
type b155Func struct {
	decl    *ast.FuncDecl
	file    string
	imports map[string]bool
}

// b155Pkg is the scan of the package's non-test files.
type b155Pkg struct {
	fset    *token.FileSet
	globals map[string]bool // package-level functions (no receiver), vars, consts and types
	funcs   map[string][]b155Func
	dots    []string // each dot import, as "file: path"
	mainImp []string // main.go's imports, as the path, or "name path" for a named import
}

// b155MainImports is main.go's import list at e234189. The four routing functions must be in main.go (fn), so a
// package they use is one of these: a new import, or a new path under a name already used, changes this list.
var b155MainImports = []string{
	"context", "flag", "fmt", "log/slog", "os", "os/signal", "strings", "syscall",
	"github.com/prilive-com/galigo/cmd/galigo-testbot/cleanup",
	"github.com/prilive-com/galigo/cmd/galigo-testbot/config",
	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine",
	"github.com/prilive-com/galigo/cmd/galigo-testbot/evidence",
	"github.com/prilive-com/galigo/cmd/galigo-testbot/registry",
	"github.com/prilive-com/galigo/cmd/galigo-testbot/suites",
	"github.com/prilive-com/galigo/receiver",
	"github.com/prilive-com/galigo/sender",
	"github.com/prilive-com/galigo/tg",
}

// b155Scan parses every non-test .go file of the package.
func b155Scan(t *testing.T) *b155Pkg {
	t.Helper()
	p := &b155Pkg{fset: token.NewFileSet(), globals: map[string]bool{}, funcs: map[string][]b155Func{}}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(p.fset, name, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		imports := map[string]bool{}
		for _, im := range f.Imports {
			ipath, err := strconv.Unquote(im.Path.Value)
			require.NoError(t, err)
			n, spec := path.Base(ipath), ipath
			if im.Name != nil {
				n, spec = im.Name.Name, im.Name.Name+" "+ipath
			}
			if name == "main.go" {
				p.mainImp = append(p.mainImp, spec)
			}
			if n == "." {
				p.dots = append(p.dots, name+": "+ipath)
			} else if n != "_" {
				imports[n] = true
			}
		}
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				if x.Recv == nil {
					p.globals[x.Name.Name] = true
					p.funcs[x.Name.Name] = append(p.funcs[x.Name.Name], b155Func{x, name, imports})
				}
			case *ast.GenDecl:
				for _, sp := range x.Specs {
					switch y := sp.(type) {
					case *ast.ValueSpec:
						for _, n := range y.Names {
							p.globals[n.Name] = true
						}
					case *ast.TypeSpec:
						p.globals[y.Name.Name] = true
					}
				}
			}
		}
	}
	delete(p.globals, "_")
	require.NotEmpty(t, p.globals, "the package scan found no package-level names")
	return p
}

func (p *b155Pkg) fn(t *testing.T, name string) b155Func {
	t.Helper()
	require.Len(t, p.funcs[name], 1, "exactly one declaration of %s", name)
	f := p.funcs[name][0]
	require.Equal(t, "main.go", f.file, "B155-READS: %s is declared in main.go, whose imports are pinned", name)
	return f
}

func (p *b155Pkg) print(t *testing.T, n ast.Node) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, printer.Fprint(&b, p.fset, n))
	return b.String()
}

// b155Reads lists each use of param in the body as "<kind of the nearest non-expression ancestor>: <printed source of
// the largest enclosing expression>", in source order (AC1).
func (p *b155Pkg) reads(t *testing.T, body *ast.BlockStmt, param string) []string {
	t.Helper()
	var reads []string
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == param {
			top, i := ast.Node(id), len(stack)-1
			for ; i >= 0; i-- {
				if _, isExpr := stack[i].(ast.Expr); !isExpr {
					break
				}
				top = stack[i]
			}
			reads = append(reads, fmt.Sprintf("%T: %s", stack[i], p.print(t, top)))
		}
		stack = append(stack, n)
		return true
	})
	return reads
}

func b155Names(fl *ast.FieldList) []string {
	var names []string
	if fl != nil {
		for _, f := range fl.List {
			for _, n := range f.Names {
				if n.Name != "_" {
					names = append(names, n.Name)
				}
			}
		}
	}
	return names
}

// b155Defs lists every name the body defines (AC2): := (also in type switches and select cases), var and const,
// range :=, func-literal parameters and results, types and labels. The blank identifier is skipped. Parameter names
// inside a function type (var f func(x int)) declare nothing and are not listed.
func b155Defs(body *ast.BlockStmt) []string {
	var defs []string
	ident := func(e ast.Expr) {
		if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
			defs = append(defs, id.Name)
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, l := range x.Lhs {
					ident(l)
				}
			}
		case *ast.ValueSpec:
			for _, n := range x.Names {
				ident(n)
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				ident(x.Key)
				ident(x.Value)
			}
		case *ast.FuncLit:
			defs = append(defs, b155Names(x.Type.Params)...)
			defs = append(defs, b155Names(x.Type.Results)...)
		case *ast.TypeSpec:
			ident(x.Name)
		case *ast.LabeledStmt:
			ident(x.Label)
		}
		return true
	})
	return defs
}

// b155StructLit is the one struct type the four functions write as a keyed literal at e234189 (engine.RunnerConfig in
// handleRun and runSuiteCommand); its keys are field names.
const b155StructLit = "engine.RunnerConfig"

// b155Sorted returns the keys of a map, sorted.
func b155Sorted[V any](set map[string]V) []string {
	out := []string{}
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestB155_RunRouting pins the reads of the suite name, the local names and the package names that /run's bot path
// (handleCommand, handleRun), the CLI path (runSuiteCommand) and the selection (suiteScenarios) use.
func TestB155_RunRouting(t *testing.T) {
	p := b155Scan(t)
	const update = "; update the expected list only in a reviewed change to the /run or --run routing"

	t.Run("package", func(t *testing.T) {
		assert.Empty(t, p.dots, "B155-DOTIMPORT: a dot import puts names in scope without a package qualifier")
		assert.Equal(t, b155MainImports, p.mainImp, "B155-IMPORTS: main.go imports"+update)
	})

	t.Run("sendMessage", func(t *testing.T) {
		var b strings.Builder
		for _, st := range p.fn(t, "sendMessage").decl.Body.List {
			b.WriteString(p.print(t, st) + "\n")
		}
		assert.Equal(t, "_, _ = adapter.SendMessage(ctx, chatID, text)\n", b.String(),
			"B155-SENDBODY: sendMessage only sends the text"+update)
	})

	for _, row := range []struct {
		fn, param string
		reads     []string
		pkgRefs   []string
		imports   []string
	}{
		{"handleRun", "suite", []string{
			`*ast.IfStmt: suite == ""`,
			`*ast.IfStmt: isInteractiveSuite(suite)`,
			`*ast.AssignStmt: suiteScenarios(suite)`,
			`*ast.ExprStmt: sendMessage(ctx, adapter, chatID, "Unknown suite: "+suite)`,
		}, []string{"availableSuites", "isInteractiveSuite", "sendMessage", "suiteScenarios"},
			[]string{"engine", "evidence", "fmt", "suites", "tg"}},
		{"runSuiteCommand", "suite", []string{
			`*ast.IfStmt: isInteractiveSuite(suite)`,
			`*ast.AssignStmt: suiteScenarios(suite)`,
			`*ast.ExprStmt: logger.Error("unknown suite", "suite", suite)`,
		}, []string{"availableSuites", "interruptContext", "isInteractiveSuite", "runInteractiveSuite", "runScenarios",
			"suiteScenarios"}, []string{"engine", "evidence", "fmt", "os"}},
		{"handleCommand", "args", []string{
			`*ast.ExprStmt: handleRun(ctx, cfg, senderClient, adapter, logger, chatID, args, updates)`,
		}, []string{"handleHelp", "handleRun", "handleStatus", "sendMessage"}, []string{}},
		{"suiteScenarios", "suite", []string{
			`*ast.SwitchStmt: strings.ToLower(suite)`,
		}, []string{}, []string{"engine", "strings", "suites"}},
	} {
		t.Run(row.fn, func(t *testing.T) {
			f := p.fn(t, row.fn)
			body := f.decl.Body

			// AC1: every read of the name.
			assert.Equal(t, row.reads, p.reads(t, body, row.param), "B155-READS: %s reads of %s%s", row.fn, row.param, update)

			// AC2: no local name and no signature name hides a package name, an import or a parameter.
			sig := append(b155Names(f.decl.Type.Params), b155Names(f.decl.Type.Results)...)
			params := map[string]bool{}
			for _, n := range sig {
				params[n] = true
			}
			var bad []string
			for _, n := range sig {
				if p.globals[n] || f.imports[n] {
					bad = append(bad, "signature "+n)
				}
			}
			for _, n := range b155Defs(body) {
				if p.globals[n] || f.imports[n] || params[n] {
					bad = append(bad, "local "+n)
				}
			}
			assert.Empty(t, bad, "B155-SHADOW: %s defines names that hide others%s", row.fn, update)

			// AC2b: (i) package-level names the signature and the body use, and (ii) import names the body uses. A
			// selector's Sel names a field or a package member. A key names a field only in a literal of
			// b155StructLit, the one struct type the four functions write as a literal; every other key counts.
			notRef := map[*ast.Ident]bool{}
			inspect := func(n ast.Node, fn func(ast.Node) bool) {
				ast.Inspect(f.decl.Type, fn)
				ast.Inspect(n, fn)
			}
			inspect(body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					notRef[x.Sel] = true
				case *ast.CompositeLit:
					if x.Type == nil || p.print(t, x.Type) != b155StructLit {
						return true
					}
					for _, e := range x.Elts {
						if kv, ok := e.(*ast.KeyValueExpr); ok {
							if id, ok := kv.Key.(*ast.Ident); ok {
								notRef[id] = true
							}
						}
					}
				}
				return true
			})
			refs, imps := map[string]bool{}, map[string]bool{}
			inspect(body, func(n ast.Node) bool {
				if x, ok := n.(*ast.Ident); ok && !notRef[x] && p.globals[x.Name] {
					refs[x.Name] = true
				}
				return true
			})
			ast.Inspect(body, func(n ast.Node) bool {
				if x, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := x.X.(*ast.Ident); ok && f.imports[id.Name] {
						imps[id.Name] = true
					}
				}
				return true
			})
			assert.Equal(t, row.pkgRefs, b155Sorted(refs), "B155-PKGREFS: %s package-level names%s", row.fn, update)
			assert.Equal(t, row.imports, b155Sorted(imps), "B155-IMPORTS: %s import names%s", row.fn, update)
		})
	}
}

// B-156: /run replies from a group go to the group, also on the success path and with a message budget.
func TestB155_BotRunGroupChat(t *testing.T) {
	for _, row := range []struct {
		name, suite string
		budget, n   int
		check       func(t *testing.T, texts []string)
	}{
		{"empty", "", 50, 1, func(t *testing.T, texts []string) {
			assert.True(t, strings.HasPrefix(texts[0], "Usage: /run <suite>\n"), texts[0])
		}},
		{"empty0", "", 0, 1, func(t *testing.T, texts []string) {
			assert.True(t, strings.HasPrefix(texts[0], "Usage: /run <suite>\n"), texts[0])
		}},
		{"nosuch", "nosuch", 50, 1, func(t *testing.T, texts []string) {
			assert.Equal(t, "Unknown suite: nosuch", texts[0])
		}},
		{"nosuch0", "nosuch", 0, 1, func(t *testing.T, texts []string) {
			assert.Equal(t, "Unknown suite: nosuch", texts[0])
		}},
		{"smoke", "smoke", 0, 2, func(t *testing.T, texts []string) {
			assert.Equal(t, "Running 1 scenario(s)...", texts[0])
			assert.True(t, strings.HasPrefix(texts[1], "Test Run: "), texts[1])
		}},
		{"interactive", "interactive", 0, 2, func(t *testing.T, texts []string) {
			assert.Equal(t, "Running "+strconv.Itoa(len(suites.AllInteractiveScenarios()))+" scenario(s)...", texts[0])
			assert.True(t, strings.HasPrefix(texts[1], "Test Run: "), texts[1])
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			_, texts := b49RunBotFrom(t, b65fGroup, row.budget, row.suite)
			require.Len(t, texts, row.n, "B156-COUNT: replies to /run %q from the group", row.suite)
			row.check(t, texts)
		})
	}
	t.Run("success", func(t *testing.T) { b155RunSuccess(t, b65fGroup) })
}

// A private chat with a message budget, through the dispatcher.
func TestB155_BotRunPrivateSuccess(t *testing.T) {
	b155RunSuccess(t, 77)
}

// b155RunSuccess runs the bot's /run api104 through handleCommand from chat, with a message budget, the B-65F mock
// and a config chat that is not the command chat. It checks every request the mock server received, also those the
// B-65F handlers do not record: each body is a JSON object (not null), there is one "Running" reply and one summary,
// no request is ephemeral (no ephemeral method, no ephemeral_message_parameters, no receiver_user_id), each sendMessage
// has only chat_id, text and maybe reply_parameters, and every chat_id is the command chat. The report check is last,
// so a wrong chat is reported as B156-CHAT.
func b155RunSuccess(t *testing.T, chat int64) {
	t.Helper()
	b := newB65FBot(t)
	client := testutil.NewTestClient(t, b.server.BaseURL(), sender.WithRateLimit(1000, 100),
		sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	cfg := &config.Config{Admins: []int64{1}, ChatID: 424242, StorageDir: t.TempDir(), MaxMessagesPerRun: 50}
	updates := make(chan tg.Update)
	defer close(updates)
	handleCommand(context.Background(), cfg, client, engine.NewSenderAdapter(client), nil, b49Discard(), chat, "run",
		"api104", updates)

	captures := b.server.Captures()
	bodies := make([]map[string]any, len(captures))
	for i, c := range captures {
		require.NoError(t, json.Unmarshal(c.Body, &bodies[i]), "B156-CHAT: request %d (%s) has a JSON object body", i,
			path.Base(c.Path))
		require.NotNil(t, bodies[i], "B156-CHAT: request %d (%s) has a JSON object body, not null", i, path.Base(c.Path))
	}
	running, summaries := 0, 0
	for i, c := range captures {
		if path.Base(c.Path) != "sendMessage" {
			continue
		}
		text, _ := bodies[i]["text"].(string)
		if text == "Running 4 scenario(s)..." {
			running++
		}
		if strings.HasPrefix(text, "Test Run: ") {
			summaries++
		}
	}
	assert.Equal(t, []int{1, 1}, []int{running, summaries}, "B156-COUNT: one Running reply and one summary")
	for i, c := range captures {
		keys := b155Sorted(bodies[i])
		assert.False(t, strings.Contains(path.Base(c.Path), "Ephemeral") || slices.Contains(keys, "receiver_user_id") ||
			slices.Contains(keys, "ephemeral_message_parameters"), "B156-CHAT: request %d (%s) is not ephemeral", i,
			path.Base(c.Path))
		if path.Base(c.Path) == "sendMessage" {
			assert.Contains(t, [][]string{{"chat_id", "text"}, {"chat_id", "reply_parameters", "text"}}, keys,
				"B156-CHAT: message %d has no recipient field other than chat_id", i)
		}
		if id, ok := bodies[i]["chat_id"]; ok {
			assert.Equal(t, float64(chat), id, "B156-CHAT: request %d (%s) goes to the chat the command came from", i,
				path.Base(c.Path))
		}
	}
	rep := b72Report(t, cfg.StorageDir)
	assert.True(t, *rep.Success, "the run is successful, so the success path is checked")
}
