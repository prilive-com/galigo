package galigo_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B-110: every fenced go block in README.md and docs/*.md builds the way the
// go-check directive before it says (docs/testing.md, "Go blocks in the docs").
// The checker below is test code; TestDocGoBlocksDetects and the U tests prove it.

// docRepoRoot is the repo root: the test runs in the root package's directory.
func docRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	require.NoError(t, err)
	return root
}

// docFiles lists the scanned Markdown files in slash form.
func docFiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join("docs", "*.md"))
	if err != nil {
		return nil, err
	}
	out := []string{"README.md"}
	for _, f := range files {
		out = append(out, filepath.ToSlash(f))
	}
	sort.Strings(out[1:])
	return out, nil
}

// docGroup is one built set of blocks and what the build reported.
type docGroup struct {
	blocks  []docBlock // all blocks found, skip blocks included
	built   []docBlock // the blocks given to go build, in target order
	errs    []docErr   // extraction and generation errors
	ran     bool       // go build ran
	args    []string
	reports []docErr
}

// runDocGroup extracts, generates and builds the given Markdown files. It never
// fails the test itself: the subtests check what it stored.
func runDocGroup(goPath, root string, files []string, env []string) docGroup {
	var g docGroup
	slashRoot := filepath.ToSlash(root)
	for _, name := range files {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name))) // #nosec G304 -- name is a docs file from docFiles or a fixture path in testdata
		if err != nil {
			g.errs = append(g.errs, docErr{File: name, Msg: err.Error()})
			continue
		}
		blocks, errs := extractBlocks(name, src)
		g.errs = append(g.errs, errs...)
		for _, b := range blocks {
			b.Abs = slashRoot + "/" + b.File
			g.blocks = append(g.blocks, b)
		}
	}
	var gens []genFile
	for _, b := range g.blocks {
		if b.Mode == "skip" || b.Mode == "" {
			continue // a block with no valid directive is already an extraction error
		}
		k := len(g.built)
		src, err := generate(b, k)
		if err != nil {
			g.errs = append(g.errs, docErr{File: b.File, Line: b.Fence, Msg: err.Error()})
		}
		g.built = append(g.built, b)
		gens = append(gens, genFile{Dir: fmt.Sprintf("b%02d", k), Src: src})
	}
	if len(g.errs) > 0 {
		return g
	}
	if err := checkVirtualAbsent(root); err != nil {
		g.errs = append(g.errs, docErr{Msg: err.Error()})
		return g
	}
	out, rc, args := buildBlocks(goPath, root, gens, env)
	g.ran, g.args = true, args
	g.reports = parseBuild(out, rc, root, g.built)
	return g
}

func docTargets(n int) []string {
	out := make([]string, n)
	for k := range out {
		out[k] = fmt.Sprintf("./docsnippetchk/b%02d", k)
	}
	return out
}

func argsTail(args []string, n int) []string {
	if len(args) < n {
		return args
	}
	return args[len(args)-n:]
}

// TestDocGoBlocks builds every go block of the real docs (B-110).
func TestDocGoBlocks(t *testing.T) {
	root := docRepoRoot(t)
	goPath := requireGo(t, exec.LookPath)
	files, globErr := docFiles()
	g := runDocGroup(goPath, root, files, childEnv(os.Environ()))
	_, statErr := os.Lstat(filepath.Join(root, "docsnippetchk"))

	t.Run("scanned_set", func(t *testing.T) {
		require.NoError(t, globErr)
		assert.Equal(t, []string{"README.md", "docs/architecture.md", "docs/configuration.md", "docs/errors.md", "docs/testing.md"}, files)
	})
	t.Run("block_counts", func(t *testing.T) {
		counts := map[string]int{}
		for _, b := range g.blocks {
			counts[b.File]++
		}
		assert.Equal(t, map[string]int{"README.md": 7, "docs/architecture.md": 7, "docs/configuration.md": 6, "docs/errors.md": 8, "docs/testing.md": 17}, counts)
		assert.Empty(t, g.errs)
	})
	t.Run("skips", func(t *testing.T) {
		type skip struct {
			File    string
			Ordinal int
			Reason  string
		}
		var got []skip
		for _, b := range g.blocks {
			if b.Mode == "skip" {
				got = append(got, skip{b.File, b.Ordinal, b.Reason})
			}
		}
		assert.Equal(t, []skip{
			{"docs/architecture.md", 6, "imports k8s.io/client-go, which is not a galigo dependency"},
			{"docs/testing.md", 16, "a sketch of a type in package engine; the real one is cmd/galigo-testbot/engine/steps_geo.go"},
		}, got)
	})
	t.Run("build", func(t *testing.T) {
		require.Truef(t, g.ran, "build not run: %d extraction errors", len(g.errs))
		assert.Equal(t, docTargets(43), argsTail(g.args, 43))
		assert.Empty(t, g.reports)
	})
	t.Run("no_repo_write", func(t *testing.T) {
		assert.Truef(t, errors.Is(statErr, fs.ErrNotExist), "docsnippetchk in the repo root: %v", statErr)
	})
}

const docFixtures = "testdata/docsnippets_b110"

const docNote = "a package-load error stopped the build; the other blocks were not checked"

// docGroupA is the expected report list of each group-A fixture (go1.27.4).
var docGroupA = []struct {
	name string
	want []docErr
}{
	{"F1_after_use", []docErr{{"", 6, 5, "undefined: nope"}}},
	{"F1_decls_type", []docErr{{"", 7, 27, "undefined: nope"}}},
	{"F1_file_type", []docErr{{"", 7, 9, "undefined: nope"}}},
	{"F1_func_type", []docErr{{"", 7, 5, "undefined: nope"}}},
	{"F1_indented", []docErr{{"", 8, 8, "undefined: nope"}}},
	{"F1_params_typo", []docErr{{"", 3, 29, "undefined: galigo.Bott"}}},
	{"F1_return", nil},
	{"F1_same_line", []docErr{{"", 5, 13, "undefined: missing"}}},
	{"F1_syntax", []docErr{{"", 6, 6, "syntax error: unexpected ) at end of statement"}}},
	{"F2_chan_nested", []docErr{{"", 6, 2, "declared and not used: unused"}}},
	{"F2_chan_top", []docErr{{"", 5, 1, "channel element type too large (>64kB)"}}},
	{"F2_noBody", []docErr{{"", 6, 2, "declared and not used: unused"}}},
	{"F2_noBody_used", []docErr{{"", 10, 6, "missing function body"}}},
	{"F3_bare_const", []docErr{{"", 5, 1, `tg.ParseModeMarkdown (constant "Markdown" of string type tg.ParseMode) is not used`}}},
	{"F3_nested", []docErr{{"", 8, 2, "declared and not used: msg"}, {"", 8, 7, "declared and not used: err"}}},
	{"F3_range", []docErr{{"", 5, 5, "declared and not used: k"}, {"", 5, 8, "declared and not used: v"}}},
	{"F3_top", nil},
	{"F4_alias_in_body", []docErr{{"", 9, 5, "undefined: docp_sender"}}},
	{"F4_same_path_used", nil},
	{"F4_shown_gap", []docErr{{"", 9, 5, "undefined: sender"}}},
	{"F4_shown_unused", []docErr{{"", 5, 8, `"github.com/prilive-com/galigo/sender" imported and not used`}}},
	{"F5_no_main", []docErr{{"", 9, 0, "runtime.main_main·f: function main is undeclared in the main package"}}},
}

func docFixturePath(group, name string) string { return docFixtures + "/" + group + "/" + name + ".md" }

func docWantA() []docErr {
	var all []docErr
	for _, row := range docGroupA {
		for _, e := range row.want {
			e.File = docFixturePath("a", row.name)
			all = append(all, e)
		}
	}
	sortDocErrs(all)
	return all
}

func docOnly(reports []docErr, file string) []docErr {
	var out []docErr
	for _, e := range reports {
		if e.File == file {
			out = append(out, e)
		}
	}
	return out
}

// TestDocGoBlocksDetects builds fixtures with known defects and compares the reports.
func TestDocGoBlocksDetects(t *testing.T) {
	root := docRepoRoot(t)
	goPath := requireGo(t, exec.LookPath)
	var filesA []string
	for _, row := range docGroupA {
		filesA = append(filesA, docFixturePath("a", row.name))
	}
	a := runDocGroup(goPath, root, filesA, childEnv(os.Environ()))
	b := runDocGroup(goPath, root, []string{docFixturePath("b", "F6_load")}, childEnv(os.Environ()))
	goenv := filepath.Join(t.TempDir(), "goenv")
	goenvErr := os.WriteFile(goenv, []byte("GOFLAGS=-n\n"), 0o600)
	a7 := runDocGroup(goPath, root, filesA, childEnv(append(os.Environ(), "GOENV="+goenv)))
	var ctlEnv []string // the same GOENV file, with no GOFLAGS override
	for _, kv := range childEnv(append(os.Environ(), "GOENV="+goenv)) {
		if !strings.HasPrefix(kv, "GOFLAGS=") {
			ctlEnv = append(ctlEnv, kv)
		}
	}
	a7c := runDocGroup(goPath, root, filesA, ctlEnv)
	ran := func(t *testing.T, g docGroup) {
		t.Helper()
		require.Empty(t, g.errs)
		require.True(t, g.ran, "build not run")
	}

	t.Run("group_A_all", func(t *testing.T) {
		ran(t, a)
		assert.Equal(t, docWantA(), a.reports)
	})
	t.Run("group_A_args", func(t *testing.T) {
		ran(t, a)
		var built []string
		for _, bl := range a.built {
			built = append(built, bl.File)
		}
		assert.Equal(t, filesA, built)
		assert.Equal(t, docTargets(len(filesA)), argsTail(a.args, len(filesA)))
	})
	for _, row := range docGroupA {
		t.Run(row.name, func(t *testing.T) {
			ran(t, a)
			file := docFixturePath("a", row.name)
			var want []docErr
			for _, e := range row.want {
				e.File = file
				want = append(want, e)
			}
			assert.Equal(t, want, docOnly(a.reports, file))
		})
	}
	t.Run("F6_load", func(t *testing.T) {
		ran(t, b)
		file := docFixturePath("b", "F6_load")
		assert.Equal(t, []docErr{
			{file, 5, 8, "cannot find module providing package example.invalid/nope: import lookup disabled by -mod=readonly"},
			{docOutput, 0, 0, docNote},
		}, b.reports)
	})
	t.Run("F7_goenv", func(t *testing.T) {
		require.NoError(t, goenvErr)
		ran(t, a7)
		assert.Equal(t, docWantA(), a7.reports)
	})
	t.Run("F7_goenv_control", func(t *testing.T) {
		require.NoError(t, goenvErr)
		ran(t, a7c)
		seen := false
		for _, e := range a7c.reports {
			seen = seen || strings.Contains(e.Msg, "$WORK")
		}
		assert.Truef(t, seen, "the GOENV file's GOFLAGS=-n did not reach the child build: %v", a7c.reports)
	})
}

// --- U1, U2, U3: extractBlocks ---

func TestDocGoBlocksExtract(t *testing.T) {
	const d = "<!-- go-check: func() -->\n"
	rows := []struct {
		name, src string
		want      []docErr
	}{
		{"no_directive", "```go\nx := 1\n```\n", []docErr{{"x.md", 1, 0, "Go fence without a go-check directive"}}},
		{"blank_after_directive", d + "\n```go\nx\n```\n", []docErr{
			{"x.md", 1, 0, "go-check directive not followed by a Go fence"},
			{"x.md", 3, 0, "Go fence without a go-check directive"},
		}},
		{"directive_then_bash", d + "```bash\necho\n```\n", []docErr{{"x.md", 1, 0, "go-check directive not followed by a Go fence"}}},
		{"unknown_mode", "<!-- go-check: run -->\n```go\nx\n```\n", []docErr{{"x.md", 1, 0, `unknown go-check mode "run"`}}},
		{"skip_no_reason", "<!-- go-check: skip -->\n```go\nx\n```\n", []docErr{{"x.md", 1, 0, "go-check skip needs a reason"}}},
		{"bad_signature", "<!-- go-check: func(a b c) -->\n```go\nx\n```\n", []docErr{{"x.md", 1, 0, "go-check func signature does not parse: func(a b c)"}}},
		{"unclosed_go_fence", d + "```go\nx\n", []docErr{{"x.md", 2, 0, "unclosed fence"}}},
		{"list_go_unclosed", "- item\n\n  " + d + "  ```go\n  x := 1\n\nafter\n", []docErr{{"x.md", 4, 0, "unclosed fence"}}},
		{"indent_mismatch", d + "   ```go\n   x\n   ```\n", []docErr{{"x.md", 1, 0, "go-check directive indent differs from its Go fence"}}},
		{"bad_directive", "<!-- go-check func() -->\n```go\nx\n```\n", []docErr{
			{"x.md", 1, 0, "malformed go-check directive"},
			{"x.md", 2, 0, "Go fence without a go-check directive"},
		}},
		{"comment_open_at_eof", "<!-- a note\n```go\nx\n```\n", []docErr{{"x.md", 1, 0, "HTML comment not closed"}}},
		{"tilde_go", "~~~go\nx\n~~~\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: ~~~go"}}},
		{"four_ticks_go", "````go\nx\n````\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: ````go"}}},
		{"upper_Go", "```Go\nx\n```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: ```Go"}}},
		{"golang", "```golang\nx\n```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: ```golang"}}},
		{"go_title", "```go title\nx\n```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: ```go title"}}},
		{"quote_go", "> ```go\n> x\n> ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: > ```go"}}},
		{"quote_nested", "> > ```go\n> > x\n> > ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: > > ```go"}}},
		{"quote_tight", ">> ```go\n>> x\n>> ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: >> ```go"}}},
		{"quote_indented_code", ">     ```bash\n> ```go\n> x\n> ```\n", []docErr{{"x.md", 2, 0, "unsupported Go fence form: > ```go"}}},
		{"go_tab_title", "```go\ttitle\nx\n```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: ```go\ttitle"}}},
		{"list_marker_go", "- ```go\n  x\n  ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: - ```go"}}},
		{"quote_directive", "> <!-- go-check: func() -->\n> ```go\n> x\n> ```\n", []docErr{{"x.md", 2, 0, "unsupported Go fence form: > ```go"}}},
		{"quote_bad_directive", "> <!-- go-check func() -->\n", nil},
		{"quote_nested_spaced", ">  > ```go\n>  > x\n>  > ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: >  > ```go"}}},
		{"list_marker_star", "* ```go\n  x\n  ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: * ```go"}}},
		{"list_marker_plus", "+ ```go\n  x\n  ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: + ```go"}}},
		{"list_marker_ordered", "1. ```go\n   x\n   ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: 1. ```go"}}},
		{"list_marker_paren", "1) ```go\n   x\n   ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: 1) ```go"}}},
		{"list_quote_go", "- > ```go\n  > x := nope\n  > ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: - > ```go"}}},
		{"nested_list_go", "- - ```go\n    x := 1\n    ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: - - ```go"}}},
		{"quote_tab_go", "> \t```go\n> x\n> ```\n", []docErr{{"x.md", 1, 0, "unsupported Go fence form: > \t```go"}}},
		{"quote_tab_code", ">\t  ```bash\n> ```go\n> x\n> ```\n", []docErr{{"x.md", 2, 0, "unsupported Go fence form: > ```go"}}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			_, errs := extractBlocks("x.md", []byte(row.src))
			assert.Equal(t, row.want, errs)
		})
	}
}

// docShape is the part of a docBlock the fence rows compare.
type docShape struct {
	Fence, First int
	Mode         string
	Lines        []string
}

func docShapes(blocks []docBlock) []docShape {
	var out []docShape
	for _, b := range blocks {
		out = append(out, docShape{b.Fence, b.First, b.Mode, b.Lines})
	}
	return out
}

func TestDocGoBlocksFences(t *testing.T) {
	const goBlock = "<!-- go-check: func() -->\n```go\nx := 1\n```\n"
	at := func(fence int) []docShape { return []docShape{{fence, fence + 1, "func", []string{"x := 1"}}} }
	rows := []struct {
		name, src string
		want      []docShape
	}{
		{"go_in_bash", "```bash\n<!-- go-check: func() -->\n```go\n```\n\n" + goBlock, at(7)},
		{"quote_in_bash", "```bash\n> ```go\n```\n\n" + goBlock, at(6)},
		{"quote_bash_quoted", "> ```bash\n> ```go\n> ```\n\n" + goBlock, at(6)},
		{"inline_ticks_line", "```go`x`\n\n" + goBlock, at(4)},
		{"go_in_four_ticks", "````text\n```go\n````\n\n" + goBlock, at(6)},
		{"go_in_tilde", "~~~\n```go\n~~~\n\n" + goBlock, at(6)},
		{"ticks_in_tilde", "~~~\n```\n" + goBlock + "~~~\n\n" + goBlock, at(10)},
		{"short_in_four", "````\n```\n" + goBlock + "````\n\n" + goBlock, at(10)},
		{"go_in_comment", "<!--\n```go\nx\n```\n-->\n\n" + goBlock, at(8)},
		{"midline_comment", "Text with `<!--` in it.\n\n" + goBlock, at(4)},
		{"closer_trailing_space", "<!-- go-check: func() -->\n```go\nx := 1\n```  \n\nafter\n", at(2)},
		{"quote_comment", "> <!--\n> ```go\n> -->\n\n" + goBlock, at(6)},
		{"quote_comment_unclosed", "> <!--\n\n" + goBlock, at(4)},
		{"quote_ends_at_blank", "> ```bash\n> echo\n\n" + goBlock, at(5)},
		{"quote_ticks_in_bash", "```bash\n> ```\n```\n\n" + goBlock, at(6)},
		{"list_marker_bash", "- ```bash\n  echo\n  ```\n\n" + goBlock, at(6)},
		{"indent4_closer_top", "```markdown\n1. Step\n    ```go\n    code\n    ```\n```\n\n" + goBlock, at(9)},
		{"indent_closer_list", "1. Step\n\n   ```bash\n   echo\n       ```\n   ```\n\n" + goBlock, at(9)},
		{"root_indent_bash", "    ```bash\n\n" + goBlock, at(4)},
		{"list_outdent", "- ```bash\n  echo\n\n" + goBlock, at(5)},
		{"nested_list_bash", "- - ```bash\n    echo\n    ```\n\n" + goBlock, at(6)},
		{"list_ended_fence", "- item\n\nafter\n\n  ```bash\n  echo\n```\n\n" + goBlock, at(10)},
		{"list_then_fence", "- item\n```bash\necho\n```\n\n" + goBlock, at(7)},
		{"list_fence_outdent", "- item\n\n  ```bash\n  echo\n\n" + goBlock, at(7)},
		{"list_item", "- item\n\n   <!-- go-check: func() -->\n   ```go\n   x := 1\n   ```\n", []docShape{{4, 5, "func", []string{"   x := 1"}}}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			blocks, errs := extractBlocks("x.md", []byte(row.src))
			assert.Empty(t, errs)
			assert.Equal(t, row.want, docShapes(blocks))
		})
	}
}

func TestDocGoBlocksCRLF(t *testing.T) {
	t.Run("same_blocks", func(t *testing.T) {
		const lf = "- item\n\n   <!-- go-check: func() -->\n   ```go\n   x := 1\n   ```\n"
		b1, e1 := extractBlocks("x.md", []byte(lf))
		b2, e2 := extractBlocks("x.md", []byte(strings.ReplaceAll(lf, "\n", "\r\n")))
		assert.Empty(t, e1)
		assert.Empty(t, e2)
		require.Len(t, b1, 1)
		assert.Equal(t, b1, b2)
	})
}

// --- U4: generate ---

func TestDocGoBlocksGenerate(t *testing.T) {
	block := func(directive, body string) string {
		return "<!-- go-check: " + directive + " -->\n```go\n" + body + "```\n"
	}
	const head = "package docblk\n"
	const fn = "//line /r/x.md:1:1\nfunc docBlock00() {\n//line /r/x.md:3:1\n"
	rows := []struct {
		name, md, want string
	}{
		{"func_wrapper", block("func(ctx context.Context) error", "import \"errors\"\n\n_ = ctx\nreturn errors.New(\"x\")\n"),
			head + "import (\n\tdocp_context \"context\"\n)\n//line /r/x.md:3:1\nimport \"errors\"\n//line /r/x.md:1:1\nfunc docBlock00(ctx docp_context.Context) error {\n//line /r/x.md:4:1\n\n_ = ctx\nreturn errors.New(\"x\")\n}\n"},
		{"use_define", block("func()", "msg, err := f()\n"),
			head + fn + "msg, err := f(); _, _ = msg, err /*line /r/x.md:3:16*/\n}\n"},
		{"use_blank", block("func()", "_, err := f()\n"),
			head + fn + "_, err := f(); _ = err /*line /r/x.md:3:14*/\n}\n"},
		{"use_var", block("func()", "var a, b = f()\n"),
			head + fn + "var a, b = f(); _, _ = a, b /*line /r/x.md:3:15*/\n}\n"},
		{"use_blank_var", block("func()", "var _ = f()\n"),
			head + fn + "var _ = f()\n}\n"},
		{"use_return", block("func() int", "x := 1; return x\n"),
			head + "//line /r/x.md:1:1\nfunc docBlock00() int {\n//line /r/x.md:3:1\nx := 1; _ = x /*line /r/x.md:3:7*/; return x\n}\n"},
		{"use_same_line", block("func()", "x := 1; _ = missing\n"),
			head + fn + "x := 1; _ = x /*line /r/x.md:3:7*/; _ = missing\n}\n"},
		{"use_multiline", block("func()", "msg, err := f(\n\t1,\n)\n"),
			head + fn + "msg, err := f(\n\t1,\n); _, _ = msg, err /*line /r/x.md:5:2*/\n}\n"},
		{"no_use_nested", block("func()", "if true {\n\tx := 1\n}\nselect {\ncase x := <-ch:\n}\n_ = func() {\n\ty := 2\n}\n"),
			head + fn + "if true {\n\tx := 1\n}\nselect {\ncase x := <-ch:\n}\n_ = func() {\n\ty := 2\n}\n}\n"},
		{"parse_error_passthrough", block("func()", "x := time.Second\n_ = T{...}\n"),
			head + fn + "x := time.Second\n_ = T{...}\n}\n"},
		{"file_verbatim", block("file", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println() }\n"),
			"//line /r/x.md:3:1\npackage main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println() }\n"},
		{"plain_imports", block("func()", "_ = time.Second\n_ = fmt.Sprint()\n"),
			head + "import (\n\t\"fmt\"\n\t\"time\"\n)\n" + fn + "_ = time.Second\n_ = fmt.Sprint()\n}\n"},
		{"scope_after_if", block("func()", "if time := 1; time > 0 {\n}\n_ = time.Second\n"),
			head + "import (\n\t\"time\"\n)\n" + fn + "if time := 1; time > 0 {\n}\n_ = time.Second\n}\n"},
		{"shadow_local", block("func()", "log := slog.Default(); log.Info(\"x\")\n"),
			head + "import (\n\t\"log/slog\"\n)\n" + fn + "log := slog.Default(); _ = log /*line /r/x.md:3:22*/; log.Info(\"x\")\n}\n"},
		{"type_switch_case", block("func(x any)", "switch time := x.(type) {\ncase time.Duration:\n\t_ = time\n}\n"),
			head + "import (\n\t\"time\"\n)\n//line /r/x.md:1:1\nfunc docBlock00(x any) {\n//line /r/x.md:3:1\nswitch time := x.(type) {\ncase time.Duration:\n\t_ = time\n}\n}\n"},
		{"alias_params", block("func(client *sender.Client)", "import \"github.com/prilive-com/galigo/tg\"\n\n_ = tg.ParseModeHTML\n_ = sender.SendMessageRequest{}\n"),
			head + "import (\n\tdocp_sender \"github.com/prilive-com/galigo/sender\"\n)\n//line /r/x.md:3:1\nimport \"github.com/prilive-com/galigo/tg\"\n//line /r/x.md:1:1\nfunc docBlock00(client *docp_sender.Client) {\n//line /r/x.md:4:1\n\n_ = tg.ParseModeHTML\n_ = sender.SendMessageRequest{}\n}\n"},
		{"alias_results", block("func() *sender.Client", "import \"fmt\"\n\n_ = fmt.Sprint()\nreturn nil\n"),
			head + "import (\n\tdocp_sender \"github.com/prilive-com/galigo/sender\"\n)\n//line /r/x.md:3:1\nimport \"fmt\"\n//line /r/x.md:1:1\nfunc docBlock00() *docp_sender.Client {\n//line /r/x.md:4:1\n\n_ = fmt.Sprint()\nreturn nil\n}\n"},
		{"alias_unique", block("func(client *sender.Client)", "import \"fmt\"\n\ndocp_sender := fmt.Sprint(client)\n"),
			head + "import (\n\tdocp_sender1 \"github.com/prilive-com/galigo/sender\"\n)\n//line /r/x.md:3:1\nimport \"fmt\"\n//line /r/x.md:1:1\nfunc docBlock00(client *docp_sender1.Client) {\n//line /r/x.md:4:1\n\ndocp_sender := fmt.Sprint(client); _ = docp_sender /*line /r/x.md:5:34*/\n}\n"},
		{"import_comment_closer", block("func()", "import (\n\t\"fmt\"\n) // fmt\n\n_ = fmt.Sprint()\n"),
			head + "//line /r/x.md:3:1\nimport (\n\t\"fmt\"\n) // fmt\n//line /r/x.md:1:1\nfunc docBlock00() {\n//line /r/x.md:6:1\n\n_ = fmt.Sprint()\n}\n"},
		{"import_two_decls", block("func()", "import \"fmt\"\nimport \"os\"\n\n_ = fmt.Sprint(os.Args)\n"),
			head + "//line /r/x.md:3:1\nimport \"fmt\"\nimport \"os\"\n//line /r/x.md:1:1\nfunc docBlock00() {\n//line /r/x.md:5:1\n\n_ = fmt.Sprint(os.Args)\n}\n"},
		{"import_same_line", block("func()", "import \"fmt\"; _ = fmt.Sprint(1)\n"),
			head + fn + "import \"fmt\"; _ = fmt.Sprint(1)\n}\n"},
		{"import_block_comment", block("func()", "import \"fmt\" /* a\nb */\n_ = fmt.Sprint()\n"),
			head + fn + "import \"fmt\" /* a\nb */\n_ = fmt.Sprint()\n}\n"},
		{"import_line_directive", block("func()", "//line demo.go:100\nimport \"fmt\"\n\n_ = fmt.Sprint()\n"),
			head + "//line /r/x.md:3:1\n//line demo.go:100\nimport \"fmt\"\n//line /r/x.md:1:1\nfunc docBlock00() {\n//line /r/x.md:5:1\n\n_ = fmt.Sprint()\n}\n"},
		{"import_empty_group", block("func()", "import ()\n\n_ = errors.New(\"x\")\n"),
			head + "//line /r/x.md:3:1\nimport ()\n//line /r/x.md:1:1\nfunc docBlock00() {\n//line /r/x.md:4:1\n\n_ = errors.New(\"x\")\n}\n"},
		{"import_not_leading", block("func()", "_ = 1\nimport \"fmt\"\n"),
			head + fn + "_ = 1\nimport \"fmt\"\n}\n"},
		{"names_in_comments", block("func()", "// time.Second in a comment\n_ = \"fmt.Sprint\"\n"),
			head + fn + "// time.Second in a comment\n_ = \"fmt.Sprint\"\n}\n"},
	}
	gen := func(t *testing.T, md string) string {
		t.Helper()
		blocks, errs := extractBlocks("x.md", []byte(md))
		require.Empty(t, errs)
		require.Len(t, blocks, 1)
		blocks[0].Abs = "/r/x.md"
		src, err := generate(blocks[0], 0)
		require.NoError(t, err)
		return src
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			assert.Equal(t, row.want, gen(t, row.md))
		})
	}
	t.Run("decls_file_no_use", func(t *testing.T) {
		const decls = "var x = 1\n\nfunc f() {\n\ty := 2\n}\n"
		assert.Equal(t, head+"//line /r/x.md:3:1\n"+decls, gen(t, block("decls", decls)))
		assert.Equal(t, "//line /r/x.md:3:1\npackage p\n\nvar x = 1\n", gen(t, block("file", "package p\n\nvar x = 1\n")))
	})
	t.Run("stray_brace", func(t *testing.T) {
		for _, body := range []string{"}\nfunc helper() {\n", "}\ntype T struct {\n"} {
			blocks, errs := extractBlocks("x.md", []byte(block("func()", body)))
			require.Empty(t, errs)
			require.Len(t, blocks, 1)
			blocks[0].Abs = "/r/x.md"
			var err error
			require.NotPanics(t, func() { _, err = generate(blocks[0], 0) }, body)
			assert.EqualError(t, err, "block is not a function body: its braces do not match the wrapper", body)
		}
	})
}

// --- U5: parseBuild ---

func TestDocGoBlocksParse(t *testing.T) {
	var blocks []docBlock
	for k := range 8 {
		blocks = append(blocks, docBlock{File: "README.md", Fence: 10 * (k + 1)})
	}
	blocks[7].File = "docs/x.md"
	const grp = "# github.com/prilive-com/galigo/docsnippetchk/"
	const mod = "github.com/prilive-com/galigo/docsnippetchk/"
	raw := func(rc int, out string) docErr {
		return docErr{docOutput, 0, 0, fmt.Sprintf("go build output not attributed (exit %d):\n%s", rc, out)}
	}
	note := docErr{docOutput, 0, 0, docNote}
	const unknownG = "# github.com/prilive-com/galigo/sender\nsender/x.go:1:1: oops\n" + grp + "b00\n./README.md:12:1: msg\n"
	const loadCycle = "package " + mod + "b02\n\timports " + mod + "b03\n\timports " + mod + "b02: import cycle not allowed\n"
	const sortTail = "docs/x.md:5:1: load\n"
	rows := []struct {
		name, root, out string
		rc              int
		want            []docErr
	}{
		{"col", "/r", grp + "b00\n./README.md:12:1: msg\n", 1, []docErr{{"README.md", 12, 1, "msg"}}},
		{"no_col", "/r", grp + "b00\n./README.md:12: msg\n", 1, []docErr{{"README.md", 12, 0, "msg"}}},
		{"no_dot", "/r", grp + "b00\nREADME.md:12:1: msg\n", 1, []docErr{{"README.md", 12, 1, "msg"}}},
		{"drive_slash", "C:/r", grp + "b00\nC:/r/README.md:12:3: msg\n", 1, []docErr{{"README.md", 12, 3, "msg"}}},
		{"md_backslash", "C:/r", grp + "b00\nC:\\r\\README.md:12:3: msg\n", 1, []docErr{{"README.md", 12, 3, "msg"}}},
		{"temp_path", "/r", grp + "b03\n../ov/b03.go:4:20: undefined: x\n", 1, []docErr{{"README.md", 40, 0, "undefined: x"}}},
		{"temp_path_windows", "C:/r", grp + "b03\nC:\\Users\\x\\Temp\\T\\001\\b03.go:4:20: undefined: x\n", 1, []docErr{{"README.md", 40, 0, "undefined: x"}}},
		{"virtual_ungrouped", "/r", "docsnippetchk/b06/s.go:2:8: cannot find module providing package x\n", 1,
			[]docErr{{"README.md", 70, 0, "cannot find module providing package x"}, note}},
		{"virtual_backslash_ungrouped", "C:/r", "docsnippetchk\\b06\\s.go:2:8: cannot find module providing package x\n", 1,
			[]docErr{{"README.md", 70, 0, "cannot find module providing package x"}, note}},
		{"no_path_line", "/r", grp + "b04\nruntime.main_main·f: function main is undeclared in the main package\n", 1,
			[]docErr{{"README.md", 50, 0, "runtime.main_main·f: function main is undeclared in the main package"}}},
		{"continuation", "/r", grp + "b00\n./README.md:12:5: cannot use x\n\thave (int)\n\twant (string)\n", 1,
			[]docErr{{"README.md", 12, 5, "cannot use x\n\thave (int)\n\twant (string)"}}},
		{"load_md", "/r", "docs/x.md:10:8: cannot find module providing package example.invalid/nope: import lookup disabled by -mod=readonly\n", 1,
			[]docErr{{"docs/x.md", 10, 8, "cannot find module providing package example.invalid/nope: import lookup disabled by -mod=readonly"}, note}},
		{"load_package", "/r", "package " + mod + "b02: build constraints exclude all Go files in /r/docsnippetchk/b02\n", 1,
			[]docErr{{"README.md", 30, 0, "package " + mod + "b02: build constraints exclude all Go files in /r/docsnippetchk/b02"}, note}},
		{"load_cycle", "/r", loadCycle, 1, []docErr{{"README.md", 30, 0, strings.TrimSuffix(loadCycle, "\n")}, note}},
		{"unknown_grouped", "/r", unknownG, 1, []docErr{{"README.md", 12, 1, "msg"}, raw(1, unknownG)}},
		{"unknown_ungrouped", "/r", "go: something odd\n", 1, []docErr{raw(1, "go: something odd\n"), note}},
		{"rc1_empty", "/r", "", 1, []docErr{raw(1, "")}},
		{"rc0_error", "/r", grp + "b00\n./README.md:12:1: msg\n", 0, []docErr{{"README.md", 12, 1, "msg"}}},
		{"unused_only", "/r", grp + "b00\n./README.md:12:2: declared and not used: x\n" + grp + "b01\n./README.md:22:2: declared and not used: x\n", 1,
			[]docErr{{"README.md", 12, 2, "declared and not used: x"}, {"README.md", 22, 2, "declared and not used: x"}}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			assert.Equal(t, row.want, parseBuild(row.out, row.rc, row.root, blocks))
		})
	}
	t.Run("sorted", func(t *testing.T) {
		g0 := grp + "b00\n./README.md:12:5: a2\n./README.md:12:2: a1\n"
		g1 := grp + "b01\n./README.md:22:3: b\n"
		g2 := grp + "b02\n./README.md:32:1: c\n"
		want := []docErr{
			{"README.md", 12, 2, "a1"}, {"README.md", 12, 5, "a2"}, {"README.md", 22, 3, "b"}, {"README.md", 32, 1, "c"},
			{"docs/x.md", 5, 1, "load"}, note,
		}
		assert.Equal(t, want, parseBuild(sortTail+g1+g0+g2, 1, "/r", blocks))
		assert.Equal(t, want, parseBuild(sortTail+g2+g0+g1, 1, "/r", blocks))
	})
}

// --- U6, U7, U8 ---

func TestDocGoBlocksEnv(t *testing.T) {
	t.Run("override", func(t *testing.T) {
		base := []string{"PATH=/bin", "GOFLAGS=-n", "GOWORK=/x/go.work", "GOTOOLCHAIN=go1.27.4", "HOME=/h"}
		assert.Equal(t, []string{"PATH=/bin", "HOME=/h", "GOFLAGS=-mod=readonly -buildvcs=false", "GOWORK=off", "GOTOOLCHAIN=local"}, childEnv(base))
	})
}

func TestDocGoBlocksVirtual(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "docsnippetchk"), 0o700))
		assert.Error(t, checkVirtualAbsent(dir))
	})
	t.Run("absent", func(t *testing.T) {
		assert.NoError(t, checkVirtualAbsent(t.TempDir()))
	})
}

type docFakeTB struct{ fatal, skip int }

func (f *docFakeTB) Fatalf(string, ...any) { f.fatal++ }
func (f *docFakeTB) Skipf(string, ...any)  { f.skip++ }

func TestDocGoBlocksRequireGo(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		var tb docFakeTB
		got := requireGo(&tb, func(string) (string, error) { return "", exec.ErrNotFound })
		assert.Equal(t, docFakeTB{fatal: 1}, tb)
		assert.Empty(t, got)
	})
	t.Run("found", func(t *testing.T) {
		var tb docFakeTB
		got := requireGo(&tb, func(string) (string, error) { return "/usr/bin/go", nil })
		assert.Equal(t, docFakeTB{}, tb)
		assert.Equal(t, "/usr/bin/go", got)
	})
}

// --- the checker ---

// docBlock is one fenced go block found in a Markdown file.
type docBlock struct {
	File    string   // Markdown path, slash form, relative to the repo root
	Abs     string   // absolute Markdown path, slash form, for //line (set by the caller)
	Ordinal int      // 1-based number of the Go block in File
	DirLine int      // line of the directive (0 = none)
	Fence   int      // line of the opening fence
	First   int      // line of the first content line
	Mode    string   // "file", "decls", "func" or "skip"; "" without a valid directive
	Sig     string   // for "func": the signature after "func"
	Reason  string   // for "skip"
	Lines   []string // content lines as written
}

// docErr is one report: a checker error or a compiler message at its Markdown place.
type docErr struct {
	File      string
	Line, Col int // Col 0 = none printed
	Msg       string
}

// genFile is one generated Go file; Dir is its virtual package directory (bNN).
type genFile struct{ Dir, Src string }

// docOutput is the place of a report that names no Markdown line.
const docOutput = "<go build output>"

var docDirectiveRE = regexp.MustCompile(`^<!-- go-check: (.*) -->$`)

// docFence returns the fence character, run length and info string of an
// opener line (text after indent and quote markers), or ok false.
func docFence(text string) (ch byte, n int, info string, ok bool) {
	text = docTrimIndent(text)
	if text == "" || (text[0] != '`' && text[0] != '~') {
		return 0, 0, "", false
	}
	ch = text[0]
	for n < len(text) && text[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	info = strings.TrimSpace(text[n:])
	if ch == '`' && strings.Contains(info, "`") {
		return 0, 0, "", false
	}
	return ch, n, info, true
}

// docExpandTabs replaces each tab with spaces up to the next tab stop of 4
// (CommonMark 2.2). Only the copy used to find fences is expanded.
func docExpandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	for _, c := range []byte(s) {
		if c == '\t' {
			b.WriteString("    "[b.Len()%4:])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// docTrimIndent removes up to 3 leading spaces (CommonMark: 4 start indented code).
func docTrimIndent(s string) string {
	i := 0
	for i < 3 && i < len(s) && s[i] == ' ' {
		i++
	}
	return s[i:]
}

// docQuote removes the indent and up to limit block-quote markers (all for limit < 0).
// A nested marker may have up to 3 spaces before it; each marker takes one optional
// space after it, and the text keeps any further indent (CommonMark 5.1). The
// caller expands tabs first.
func docQuote(line string, limit int) (depth int, text string) {
	text = strings.TrimLeft(line, " \t")
	for limit < 0 || depth < limit {
		rest := text
		if depth > 0 {
			rest = docTrimIndent(text)
		}
		if !strings.HasPrefix(rest, ">") {
			break
		}
		depth++
		text = rest[1:]
		text = strings.TrimPrefix(text, " ")
	}
	return depth, text
}

// docListRE matches a list marker and the spaces after it (CommonMark 5.2).
var docListRE = regexp.MustCompile(`^(?:[-+*]|\d{1,9}[.)])[ \t]+`)

func docGoish(info string) bool {
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return false
	}
	word := strings.ToLower(fields[0])
	return word == "go" || word == "golang"
}

// docMode parses a directive's MODE text.
func docMode(b *docBlock, mode string) string {
	switch {
	case mode == "file" || mode == "decls":
		b.Mode = mode
	case mode == "skip" || strings.HasPrefix(mode, "skip "):
		b.Reason = strings.TrimSpace(strings.TrimPrefix(mode, "skip"))
		if b.Reason == "" {
			return "go-check skip needs a reason"
		}
		b.Mode = "skip"
	case strings.HasPrefix(mode, "func("):
		if e, err := parser.ParseExpr(mode); err != nil {
			return "go-check func signature does not parse: " + mode
		} else if _, ok := e.(*ast.FuncType); !ok {
			return "go-check func signature does not parse: " + mode
		}
		b.Mode, b.Sig = "func", strings.TrimPrefix(mode, "func")
	default:
		return fmt.Sprintf("unknown go-check mode %q", mode)
	}
	return ""
}

// extractBlocks finds the go blocks of one Markdown file (PLAN.md §4, Markdown rules).
func extractBlocks(name string, src []byte) ([]docBlock, []docErr) {
	var blocks []docBlock
	var errs []docErr
	fail := func(line int, msg string) { errs = append(errs, docErr{File: name, Line: line, Msg: msg}) }
	type fence struct {
		ch           byte
		n, depth, at int
		col          int       // at depth 0: the column of its container (a list item), 0 at top level
		block        *docBlock // nil for a fence that is not a Go block
	}
	type directive struct {
		line   int
		indent string
		block  docBlock
		ok     bool
	}
	type htmlComment struct{ line, depth int }
	var open *fence
	var dir *directive
	var comment *htmlComment // an open HTML comment
	listCol := -1            // content column of the open list item at depth 0 (-1 = none)
	prevBlank := false
	ordinal := 0
	lines := strings.Split(string(src), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		ln := i + 1
		line = strings.TrimSuffix(line, "\r")
		x := docExpandTabs(line) // the copy used to find fences and markers
		depth, text := docQuote(x, -1)
		ind := len(x) - len(strings.TrimLeft(x, " ")) // indent at depth 0
		blank, wasBlank := strings.TrimSpace(x) == "", prevBlank
		prevBlank = blank
		if comment != nil {
			if depth >= comment.depth {
				if strings.Contains(line, "-->") {
					comment = nil
				}
				continue
			}
			comment = nil // the block quote ended, and its comment with it
		}
		if open != nil && depth >= open.depth && open.depth == 0 && !blank && ind < open.col {
			if open.block != nil { // the list item ended, and its fence with it
				fail(open.at, "unclosed fence")
			}
			open = nil
		}
		if open != nil {
			if depth >= open.depth {
				_, inner := docQuote(x, open.depth)
				if ch, n, info, ok := docFence(inner); ok && ch == open.ch && n >= open.n && info == "" &&
					(open.depth > 0 || ind <= open.col+3) {
					if open.block != nil {
						blocks = append(blocks, *open.block)
					}
					open = nil
					continue
				}
				if open.block != nil {
					open.block.Lines = append(open.block.Lines, line)
				}
				continue
			}
			open = nil // the block quote ended, and its fence with it
		}
		if depth == 0 && !blank {
			if m := docListRE.FindString(text); m != "" {
				listCol = ind + len(m)
			} else if listCol >= 0 && ind < listCol && wasBlank {
				listCol = -1 // a less indented line after a blank line ends the list
			}
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		ch, n, info, isFence := docFence(text)
		col, fdepth := 0, depth
		if isFence && depth == 0 {
			if listCol >= 0 && ind < listCol {
				listCol = -1 // a fence less indented than the item ends the list
			}
			if listCol >= 0 {
				col = listCol
			}
			if ind-col >= 4 {
				isFence = false // indented code, not a fence (CommonMark 4.4)
			}
		}
		listFence := false
		for rest := text; !isFence; { // list and quote markers, in any order
			r := docTrimIndent(rest)
			if m := docListRE.FindString(r); m != "" {
				rest = r[len(m):]
			} else if strings.HasPrefix(r, ">") {
				rest = strings.TrimPrefix(r[1:], " ")
				fdepth++
			} else {
				break
			}
			if ch, n, info, isFence = docFence(rest); isFence {
				listFence = true
				if fdepth == 0 {
					col = len(x) - len(rest)
				}
			}
		}
		goOpener := isFence && !listFence && depth == 0 && ch == '`' && n == 3 && info == "go"
		if dir != nil && dir.line == ln-1 && !goOpener {
			fail(dir.line, "go-check directive not followed by a Go fence")
			dir = nil
		}
		if lead := docTrimIndent(text); strings.HasPrefix(lead, "<!--") {
			if m := docDirectiveRE.FindStringSubmatch(text); depth == 0 && m != nil {
				d := directive{line: ln, indent: indent}
				d.block.DirLine = ln
				if msg := docMode(&d.block, m[1]); msg != "" {
					fail(ln, msg)
				} else {
					d.ok = true
				}
				dir = &d
				continue
			}
			if depth == 0 && strings.HasPrefix(text, "<!-- go-check") {
				fail(ln, "malformed go-check directive")
			}
			if !strings.Contains(lead[4:], "-->") {
				comment = &htmlComment{ln, depth}
			}
			continue
		}
		if !isFence {
			continue
		}
		open = &fence{ch: ch, n: n, depth: fdepth, at: ln, col: col}
		if !goOpener {
			if docGoish(info) {
				fail(ln, "unsupported Go fence form: "+strings.TrimSpace(line))
			}
			continue
		}
		ordinal++
		b := docBlock{File: name, Ordinal: ordinal, Fence: ln, First: ln + 1}
		switch {
		case dir == nil || dir.line != ln-1:
			fail(ln, "Go fence without a go-check directive")
		case dir.indent != indent:
			fail(dir.line, "go-check directive indent differs from its Go fence")
		case dir.ok:
			b.DirLine, b.Mode, b.Sig, b.Reason = dir.block.DirLine, dir.block.Mode, dir.block.Sig, dir.block.Reason
		}
		dir = nil
		open.block = &b
	}
	if dir != nil {
		fail(dir.line, "go-check directive not followed by a Go fence")
	}
	if open != nil && open.depth == 0 {
		fail(open.at, "unclosed fence")
	}
	if comment != nil {
		fail(comment.line, "HTML comment not closed")
	}
	return blocks, errs
}

// docImports maps a package name a block may use without importing it to its path.
var docImports = map[string]string{
	"galigo": "github.com/prilive-com/galigo", "tg": "github.com/prilive-com/galigo/tg",
	"sender": "github.com/prilive-com/galigo/sender", "receiver": "github.com/prilive-com/galigo/receiver",
	"engine": "github.com/prilive-com/galigo/cmd/galigo-testbot/engine", "testutil": "github.com/prilive-com/galigo/internal/testutil",
	"context": "context", "time": "time", "slog": "log/slog", "log": "log", "errors": "errors", "fmt": "fmt",
	"http": "net/http", "httptest": "net/http/httptest", "os": "os", "signal": "os/signal", "syscall": "syscall",
	"testing": "testing", "assert": "github.com/stretchr/testify/assert", "require": "github.com/stretchr/testify/require",
	"json": "encoding/json", "strings": "strings", "sync": "sync", "atomic": "sync/atomic", "io": "io", "bytes": "bytes",
}

// docEdit replaces src[from:to] with text.
type docEdit struct {
	from, to int
	text     string
}

func docApply(src string, edits []docEdit) string {
	sort.Slice(edits, func(i, j int) bool { return edits[i].from > edits[j].from })
	for _, e := range edits {
		src = src[:e.from] + e.text + src[e.to:]
	}
	return src
}

// docUnresolvedBases returns the names X of selectors X.Sel that name no
// declaration in the file: go/parser's Unresolved, plus a type-switch case
// qualifier that resolves to the switch's own binding (PLAN.md §13.3).
func docUnresolvedBases(f *ast.File) map[string]bool {
	unresolved := map[*ast.Ident]bool{}
	for _, id := range f.Unresolved {
		unresolved[id] = true
	}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSwitchStmt)
		if !ok {
			return true
		}
		as, ok := ts.Assign.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 {
			return true
		}
		bind, ok := as.Lhs[0].(*ast.Ident)
		if !ok || bind.Obj == nil {
			return true
		}
		for _, st := range ts.Body.List {
			for _, e := range st.(*ast.CaseClause).List {
				ast.Inspect(e, func(n ast.Node) bool {
					if sel, ok := n.(*ast.SelectorExpr); ok {
						if id, ok := sel.X.(*ast.Ident); ok && id.Obj == bind.Obj {
							unresolved[id] = true
						}
					}
					return true
				})
			}
		}
		return true
	})
	bases := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && unresolved[id] {
				bases[id.Name] = true
			}
		}
		return true
	})
	return bases
}

func docImportDecl(specs []string) string {
	if len(specs) == 0 {
		return ""
	}
	sort.Strings(specs)
	return "import (\n\t" + strings.Join(specs, "\n\t") + "\n)\n"
}

// docImportLines returns how many leading lines of a func block hold its import
// declarations (0 = none), read with go/scanner so comments and layout do not matter.
// An import declaration counts only when the rest of its line is empty or a "//"
// comment; otherwise nothing is hoisted and the compiler reports the import.
func docImportLines(lines []string) int {
	src := strings.Join(lines, "\n")
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, []byte(src), nil, 0)
	end := 0
	for {
		_, tok, _ := s.Scan()
		for tok == token.SEMICOLON {
			_, tok, _ = s.Scan()
		}
		if tok != token.IMPORT {
			return end
		}
		pos, tok, lit := s.Scan()
		want := token.STRING
		if tok == token.LPAREN {
			want = token.RPAREN
		}
		for tok != want && tok != token.EOF {
			pos, tok, lit = s.Scan()
		}
		if tok == token.EOF {
			return end
		}
		at := file.Offset(pos) + max(len(lit), 1) // byte offsets: a //line comment does not move them
		rest, _, _ := strings.Cut(src[at:], "\n")
		rest = strings.TrimSpace(rest)
		if rest != "" && !strings.HasPrefix(rest, "//") {
			return 0
		}
		end = 1 + strings.Count(src[:at], "\n")
	}
}

// generate writes the Go file for block k (PLAN.md §4, Modes, Generated imports, Line mapping).
func generate(b docBlock, k int) (string, error) {
	lineDir := func(line int) string { return fmt.Sprintf("//line %s:%d:1\n", b.Abs, line) }
	body := strings.Join(b.Lines, "\n") + "\n"
	if len(b.Lines) == 0 {
		body = ""
	}
	switch b.Mode {
	case "file":
		return lineDir(b.First) + body, nil
	case "decls", "func":
	default:
		return "", fmt.Errorf("block mode %q is not built", b.Mode)
	}
	const pkg = "package docblk\n"
	var src string
	var hoistLines int
	if b.Mode == "decls" {
		src = pkg + lineDir(b.First) + body
	} else {
		hoistLines = docImportLines(b.Lines)
		src = pkg
		if hoistLines > 0 {
			src += lineDir(b.First) + strings.Join(b.Lines[:hoistLines], "\n") + "\n"
		}
		src += lineDir(b.DirLine) + fmt.Sprintf("func docBlock%02d", k) + b.Sig + " {\n"
		if hoistLines < len(b.Lines) {
			src += lineDir(b.First+hoistLines) + strings.Join(b.Lines[hoistLines:], "\n") + "\n"
		}
		src += "}\n"
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "s.go", src, 0)
	if err != nil {
		return src, nil //nolint:nilerr // the compiler reports the syntax error at its Markdown line
	}
	off := func(p token.Pos) int { return fset.PositionFor(p, false).Offset }
	if b.Mode == "func" {
		fd, ok := f.Decls[len(f.Decls)-1].(*ast.FuncDecl)
		if !ok || fd.Name.Name != fmt.Sprintf("docBlock%02d", k) {
			return "", errors.New("block is not a function body: its braces do not match the wrapper")
		}
	}
	shown := false // the block shows an import declaration, maybe an empty one
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			shown = true
		}
	}
	var edits []docEdit
	var specs []string
	if !shown {
		for name := range docUnresolvedBases(f) {
			if p, ok := docImports[name]; ok {
				specs = append(specs, strconv.Quote(p))
			}
		}
	} else if b.Mode == "func" {
		idents := map[string]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				idents[id.Name] = true
			}
			return true
		})
		fd := f.Decls[len(f.Decls)-1].(*ast.FuncDecl)
		alias := map[string]string{}
		ast.Inspect(fd.Type, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			p, ok := docImports[id.Name]
			if !ok {
				return true
			}
			if alias[id.Name] == "" {
				a := "docp_" + id.Name
				for i := 1; idents[a]; i++ {
					a = fmt.Sprintf("docp_%s%d", id.Name, i)
				}
				alias[id.Name] = a
				specs = append(specs, a+" "+strconv.Quote(p))
			}
			edits = append(edits, docEdit{off(id.Pos()), off(id.End()), alias[id.Name]})
			return true
		})
	}
	if b.Mode == "func" {
		fd := f.Decls[len(f.Decls)-1].(*ast.FuncDecl)
		for _, st := range fd.Body.List {
			var names []string
			switch st := st.(type) {
			case *ast.AssignStmt:
				if st.Tok == token.DEFINE {
					for _, e := range st.Lhs {
						if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
							names = append(names, id.Name)
						}
					}
				}
			case *ast.DeclStmt:
				if gd, ok := st.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
					for _, sp := range gd.Specs {
						for _, id := range sp.(*ast.ValueSpec).Names {
							if id.Name != "_" {
								names = append(names, id.Name)
							}
						}
					}
				}
			}
			if len(names) == 0 {
				continue
			}
			end := fset.PositionFor(st.End(), true)
			blanks := strings.TrimSuffix(strings.Repeat("_, ", len(names)), ", ")
			text := fmt.Sprintf("; %s = %s /*line %s:%d:%d*/", blanks, strings.Join(names, ", "), b.Abs, end.Line, end.Column)
			edits = append(edits, docEdit{off(st.End()), off(st.End()), text})
		}
	}
	src = docApply(src, edits)
	return pkg + docImportDecl(specs) + strings.TrimPrefix(src, pkg), nil
}

// buildBlocks runs one go build of the generated files through an overlay (PLAN.md §4, Build).
func buildBlocks(goPath, root string, files []genFile, env []string) (out string, rc int, args []string) {
	tmp, err := os.MkdirTemp("", "docsnippet")
	if err != nil {
		return err.Error(), -1, nil
	}
	defer os.RemoveAll(tmp)
	replace := map[string]string{}
	targets := make([]string, 0, len(files))
	for _, f := range files {
		p := filepath.Join(tmp, f.Dir+".go")
		if err = os.WriteFile(p, []byte(f.Src), 0o600); err != nil {
			return err.Error(), -1, nil
		}
		replace[filepath.Join(root, "docsnippetchk", f.Dir, "s.go")] = p
		targets = append(targets, "./docsnippetchk/"+f.Dir)
	}
	ov, err := json.Marshal(map[string]any{"Replace": replace})
	if err == nil {
		err = os.WriteFile(filepath.Join(tmp, "overlay.json"), ov, 0o600)
	}
	if err != nil {
		return err.Error(), -1, nil
	}
	argv := append([]string{"build", "-overlay=" + filepath.Join(tmp, "overlay.json"), "-gcflags=-e", "-o", os.DevNull}, targets...)
	cmd := exec.Command(goPath, argv...) // #nosec G204 -- goPath is the go that exec.LookPath found; the arguments are fixed flags and generated package paths
	cmd.Dir, cmd.Env = root, env
	b, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		rc = exit.ExitCode()
	case err != nil:
		return string(b) + err.Error(), -1, cmd.Args
	}
	return string(b), rc, cmd.Args
}

// normPath puts a printed path in the repo-relative slash form, the same way on every OS.
func normPath(p, root string) string {
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	r := strings.TrimSuffix(path.Clean(strings.ReplaceAll(root, `\`, "/")), "/")
	if strings.HasPrefix(p, r+"/") {
		p = p[len(r)+1:]
	}
	return strings.TrimPrefix(p, "./")
}

var (
	docErrLineRE = regexp.MustCompile(`^(.+?\.(?:md|go)):(\d+)(?::(\d+))?: (.*)$`)
	docGroupRE   = regexp.MustCompile(`(?:^|/)docsnippetchk/b(\d+)$`)
	docVirtualRE = regexp.MustCompile(`^docsnippetchk/b(\d+)/s\.go$`)
	docPackageRE = regexp.MustCompile(`^package (\S+?):?(?: |$)`)
)

func sortDocErrs(errs []docErr) {
	rank := func(e docErr) int {
		switch {
		case e.File != docOutput:
			return 0
		case e.Msg == docNote:
			return 2
		}
		return 1
	}
	sort.SliceStable(errs, func(i, j int) bool {
		a, b := errs[i], errs[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.Msg < b.Msg
	})
}

// parseBuild maps the go build output to Markdown places (PLAN.md §4, the output parser).
func parseBuild(out string, rc int, root string, blocks []docBlock) []docErr {
	md := map[string]bool{}
	for _, b := range blocks {
		md[b.File] = true
	}
	block := func(re *regexp.Regexp, s string) int {
		m := re.FindStringSubmatch(s)
		if m == nil {
			return -1
		}
		k, err := strconv.Atoi(m[1])
		if err != nil || k >= len(blocks) {
			return -1
		}
		return k
	}
	var reports []docErr
	raw, load := false, false
	group, inGroup := -1, false
	last := -1 // index of the report a continuation line belongs to
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "# "):
			inGroup, last = true, -1
			group = block(docGroupRE, normPath(line[2:], root))
			continue
		case strings.HasPrefix(line, "\t"):
			if last < 0 {
				raw = true
			} else {
				reports[last].Msg += "\n" + line
			}
			continue
		}
		if !inGroup {
			load = true
		}
		if m := docErrLineRE.FindStringSubmatch(line); m != nil {
			p := normPath(m[1], root)
			ln, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			if md[p] {
				reports = append(reports, docErr{p, ln, col, m[4]})
			} else if k := block(docVirtualRE, p); k >= 0 {
				reports = append(reports, docErr{blocks[k].File, blocks[k].Fence, 0, m[4]})
			} else if inGroup && group >= 0 {
				reports = append(reports, docErr{blocks[group].File, blocks[group].Fence, 0, m[4]})
			} else {
				raw, last = true, -1
				continue
			}
			last = len(reports) - 1
			continue
		}
		if m := docPackageRE.FindStringSubmatch(line); m != nil && !inGroup {
			if k := block(docGroupRE, m[1]); k >= 0 {
				reports = append(reports, docErr{blocks[k].File, blocks[k].Fence, 0, line})
				last = len(reports) - 1
				continue
			}
		}
		if inGroup && group >= 0 {
			reports = append(reports, docErr{blocks[group].File, blocks[group].Fence, 0, line})
			last = len(reports) - 1
			continue
		}
		raw, last = true, -1
	}
	if raw || (rc != 0 && len(reports) == 0) {
		reports = append(reports, docErr{docOutput, 0, 0, fmt.Sprintf("go build output not attributed (exit %d):\n%s", rc, out)})
	}
	if load {
		reports = append(reports, docErr{docOutput, 0, 0, docNote})
	}
	sortDocErrs(reports)
	return reports
}

// childEnv is the child build's env: the parent's, with GOFLAGS, GOWORK and GOTOOLCHAIN fixed.
func childEnv(base []string) []string {
	var out []string
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "GOFLAGS") || strings.EqualFold(k, "GOWORK") || strings.EqualFold(k, "GOTOOLCHAIN") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "GOFLAGS=-mod=readonly -buildvcs=false", "GOWORK=off", "GOTOOLCHAIN=local")
}

// checkVirtualAbsent fails when the virtual package directory exists on disk.
func checkVirtualAbsent(root string) error {
	_, err := os.Lstat(filepath.Join(root, "docsnippetchk"))
	switch {
	case err == nil:
		return errors.New("docsnippetchk exists in the repo root; the overlay build needs it absent")
	case errors.Is(err, fs.ErrNotExist):
		return nil
	}
	return err
}

// fatalSkipper is the part of testing.TB that requireGo uses.
type fatalSkipper interface {
	Fatalf(format string, args ...any)
	Skipf(format string, args ...any)
}

// requireGo finds the go command; without one the test fails (it does not skip).
func requireGo(tb fatalSkipper, look func(string) (string, error)) string {
	p, err := look("go")
	if err != nil {
		tb.Fatalf("go command not found: %v", err)
		return ""
	}
	return p
}
