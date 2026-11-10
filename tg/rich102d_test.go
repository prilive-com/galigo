package tg_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 rich messages, input blocks without media (B-25 10.2d, TEST-PLAN §2, rows T-kind … T-depth).

func rt102d(s string) tg.RichText { return tg.RichText{Plain: s} }

func rtp102d(s string) *tg.RichText { r := rt102d(s); return &r }

func ip102d(n int) *int { return &n }

// rich102dKinds is the table of the 16 block types of 10.2d and their "type" constants (T-kind). The gate
// E16/tools/spec102d.py checks this table against the spec text: the same 16 classes, the same constants.
var rich102dKinds = []struct {
	goType   string
	block    tg.InputRichBlock
	constant string
}{
	{"InputRichBlockParagraph", tg.InputRichBlockParagraph{}, "paragraph"},
	{"InputRichBlockSectionHeading", tg.InputRichBlockSectionHeading{}, "heading"},
	{"InputRichBlockPreformatted", tg.InputRichBlockPreformatted{}, "pre"},
	{"InputRichBlockFooter", tg.InputRichBlockFooter{}, "footer"},
	{"InputRichBlockDivider", tg.InputRichBlockDivider{}, "divider"},
	{"InputRichBlockMathematicalExpression", tg.InputRichBlockMathematicalExpression{}, "mathematical_expression"},
	{"InputRichBlockAnchor", tg.InputRichBlockAnchor{}, "anchor"},
	{"InputRichBlockList", tg.InputRichBlockList{}, "list"},
	{"InputRichBlockBlockQuotation", tg.InputRichBlockBlockQuotation{}, "blockquote"},
	{"InputRichBlockPullQuotation", tg.InputRichBlockPullQuotation{}, "pullquote"},
	{"InputRichBlockCollage", tg.InputRichBlockCollage{}, "collage"},
	{"InputRichBlockSlideshow", tg.InputRichBlockSlideshow{}, "slideshow"},
	{"InputRichBlockTable", tg.InputRichBlockTable{}, "table"},
	{"InputRichBlockDetails", tg.InputRichBlockDetails{}, "details"},
	{"InputRichBlockMap", tg.InputRichBlockMap{}, "map"},
	{"InputRichBlockThinking", tg.InputRichBlockThinking{}, "thinking"},
}

// T-kind: each of the 16 types (as a value and as a pointer) encodes a JSON object whose first key is "type" with its
// constant, and "type" appears once.
func TestAPI102D_Kinds(t *testing.T) {
	require.Len(t, rich102dKinds, 16)
	names, consts := map[string]bool{}, map[string]bool{}
	for _, k := range rich102dKinds {
		t.Run(k.constant, func(t *testing.T) {
			assert.Equal(t, k.goType, reflect.TypeOf(k.block).Name())
			assert.False(t, names[k.goType], "type listed twice")
			assert.False(t, consts[k.constant], "constant listed twice")
			names[k.goType], consts[k.constant] = true, true

			ptr := reflect.New(reflect.TypeOf(k.block))
			ptr.Elem().Set(reflect.ValueOf(k.block))
			for _, v := range []any{k.block, ptr.Interface()} {
				out, err := json.Marshal(v)
				require.NoError(t, err)
				assert.True(t, strings.HasPrefix(string(out), `{"type":"`+k.constant+`"`), "%s", out)
				assert.Equal(t, 1, strings.Count(string(out), `"type":`), "%s", out)
				var m map[string]any
				require.NoError(t, json.Unmarshal(out, &m))
				assert.Equal(t, k.constant, m["type"])
			}
		})
	}
}

// T-marker: the interface has exactly one method, unexported; values and pointers of the 16 types implement it.
func TestAPI102D_Marker(t *testing.T) {
	it := reflect.TypeOf((*tg.InputRichBlock)(nil)).Elem()
	require.Equal(t, 1, it.NumMethod())
	assert.Equal(t, "inputRichBlock", it.Method(0).Name)
	assert.NotEmpty(t, it.Method(0).PkgPath, "the method is unexported")
	for _, k := range rich102dKinds {
		rtyp := reflect.TypeOf(k.block)
		assert.True(t, rtyp.Implements(it), k.goType)
		assert.True(t, reflect.PointerTo(rtyp).Implements(it), k.goType)
	}
	// Read-side blocks and other types are not input blocks.
	for _, v := range []any{tg.RichBlockParagraph{}, tg.RichText{}, tg.InputRichBlockListItem{}, tg.InputRichMessage{}} {
		assert.False(t, reflect.TypeOf(v).Implements(it), reflect.TypeOf(v).Name())
	}

	// Census (T1-03): in the package source, exactly the 16 types declare the marker, and exactly the five containers
	// declare the children method of the depth walk. A marker on any other type (a read-side block) fails here.
	receivers := map[string][]string{}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			rt := fd.Recv.List[0].Type
			if st, isStar := rt.(*ast.StarExpr); isStar {
				rt = st.X
			}
			id, ok := rt.(*ast.Ident)
			require.True(t, ok, "%s: receiver of %s", f, fd.Name.Name)
			receivers[fd.Name.Name] = append(receivers[fd.Name.Name], id.Name)
		}
	}
	var want []string
	for _, k := range rich102dKinds {
		want = append(want, k.goType)
	}
	for _, k := range rich102eKinds { // B-25 10.2e: the five media blocks
		want = append(want, k.goType)
	}
	slices.Sort(want)
	slices.Sort(receivers["inputRichBlock"])
	assert.Equal(t, want, receivers["inputRichBlock"], "the types that declare inputRichBlock")
	slices.Sort(receivers["inputRichBlockChildren"])
	assert.Equal(t, []string{"InputRichBlockBlockQuotation", "InputRichBlockCollage", "InputRichBlockDetails",
		"InputRichBlockList", "InputRichBlockSlideshow"}, receivers["inputRichBlockChildren"],
		"the types that declare inputRichBlockChildren")
}

// Compile-time: each type is an InputRichBlock.
var (
	_ tg.InputRichBlock = tg.InputRichBlockParagraph{}
	_ tg.InputRichBlock = tg.InputRichBlockSectionHeading{}
	_ tg.InputRichBlock = tg.InputRichBlockPreformatted{}
	_ tg.InputRichBlock = tg.InputRichBlockFooter{}
	_ tg.InputRichBlock = tg.InputRichBlockDivider{}
	_ tg.InputRichBlock = tg.InputRichBlockMathematicalExpression{}
	_ tg.InputRichBlock = tg.InputRichBlockAnchor{}
	_ tg.InputRichBlock = tg.InputRichBlockList{}
	_ tg.InputRichBlock = tg.InputRichBlockBlockQuotation{}
	_ tg.InputRichBlock = tg.InputRichBlockPullQuotation{}
	_ tg.InputRichBlock = tg.InputRichBlockCollage{}
	_ tg.InputRichBlock = tg.InputRichBlockSlideshow{}
	_ tg.InputRichBlock = tg.InputRichBlockTable{}
	_ tg.InputRichBlock = tg.InputRichBlockDetails{}
	_ tg.InputRichBlock = tg.InputRichBlockMap{}
	_ tg.InputRichBlock = tg.InputRichBlockThinking{}
)

// T-json-full and T-json-min: the exact bytes of each block, with every field set (distinct values) and with no field
// set. Exact bytes prove the key set, the values, "type" first and once, and the field order.
func TestAPI102D_BlockJSON(t *testing.T) {
	para := func(s string) tg.InputRichBlock { return tg.InputRichBlockParagraph{Text: rt102d(s)} }
	rows := []struct {
		name string
		in   any
		want string
	}{
		{"paragraph/full", tg.InputRichBlockParagraph{Text: rt102d("p1")}, `{"type":"paragraph","text":"p1"}`},
		{"paragraph/min", tg.InputRichBlockParagraph{}, `{"type":"paragraph","text":""}`},
		{"heading/full", tg.InputRichBlockSectionHeading{Text: rt102d("h1"), Size: 3}, `{"type":"heading","text":"h1","size":3}`},
		{"heading/min", tg.InputRichBlockSectionHeading{}, `{"type":"heading","text":"","size":0}`},
		{"pre/full", tg.InputRichBlockPreformatted{Text: rt102d("x := 1"), Language: "go"},
			`{"type":"pre","text":"x := 1","language":"go"}`},
		{"pre/min", tg.InputRichBlockPreformatted{}, `{"type":"pre","text":""}`},
		{"footer/full", tg.InputRichBlockFooter{Text: rt102d("f1")}, `{"type":"footer","text":"f1"}`},
		{"footer/min", tg.InputRichBlockFooter{}, `{"type":"footer","text":""}`},
		{"divider", tg.InputRichBlockDivider{}, `{"type":"divider"}`},
		{"math/full", tg.InputRichBlockMathematicalExpression{Expression: "e=mc^2"},
			`{"type":"mathematical_expression","expression":"e=mc^2"}`},
		{"math/min", tg.InputRichBlockMathematicalExpression{}, `{"type":"mathematical_expression","expression":""}`},
		{"anchor/full", tg.InputRichBlockAnchor{Name: "a1"}, `{"type":"anchor","name":"a1"}`},
		{"anchor/min", tg.InputRichBlockAnchor{}, `{"type":"anchor","name":""}`},
		{"list/full", tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{
			{Blocks: []tg.InputRichBlock{para("i1")}, HasCheckbox: true, IsChecked: true, Value: ip102d(5), Type: "A"},
			{Blocks: []tg.InputRichBlock{para("i2")}}}},
			`{"type":"list","items":[{"blocks":[{"type":"paragraph","text":"i1"}],"has_checkbox":true,"is_checked":true,"value":5,"type":"A"},{"blocks":[{"type":"paragraph","text":"i2"}]}]}`},
		{"list/min", tg.InputRichBlockList{}, `{"type":"list","items":null}`},
		// Each bool of a list item alone (T1-02): a swap of the two values is seen.
		{"list/checkbox-only", tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{HasCheckbox: true}}},
			`{"type":"list","items":[{"blocks":null,"has_checkbox":true}]}`},
		{"list/checked-only", tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{IsChecked: true}}},
			`{"type":"list","items":[{"blocks":null,"is_checked":true}]}`},
		{"blockquote/full", tg.InputRichBlockBlockQuotation{Blocks: []tg.InputRichBlock{para("q1"), para("q2")}, Credit: rtp102d("c1")},
			`{"type":"blockquote","blocks":[{"type":"paragraph","text":"q1"},{"type":"paragraph","text":"q2"}],"credit":"c1"}`},
		{"blockquote/min", tg.InputRichBlockBlockQuotation{}, `{"type":"blockquote","blocks":null}`},
		{"pullquote/full", tg.InputRichBlockPullQuotation{Text: rt102d("pq"), Credit: rtp102d("pc")},
			`{"type":"pullquote","text":"pq","credit":"pc"}`},
		{"pullquote/min", tg.InputRichBlockPullQuotation{}, `{"type":"pullquote","text":""}`},
		{"collage/full", tg.InputRichBlockCollage{Blocks: []tg.InputRichBlock{para("k1")},
			Caption: &tg.RichBlockCaption{Text: rt102d("kc"), Credit: rtp102d("kr")}},
			`{"type":"collage","blocks":[{"type":"paragraph","text":"k1"}],"caption":{"text":"kc","credit":"kr"}}`},
		{"collage/min", tg.InputRichBlockCollage{}, `{"type":"collage","blocks":null}`},
		{"slideshow/full", tg.InputRichBlockSlideshow{Blocks: []tg.InputRichBlock{para("s1")},
			Caption: &tg.RichBlockCaption{Text: rt102d("sc")}},
			`{"type":"slideshow","blocks":[{"type":"paragraph","text":"s1"}],"caption":{"text":"sc"}}`},
		{"slideshow/min", tg.InputRichBlockSlideshow{}, `{"type":"slideshow","blocks":null}`},
		{"table/full", tg.InputRichBlockTable{
			Cells:      [][]tg.RichBlockTableCell{{{Text: rtp102d("t1"), IsHeader: true, Align: "left", VAlign: "top"}}, {{Align: "right", VAlign: "bottom", Colspan: 2}}},
			IsBordered: true, IsStriped: true, Caption: rtp102d("tc")},
			`{"type":"table","cells":[[{"text":"t1","is_header":true,"align":"left","valign":"top"}],[{"colspan":2,"align":"right","valign":"bottom"}]],"is_bordered":true,"is_striped":true,"caption":"tc"}`},
		{"table/min", tg.InputRichBlockTable{}, `{"type":"table","cells":null}`},
		// Each bool of a table alone (T1-02).
		{"table/bordered-only", tg.InputRichBlockTable{IsBordered: true}, `{"type":"table","cells":null,"is_bordered":true}`},
		{"table/striped-only", tg.InputRichBlockTable{IsStriped: true}, `{"type":"table","cells":null,"is_striped":true}`},
		{"details/full", tg.InputRichBlockDetails{Summary: rt102d("ds"), Blocks: []tg.InputRichBlock{para("d1")}, IsOpen: true},
			`{"type":"details","summary":"ds","blocks":[{"type":"paragraph","text":"d1"}],"is_open":true}`},
		{"details/min", tg.InputRichBlockDetails{}, `{"type":"details","summary":"","blocks":null}`},
		{"map/full", tg.InputRichBlockMap{Location: tg.Location{Latitude: 52.5, Longitude: 13.25}, Zoom: ip102d(7),
			Width: ip102d(640), Height: ip102d(480), Caption: &tg.RichBlockCaption{Text: rt102d("mc")}},
			`{"type":"map","location":{"longitude":13.25,"latitude":52.5},"zoom":7,"width":640,"height":480,"caption":{"text":"mc"}}`},
		{"map/min", tg.InputRichBlockMap{}, `{"type":"map","location":{"longitude":0,"latitude":0}}`},
		{"thinking/full", tg.InputRichBlockThinking{Text: rt102d("th")}, `{"type":"thinking","text":"th"}`},
		{"thinking/min", tg.InputRichBlockThinking{}, `{"type":"thinking","text":""}`},
		// T3-03: every string of a block is sent as given: spaces and tabs at the edges stay.
		{"paragraph/as-given", tg.InputRichBlockParagraph{Text: rt102d(" p\t")}, `{"type":"paragraph","text":" p\t"}`},
		{"heading/as-given", tg.InputRichBlockSectionHeading{Text: rt102d(" h\t"), Size: 1}, `{"type":"heading","text":" h\t","size":1}`},
		{"pre/as-given", tg.InputRichBlockPreformatted{Text: rt102d(" x\t"), Language: " go "}, `{"type":"pre","text":" x\t","language":" go "}`},
		{"footer/as-given", tg.InputRichBlockFooter{Text: rt102d(" f\t")}, `{"type":"footer","text":" f\t"}`},
		{"math/as-given", tg.InputRichBlockMathematicalExpression{Expression: " e\t"}, `{"type":"mathematical_expression","expression":" e\t"}`},
		{"anchor/as-given", tg.InputRichBlockAnchor{Name: " a\t"}, `{"type":"anchor","name":" a\t"}`},
		{"list/as-given", tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Type: " A "}}}, `{"type":"list","items":[{"blocks":null,"type":" A "}]}`},
		{"blockquote/as-given", tg.InputRichBlockBlockQuotation{Credit: rtp102d(" c\t")}, `{"type":"blockquote","blocks":null,"credit":" c\t"}`},
		{"pullquote/as-given", tg.InputRichBlockPullQuotation{Text: rt102d(" q\t"), Credit: rtp102d(" r ")}, `{"type":"pullquote","text":" q\t","credit":" r "}`},
		{"collage/as-given", tg.InputRichBlockCollage{Caption: &tg.RichBlockCaption{Text: rt102d(" k\t"), Credit: rtp102d(" r ")}},
			`{"type":"collage","blocks":null,"caption":{"text":" k\t","credit":" r "}}`},
		{"slideshow/as-given", tg.InputRichBlockSlideshow{Caption: &tg.RichBlockCaption{Text: rt102d(" k\t"), Credit: rtp102d(" r ")}},
			`{"type":"slideshow","blocks":null,"caption":{"text":" k\t","credit":" r "}}`},
		{"map/as-given", tg.InputRichBlockMap{Caption: &tg.RichBlockCaption{Text: rt102d(" m\t"), Credit: rtp102d(" r ")}},
			`{"type":"map","location":{"longitude":0,"latitude":0},"caption":{"text":" m\t","credit":" r "}}`},
		{"table/as-given", tg.InputRichBlockTable{Caption: rtp102d(" t\t")}, `{"type":"table","cells":null,"caption":" t\t"}`},
		{"details/as-given", tg.InputRichBlockDetails{Summary: rt102d(" s\t")}, `{"type":"details","summary":" s\t","blocks":null}`},
		{"thinking/as-given", tg.InputRichBlockThinking{Text: rt102d(" t\t")}, `{"type":"thinking","text":" t\t"}`},
		// T3-04: an optional text set to an empty value is sent ("omitempty" omits only a nil pointer).
		{"blockquote/empty-credit", tg.InputRichBlockBlockQuotation{Credit: &tg.RichText{}}, `{"type":"blockquote","blocks":null,"credit":""}`},
		{"pullquote/empty-credit", tg.InputRichBlockPullQuotation{Credit: &tg.RichText{}}, `{"type":"pullquote","text":"","credit":""}`},
		{"collage/empty-caption", tg.InputRichBlockCollage{Caption: &tg.RichBlockCaption{Credit: &tg.RichText{}}},
			`{"type":"collage","blocks":null,"caption":{"text":"","credit":""}}`},
		{"slideshow/empty-caption", tg.InputRichBlockSlideshow{Caption: &tg.RichBlockCaption{}}, `{"type":"slideshow","blocks":null,"caption":{"text":""}}`},
		{"table/empty-caption", tg.InputRichBlockTable{Caption: &tg.RichText{}}, `{"type":"table","cells":null,"caption":""}`},
		{"map/empty-caption", tg.InputRichBlockMap{Caption: &tg.RichBlockCaption{}}, `{"type":"map","location":{"longitude":0,"latitude":0},"caption":{"text":""}}`},
		// The RichText of a block keeps its own form (parts, a node).
		{"paragraph/parts", tg.InputRichBlockParagraph{Text: tg.RichText{Parts: []tg.RichText{rt102d("a"), {Node: tg.RichTextBold{Type: "bold", Text: rt102d("b")}}}}},
			`{"type":"paragraph","text":["a",{"type":"bold","text":"b"}]}`},
		// A pointer encodes like the value; a nil element (interface or typed pointer) encodes as null.
		{"pointer", &tg.InputRichBlockFooter{Text: rt102d("pf")}, `{"type":"footer","text":"pf"}`},
		// (A typed nil container is a T-depth row: there a regression crashes the child, not this process.)
		{"nil-elements", tg.InputRichBlockBlockQuotation{Blocks: []tg.InputRichBlock{nil, (*tg.InputRichBlockParagraph)(nil)}},
			`{"type":"blockquote","blocks":[null,null]}`},
		// Nesting: a block inside a list item inside details inside a quotation.
		{"nested", tg.InputRichBlockBlockQuotation{Blocks: []tg.InputRichBlock{tg.InputRichBlockDetails{Summary: rt102d("n1"),
			Blocks: []tg.InputRichBlock{tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Blocks: []tg.InputRichBlock{tg.InputRichBlockDivider{}}}}}}}}},
			`{"type":"blockquote","blocks":[{"type":"details","summary":"n1","blocks":[{"type":"list","items":[{"blocks":[{"type":"divider"}]}]}]}]}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
}

// T-err: an encoding error inside a block reaches the caller; the helper does not replace it.
func TestAPI102D_EncodeError(t *testing.T) {
	bad := tg.RichText{Node: tg.RichTextUnknown{Type: "x", Raw: json.RawMessage(`{`)}}
	for name, v := range map[string]any{
		"paragraph": tg.InputRichBlockParagraph{Text: bad},
		"nested":    tg.InputRichBlockBlockQuotation{Blocks: []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: bad}}},
		"message":   tg.InputRichMessage{Blocks: []tg.InputRichBlock{tg.InputRichBlockFooter{Text: bad}}},
	} {
		out, err := json.Marshal(v)
		require.Error(t, err, "%s: %s", name, out)
		assert.NotContains(t, err.Error(), "nested more than 100", name)
	}
}

// T-int: the four *int fields: nil is not sent, a pointer to 0 sends 0, a pointer to another value sends it.
func TestAPI102D_OptionalInts(t *testing.T) {
	rows := []struct {
		name string
		in   any
		want string
	}{
		{"map/nil", tg.InputRichBlockMap{Location: tg.Location{Latitude: 1, Longitude: 2}}, `{"type":"map","location":{"longitude":2,"latitude":1}}`},
		{"map/zero", tg.InputRichBlockMap{Location: tg.Location{Latitude: 1, Longitude: 2}, Zoom: ip102d(0), Width: ip102d(0), Height: ip102d(0)},
			`{"type":"map","location":{"longitude":2,"latitude":1},"zoom":0,"width":0,"height":0}`},
		{"map/zoom", tg.InputRichBlockMap{Zoom: ip102d(24)}, `{"type":"map","location":{"longitude":0,"latitude":0},"zoom":24}`},
		{"map/width", tg.InputRichBlockMap{Width: ip102d(10000)}, `{"type":"map","location":{"longitude":0,"latitude":0},"width":10000}`},
		{"map/height", tg.InputRichBlockMap{Height: ip102d(1)}, `{"type":"map","location":{"longitude":0,"latitude":0},"height":1}`},
		{"item/nil", tg.InputRichBlockListItem{Type: "i"}, `{"blocks":null,"type":"i"}`},
		{"item/zero", tg.InputRichBlockListItem{Value: ip102d(0)}, `{"blocks":null,"value":0}`},
		{"item/value", tg.InputRichBlockListItem{Value: ip102d(-3)}, `{"blocks":null,"value":-3}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
}

// rich102dShape is every field of the 17 new types: name, Go type, JSON tag (T-shape). The gate E16/tools/spec102d.py
// checks the tags against the spec text: required fields have no omitempty, optional fields have it.
var rich102dShape = map[string][]rich101dField{
	"InputRichBlockParagraph":              {{"Text", "tg.RichText", "text"}},
	"InputRichBlockSectionHeading":         {{"Text", "tg.RichText", "text"}, {"Size", "int", "size"}},
	"InputRichBlockPreformatted":           {{"Text", "tg.RichText", "text"}, {"Language", "string", "language,omitempty"}},
	"InputRichBlockFooter":                 {{"Text", "tg.RichText", "text"}},
	"InputRichBlockDivider":                nil,
	"InputRichBlockMathematicalExpression": {{"Expression", "string", "expression"}},
	"InputRichBlockAnchor":                 {{"Name", "string", "name"}},
	"InputRichBlockList":                   {{"Items", "[]tg.InputRichBlockListItem", "items"}},
	"InputRichBlockBlockQuotation":         {{"Blocks", "[]tg.InputRichBlock", "blocks"}, {"Credit", "*tg.RichText", "credit,omitempty"}},
	"InputRichBlockPullQuotation":          {{"Text", "tg.RichText", "text"}, {"Credit", "*tg.RichText", "credit,omitempty"}},
	"InputRichBlockCollage":                {{"Blocks", "[]tg.InputRichBlock", "blocks"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichBlockSlideshow":              {{"Blocks", "[]tg.InputRichBlock", "blocks"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichBlockTable": {{"Cells", "[][]tg.RichBlockTableCell", "cells"}, {"IsBordered", "bool", "is_bordered,omitempty"},
		{"IsStriped", "bool", "is_striped,omitempty"}, {"Caption", "*tg.RichText", "caption,omitempty"}},
	"InputRichBlockDetails": {{"Summary", "tg.RichText", "summary"}, {"Blocks", "[]tg.InputRichBlock", "blocks"},
		{"IsOpen", "bool", "is_open,omitempty"}},
	"InputRichBlockMap": {{"Location", "tg.Location", "location"}, {"Zoom", "*int", "zoom,omitempty"}, {"Width", "*int", "width,omitempty"},
		{"Height", "*int", "height,omitempty"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichBlockThinking": {{"Text", "tg.RichText", "text"}},
	"InputRichBlockListItem": {{"Blocks", "[]tg.InputRichBlock", "blocks"}, {"HasCheckbox", "bool", "has_checkbox,omitempty"},
		{"IsChecked", "bool", "is_checked,omitempty"}, {"Value", "*int", "value,omitempty"}, {"Type", "string", "type,omitempty"}},
}

// T-shape: the fields of the 17 types, in order, with their exact count; none embedded; MarshalJSON is the only
// exported method of the 16 blocks, on the value and on the pointer (none on the list item).
func TestAPI102D_Shapes(t *testing.T) {
	values := map[string]any{"InputRichBlockListItem": tg.InputRichBlockListItem{}}
	for _, k := range rich102dKinds {
		values[k.goType] = k.block
	}
	require.Len(t, rich102dShape, 17)
	require.Len(t, values, 17)
	for name, want := range rich102dShape {
		t.Run(name, func(t *testing.T) {
			v, ok := values[name]
			require.True(t, ok)
			rtyp := reflect.TypeOf(v)
			assert.Equal(t, want, rich101dFields(t, v))
			require.Equal(t, len(want), rtyp.NumField())
			for i := range rtyp.NumField() {
				assert.False(t, rtyp.Field(i).Anonymous, rtyp.Field(i).Name)
			}
			methods := 1
			if name == "InputRichBlockListItem" {
				methods = 0
			}
			require.Equal(t, methods, rtyp.NumMethod())
			require.Equal(t, methods, reflect.PointerTo(rtyp).NumMethod())
			if methods == 1 {
				assert.Equal(t, "MarshalJSON", rtyp.Method(0).Name)
			}
		})
	}
}

// T-msg: a message with blocks: "blocks" comes after the four 10.1 fields; an empty slice is not sent; the inline
// content carries the blocks unchanged.
func TestAPI102D_MessageBlocks(t *testing.T) {
	blocks := []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: rt102d("m1")}, tg.InputRichBlockDivider{}}
	rows := []struct {
		name string
		in   any
		want string
	}{
		{"blocks", tg.InputRichMessage{Blocks: blocks}, `{"blocks":[{"type":"paragraph","text":"m1"},{"type":"divider"}]}`},
		{"all", tg.InputRichMessage{HTML: "h", Markdown: "m", IsRTL: true, SkipEntityDetection: true, Blocks: blocks},
			`{"html":"h","markdown":"m","is_rtl":true,"skip_entity_detection":true,"blocks":[{"type":"paragraph","text":"m1"},{"type":"divider"}]}`},
		{"empty", tg.InputRichMessage{HTML: "h", Blocks: []tg.InputRichBlock{}}, `{"html":"h"}`},
		{"nil-element", tg.InputRichMessage{Blocks: []tg.InputRichBlock{nil}}, `{"blocks":[null]}`},
		{"inline", tg.InputRichMessageContent{RichMessage: tg.InputRichMessage{Blocks: blocks, IsRTL: true}},
			`{"rich_message":{"is_rtl":true,"blocks":[{"type":"paragraph","text":"m1"},{"type":"divider"}]}}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
}

// Wrapper types: a caller struct that embeds a container inherits its methods (T-depth wrapped rows).
type (
	w102dList  struct{ tg.InputRichBlockList }
	w102dQuote struct {
		tg.InputRichBlockBlockQuotation
	}
	w102dCollage   struct{ tg.InputRichBlockCollage }
	w102dSlideshow struct{ tg.InputRichBlockSlideshow }
	w102dDetails   struct{ tg.InputRichBlockDetails }
)

// rich102dContainers builds each of the five containers around children, as a value and wrapped in a caller type.
var rich102dContainers = []struct {
	name    string
	make    func([]tg.InputRichBlock) tg.InputRichBlock
	wrapped func([]tg.InputRichBlock) tg.InputRichBlock
	ptr     func() (tg.InputRichBlock, func([]tg.InputRichBlock))
}{
	{"list",
		func(c []tg.InputRichBlock) tg.InputRichBlock {
			return tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Blocks: c}}}
		},
		func(c []tg.InputRichBlock) tg.InputRichBlock {
			return w102dList{tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Blocks: c}}}}
		},
		func() (tg.InputRichBlock, func([]tg.InputRichBlock)) {
			p := &tg.InputRichBlockList{}
			return p, func(c []tg.InputRichBlock) { p.Items = []tg.InputRichBlockListItem{{Blocks: c}} }
		}},
	{"blockquote",
		func(c []tg.InputRichBlock) tg.InputRichBlock { return tg.InputRichBlockBlockQuotation{Blocks: c} },
		func(c []tg.InputRichBlock) tg.InputRichBlock {
			return w102dQuote{tg.InputRichBlockBlockQuotation{Blocks: c}}
		},
		func() (tg.InputRichBlock, func([]tg.InputRichBlock)) {
			p := &tg.InputRichBlockBlockQuotation{}
			return p, func(c []tg.InputRichBlock) { p.Blocks = c }
		}},
	{"collage",
		func(c []tg.InputRichBlock) tg.InputRichBlock { return tg.InputRichBlockCollage{Blocks: c} },
		func(c []tg.InputRichBlock) tg.InputRichBlock {
			return w102dCollage{tg.InputRichBlockCollage{Blocks: c}}
		},
		func() (tg.InputRichBlock, func([]tg.InputRichBlock)) {
			p := &tg.InputRichBlockCollage{}
			return p, func(c []tg.InputRichBlock) { p.Blocks = c }
		}},
	{"slideshow",
		func(c []tg.InputRichBlock) tg.InputRichBlock { return tg.InputRichBlockSlideshow{Blocks: c} },
		func(c []tg.InputRichBlock) tg.InputRichBlock {
			return w102dSlideshow{tg.InputRichBlockSlideshow{Blocks: c}}
		},
		func() (tg.InputRichBlock, func([]tg.InputRichBlock)) {
			p := &tg.InputRichBlockSlideshow{}
			return p, func(c []tg.InputRichBlock) { p.Blocks = c }
		}},
	{"details",
		func(c []tg.InputRichBlock) tg.InputRichBlock { return tg.InputRichBlockDetails{Blocks: c} },
		func(c []tg.InputRichBlock) tg.InputRichBlock {
			return w102dDetails{tg.InputRichBlockDetails{Blocks: c}}
		},
		func() (tg.InputRichBlock, func([]tg.InputRichBlock)) {
			p := &tg.InputRichBlockDetails{}
			return p, func(c []tg.InputRichBlock) { p.Blocks = c }
		}},
}

const rich102dDepthErr = "tg: input rich blocks nested more than 100 levels deep (or in a cycle)"

// rich102dChildEnv makes the test binary run the T-depth rows itself, under a small stack limit, so a missing guard
// fails the parent test by a crash of the child instead of using up to 1 GB of stack in the test process.
const rich102dChildEnv = "GALIGO_RICH102D_DEPTH_CHILD"

// rich102dDepthRows are the names of the T-depth rows; the parent checks that each one passed in the child.
func rich102dDepthRows() []string {
	var rows []string
	for _, c := range rich102dContainers {
		for _, k := range []string{"self-value", "self-pointer", "two-block", "wrapped-self", "wrapped-two-block",
			"depth-100", "depth-101", "shared", "wrapped-shared", "typed-nil-wrapper",
			"second-block", "wrapped-second-block", "depth-100-second", "depth-101-second",
			"first-block", "second-parent"} {
			rows = append(rows, k+"-"+c.name)
		}
	}
	return append(rows, "cycle-second-item-list", "cycle-first-item-list", "message-cycle", "message-depth-101",
		"message-depth-100")
}

// T-depth: cycles and deep nesting return the error and do not crash; a shared child and depth 100 encode.
func TestAPI102D_Depth(t *testing.T) {
	if os.Getenv(rich102dChildEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAPI102D_Depth$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), rich102dChildEnv+"=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "the child failed or crashed:\n%s", out)
		// The row list is built from rich102dContainers, so pin that table too (T1-01 of Grok): a container dropped
		// from it would drop its rows from the child and from this check together.
		var names []string
		for _, c := range rich102dContainers {
			names = append(names, c.name)
		}
		require.Equal(t, []string{"list", "blockquote", "collage", "slideshow", "details"}, names)
		require.Len(t, rich102dDepthRows(), 85)
		for _, r := range rich102dDepthRows() {
			assert.Contains(t, string(out), "--- PASS: TestAPI102D_Depth/"+r+" ", r)
		}
		return
	}
	debug.SetMaxStack(64 << 20)

	marshalErr := func(t *testing.T, v any) {
		t.Helper()
		out, err := json.Marshal(v)
		require.Error(t, err, "%s", out)
		assert.Contains(t, err.Error(), rich102dDepthErr)
	}
	quote := func(c []tg.InputRichBlock) tg.InputRichBlock { return tg.InputRichBlockBlockQuotation{Blocks: c} }
	leaf := tg.InputRichBlockParagraph{Text: rt102d("x")}

	for _, c := range rich102dContainers {
		t.Run("self-value-"+c.name, func(t *testing.T) {
			s := make([]tg.InputRichBlock, 1)
			s[0] = c.make(s)
			marshalErr(t, s[0])
		})
		t.Run("self-pointer-"+c.name, func(t *testing.T) {
			p, set := c.ptr()
			set([]tg.InputRichBlock{p})
			marshalErr(t, p)
		})
		t.Run("two-block-"+c.name, func(t *testing.T) {
			a, b := make([]tg.InputRichBlock, 1), make([]tg.InputRichBlock, 1)
			a[0] = c.make(b)
			b[0] = quote(a)
			marshalErr(t, a[0])
			marshalErr(t, b[0])
		})
		t.Run("wrapped-self-"+c.name, func(t *testing.T) {
			s := make([]tg.InputRichBlock, 1)
			s[0] = c.wrapped(s)
			marshalErr(t, s[0])
		})
		t.Run("wrapped-two-block-"+c.name, func(t *testing.T) {
			a, b := make([]tg.InputRichBlock, 1), make([]tg.InputRichBlock, 1)
			a[0] = c.wrapped(b)
			b[0] = quote(a)
			marshalErr(t, b[0])
		})
		nest := func(n int) tg.InputRichBlock {
			var b tg.InputRichBlock = leaf
			for range n {
				b = c.make([]tg.InputRichBlock{b})
			}
			return b
		}
		t.Run("depth-100-"+c.name, func(t *testing.T) {
			out, err := json.Marshal(nest(100))
			require.NoError(t, err)
			assert.Equal(t, 100, strings.Count(string(out), `"type":"`+c.name+`"`))
		})
		t.Run("depth-101-"+c.name, func(t *testing.T) {
			marshalErr(t, nest(101))
		})
		t.Run("shared-"+c.name, func(t *testing.T) {
			shared := []tg.InputRichBlock{leaf}
			out, err := json.Marshal([]tg.InputRichBlock{c.make(shared), c.make(shared), quote([]tg.InputRichBlock{c.make(shared)})})
			require.NoError(t, err)
			assert.Equal(t, 3, strings.Count(string(out), `{"type":"paragraph","text":"x"}`))
		})
		t.Run("wrapped-shared-"+c.name, func(t *testing.T) {
			shared := []tg.InputRichBlock{leaf}
			w := c.wrapped(shared)
			out, err := json.Marshal([]tg.InputRichBlock{w, w, quote([]tg.InputRichBlock{w})})
			require.NoError(t, err)
			assert.Equal(t, 3, strings.Count(string(out), `{"type":"paragraph","text":"x"}`))
		})
		// A cycle or deep nesting in the SECOND block of a slice (T1-01): the walk must visit every block, not the first.
		t.Run("second-block-"+c.name, func(t *testing.T) {
			s := make([]tg.InputRichBlock, 2)
			s[0] = leaf
			s[1] = c.make(s)
			marshalErr(t, s[1])
		})
		t.Run("wrapped-second-block-"+c.name, func(t *testing.T) {
			s := make([]tg.InputRichBlock, 2)
			s[0] = leaf
			s[1] = c.wrapped(s)
			marshalErr(t, s[1])
		})
		nestSecond := func(n int) tg.InputRichBlock {
			var b tg.InputRichBlock = leaf
			for range n {
				b = c.make([]tg.InputRichBlock{leaf, b})
			}
			return b
		}
		t.Run("depth-100-second-"+c.name, func(t *testing.T) {
			out, err := json.Marshal(nestSecond(100))
			require.NoError(t, err)
			assert.Equal(t, 100, strings.Count(string(out), `"type":"`+c.name+`"`))
		})
		t.Run("depth-101-second-"+c.name, func(t *testing.T) {
			marshalErr(t, nestSecond(101))
		})
		// The cycle in the FIRST block, a leaf after it (T2-01 of Grok): a walk of only the last block misses it.
		t.Run("first-block-"+c.name, func(t *testing.T) {
			s := make([]tg.InputRichBlock, 2)
			s[0] = c.make(s)
			s[1] = leaf
			marshalErr(t, s[0])
		})
		// Two containers side by side, the cycle only in the SECOND one (T2-01 of Grok): a walk that stops after the
		// first container it walked misses it.
		t.Run("second-parent-"+c.name, func(t *testing.T) {
			s := make([]tg.InputRichBlock, 2)
			s[0] = quote([]tg.InputRichBlock{leaf})
			s[1] = c.make(s)
			marshalErr(t, s[1])
		})
		t.Run("typed-nil-wrapper-"+c.name, func(t *testing.T) {
			nilWrap := reflect.Zero(reflect.PointerTo(reflect.TypeOf(c.wrapped(nil)))).Interface().(tg.InputRichBlock)
			nilPtr, _ := c.ptr()
			nilPtr = reflect.Zero(reflect.TypeOf(nilPtr)).Interface().(tg.InputRichBlock)
			out, err := json.Marshal(c.make([]tg.InputRichBlock{nilWrap, nilPtr}))
			require.NoError(t, err)
			assert.Contains(t, string(out), `[null,null]`)
		})
	}
	t.Run("cycle-second-item-list", func(t *testing.T) {
		s := make([]tg.InputRichBlock, 1)
		s[0] = tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Blocks: []tg.InputRichBlock{leaf}}, {Blocks: s}}}
		marshalErr(t, s[0])
	})
	// The cycle in the FIRST list item, a normal item after it (T2-01 of Grok).
	t.Run("cycle-first-item-list", func(t *testing.T) {
		s := make([]tg.InputRichBlock, 1)
		s[0] = tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Blocks: s}, {Blocks: []tg.InputRichBlock{leaf}}}}
		marshalErr(t, s[0])
	})
	t.Run("message-cycle", func(t *testing.T) {
		s := make([]tg.InputRichBlock, 1)
		s[0] = quote(s)
		marshalErr(t, tg.InputRichMessage{Blocks: s})
	})
	t.Run("message-depth-101", func(t *testing.T) {
		var b tg.InputRichBlock = leaf
		for range 101 {
			b = quote([]tg.InputRichBlock{b})
		}
		marshalErr(t, tg.InputRichMessage{Blocks: []tg.InputRichBlock{b}})
	})
	t.Run("message-depth-100", func(t *testing.T) {
		var b tg.InputRichBlock = leaf
		for range 100 {
			b = quote([]tg.InputRichBlock{b})
		}
		_, err := json.Marshal(tg.InputRichMessage{Blocks: []tg.InputRichBlock{b}})
		require.NoError(t, err)
	})
}
