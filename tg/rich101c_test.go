package tg_test

// Bot API 10.1 rich messages, read side (B-25 part 10.1c, TEST-PLAN rows T-rm-*, T-rt-*, T-rb-*, T-shape, T-seal,
// T-copy). The live fixtures are in rich101c_fixtures_test.go. The field table rich101cShapes is generated from the
// spec, not from the code.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

var rich101cFixtures = []struct {
	name, json string
	types      []string
}{
	{"01-inline", rich101cProbeInline, []string{"paragraph"}},
	{"02-blocks", rich101cProbeBlocks, []string{"heading", "heading", "paragraph", "pre", "divider", "list", "list",
		"list", "blockquote", "table", "paragraph", "mathematical_expression", "details", "footer"}},
	{"03-html", rich101cProbeHTML, []string{"anchor", "paragraph", "pullquote", "details", "footer", "map"}},
	{"04-media", rich101cProbeMedia, []string{"photo", "collage"}},
	{"05-rtl-plain", rich101cProbeRTLPlain, []string{"paragraph"}},
}

// rich101cGoTypes maps a block "type" to the Go type the decoder must return for it.
var rich101cGoTypes = map[string]string{
	"paragraph": "tg.RichBlockParagraph", "heading": "tg.RichBlockSectionHeading", "pre": "tg.RichBlockPreformatted",
	"divider": "tg.RichBlockDivider", "list": "tg.RichBlockList", "blockquote": "tg.RichBlockBlockQuotation",
	"table": "tg.RichBlockTable", "mathematical_expression": "tg.RichBlockMathematicalExpression",
	"details": "tg.RichBlockDetails", "footer": "tg.RichBlockFooter", "anchor": "tg.RichBlockAnchor",
	"pullquote": "tg.RichBlockPullQuotation", "map": "tg.RichBlockMap", "photo": "tg.RichBlockPhoto",
	"collage": "tg.RichBlockCollage",
}

func rich101cDecode(t *testing.T, s string) (tg.Message, json.RawMessage) {
	t.Helper()
	var msg tg.Message
	require.NoError(t, json.Unmarshal([]byte(s), &msg))
	require.NotNil(t, msg.RichMessage, "rich_message was not decoded")
	var raw struct {
		RichMessage json.RawMessage `json:"rich_message"`
	}
	require.NoError(t, json.Unmarshal([]byte(s), &raw))
	require.NotEmpty(t, raw.RichMessage)
	return msg, raw.RichMessage
}

// T-rm-1
func TestAPI101C_DecodeProbes(t *testing.T) {
	for _, f := range rich101cFixtures {
		t.Run(f.name, func(t *testing.T) {
			msg, raw := rich101cDecode(t, f.json)
			require.NotNil(t, msg.RichMessage)
			assert.Empty(t, msg.Text)
			var rawBlocks struct {
				Blocks []map[string]any `json:"blocks"`
			}
			require.NoError(t, json.Unmarshal(raw, &rawBlocks))
			var rawTypes, goTypes, wantGo []string
			for _, b := range rawBlocks.Blocks {
				rawTypes = append(rawTypes, b["type"].(string))
			}
			for _, b := range msg.RichMessage.Blocks {
				goTypes = append(goTypes, fmt.Sprintf("%T", b))
			}
			for _, ty := range f.types {
				wantGo = append(wantGo, rich101cGoTypes[ty])
			}
			assert.Equal(t, f.types, rawTypes)
			assert.Equal(t, wantGo, goTypes)
		})
	}
}

// T-rm-2
func TestAPI101C_RoundTrip(t *testing.T) {
	for _, f := range rich101cFixtures {
		t.Run(f.name, func(t *testing.T) {
			msg, raw := rich101cDecode(t, f.json)
			out, err := json.Marshal(msg.RichMessage)
			require.NoError(t, err)
			assert.JSONEq(t, string(raw), string(out))
		})
	}
}

// rich101cWalkText calls fn for every node of a RichText tree, depth first.
func rich101cWalkText(rt tg.RichText, fn func(tg.RichTextNode)) {
	for _, p := range rt.Parts {
		rich101cWalkText(p, fn)
	}
	if rt.Node == nil {
		return
	}
	fn(rt.Node)
	if v := reflect.ValueOf(rt.Node); v.Kind() == reflect.Struct {
		if f := v.FieldByName("Text"); f.IsValid() {
			if inner, ok := f.Interface().(tg.RichText); ok {
				rich101cWalkText(inner, fn)
			}
		}
	}
}

func rich101cFindText[T tg.RichTextNode](t *testing.T, rt tg.RichText) []T {
	t.Helper()
	var out []T
	rich101cWalkText(rt, func(n tg.RichTextNode) {
		if v, ok := n.(T); ok {
			out = append(out, v)
		}
	})
	return out
}

func rich101cBlock[T tg.RichBlock](t *testing.T, bs tg.RichBlocks, i int) T {
	t.Helper()
	require.Greater(t, len(bs), i)
	v, ok := bs[i].(T)
	require.True(t, ok, "block %d is %T", i, bs[i])
	return v
}

// T-rm-3
func TestAPI101C_ProbeValues(t *testing.T) {
	t.Run("01-inline", func(t *testing.T) {
		msg, _ := rich101cDecode(t, rich101cProbeInline)
		p := rich101cBlock[tg.RichBlockParagraph](t, msg.RichMessage.Blocks, 0)
		require.Len(t, p.Text.Parts, 44)
		simple := []struct {
			i    int
			want tg.RichTextNode
		}{
			{2, tg.RichTextItalic{Type: "italic", Text: tg.RichText{Plain: "italic"}}},
			{4, tg.RichTextUnderline{Type: "underline", Text: tg.RichText{Plain: "under"}}},
			{6, tg.RichTextStrikethrough{Type: "strikethrough", Text: tg.RichText{Plain: "strike"}}},
			{8, tg.RichTextSpoiler{Type: "spoiler", Text: tg.RichText{Plain: "spoiler"}}},
			{10, tg.RichTextCode{Type: "code", Text: tg.RichText{Plain: "code"}}},
			{12, tg.RichTextMarked{Type: "marked", Text: tg.RichText{Plain: "marked"}}},
			{14, tg.RichTextSubscript{Type: "subscript", Text: tg.RichText{Plain: "sub"}}},
			{16, tg.RichTextSuperscript{Type: "superscript", Text: tg.RichText{Plain: "sup"}}},
			{18, tg.RichTextURL{Type: "url", Text: tg.RichText{Plain: "link"}, URL: "https://t.me/"}},
			{20, tg.RichTextEmailAddress{Type: "email_address", Text: tg.RichText{Plain: "mail"}, EmailAddress: "user@example.com"}},
			{22, tg.RichTextPhoneNumber{Type: "phone_number", Text: tg.RichText{Plain: "phone"}, PhoneNumber: "+123456789"}},
			{24, tg.RichTextDateTime{Type: "date_time", Text: tg.RichText{Plain: "22:45 tomorrow"}, UnixTime: 1647531900, DateTimeFormat: "wDT"}},
			{26, tg.RichTextMathematicalExpression{Type: "mathematical_expression", Expression: "x^2 + y^2"}},
			{28, tg.RichTextHashtag{Type: "hashtag", Text: tg.RichText{Plain: "#hashtag"}, Hashtag: "hashtag"}},
			{30, tg.RichTextCashtag{Type: "cashtag", Text: tg.RichText{Plain: "$USD"}, Cashtag: "USD"}},
			{38, tg.RichTextBotCommand{Type: "bot_command", Text: tg.RichText{Plain: "/command"}, BotCommand: "command"}},
			{40, tg.RichTextMention{Type: "mention", Text: tg.RichText{Plain: "@username"}, Username: "username"}},
		}
		for _, s := range simple {
			assert.Equal(t, s.want, p.Text.Parts[s.i].Node, "part %d", s.i)
		}
		assert.Equal(t, tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "bold"}}, p.Text.Parts[0].Node)
		assert.Equal(t, tg.RichText{Plain: " plain tail"}, p.Text.Parts[43])
		nested := tg.RichTextBold{Type: "bold", Text: tg.RichText{Parts: []tg.RichText{
			{Plain: "Bold "},
			{Node: tg.RichTextItalic{Type: "italic", Text: tg.RichText{Parts: []tg.RichText{
				{Plain: "italic "},
				{Node: tg.RichTextUnderline{Type: "underline", Text: tg.RichText{Plain: "underlined"}}},
				{Plain: " italic"},
			}}}},
			{Plain: " bold"},
		}}}
		assert.Equal(t, nested, p.Text.Parts[42].Node)
	})
	t.Run("02-blocks", func(t *testing.T) {
		msg, _ := rich101cDecode(t, rich101cProbeBlocks)
		bs := msg.RichMessage.Blocks
		assert.Equal(t, 1, rich101cBlock[tg.RichBlockSectionHeading](t, bs, 0).Size)
		assert.Equal(t, 6, rich101cBlock[tg.RichBlockSectionHeading](t, bs, 1).Size)
		assert.Equal(t, tg.RichBlockPreformatted{Type: "pre", Text: tg.RichText{Plain: "print('x')"}, Language: "python"},
			rich101cBlock[tg.RichBlockPreformatted](t, bs, 3))
		unordered := rich101cBlock[tg.RichBlockList](t, bs, 5)
		require.Len(t, unordered.Items, 2)
		assert.Equal(t, "•", unordered.Items[0].Label)
		assert.Empty(t, unordered.Items[0].Type)
		ordered := rich101cBlock[tg.RichBlockList](t, bs, 6)
		require.Len(t, ordered.Items, 2)
		assert.Equal(t, []string{"1.", "2."}, []string{ordered.Items[0].Label, ordered.Items[1].Label})
		assert.Equal(t, []string{"1", "1"}, []string{ordered.Items[0].Type, ordered.Items[1].Type})
		assert.Equal(t, []int{1, 2}, []int{ordered.Items[0].Value, ordered.Items[1].Value})
		tasks := rich101cBlock[tg.RichBlockList](t, bs, 7)
		require.Len(t, tasks.Items, 2)
		assert.Equal(t, []bool{true, true}, []bool{tasks.Items[0].HasCheckbox, tasks.Items[1].HasCheckbox})
		assert.Equal(t, []bool{false, true}, []bool{tasks.Items[0].IsChecked, tasks.Items[1].IsChecked})
		assert.Equal(t, tg.RichBlocks{tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Plain: "done"}}},
			tasks.Items[1].Blocks)
		table := rich101cBlock[tg.RichBlockTable](t, bs, 9)
		h1 := tg.RichText{Plain: "H1"}
		h2 := tg.RichText{Plain: "H2"}
		l := tg.RichText{Plain: "left"}
		c := tg.RichText{Plain: "center"}
		assert.Equal(t, tg.RichBlockTable{Type: "table", IsBordered: true, IsStriped: true, Cells: [][]tg.RichBlockTableCell{
			{{Text: &h1, IsHeader: true, Align: "left", VAlign: "middle"}, {Text: &h2, IsHeader: true, Align: "center", VAlign: "middle"}},
			{{Text: &l, Align: "left", VAlign: "middle"}, {Text: &c, Align: "center", VAlign: "middle"}},
		}}, table)
		assert.Equal(t, tg.RichBlockMathematicalExpression{Type: "mathematical_expression", Expression: "E = mc^2"},
			rich101cBlock[tg.RichBlockMathematicalExpression](t, bs, 11))
		details := rich101cBlock[tg.RichBlockDetails](t, bs, 12)
		assert.True(t, details.IsOpen)
		assert.Equal(t, tg.RichText{Parts: []tg.RichText{{Plain: "Summary "},
			{Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "bold"}}}}}, details.Summary)
		require.Len(t, details.Blocks, 2)
		assert.Equal(t, 3, rich101cBlock[tg.RichBlockSectionHeading](t, details.Blocks, 0).Size)
		footer := rich101cBlock[tg.RichBlockFooter](t, bs, 13)
		assert.Equal(t, []tg.RichTextReference{{Type: "reference", Text: tg.RichText{Plain: "Definition of the footnote."}, Name: "fn-id1"}},
			rich101cFindText[tg.RichTextReference](t, footer.Text))
		assert.Equal(t, []tg.RichTextAnchorLink{{Type: "anchor_link", Text: tg.RichText{Plain: "↩"}, AnchorName: "fnref-id1-1"}},
			rich101cFindText[tg.RichTextAnchorLink](t, footer.Text))
		var anchors []tg.RichTextAnchor
		var refLinks []tg.RichTextReferenceLink
		for _, b := range bs {
			if p, ok := b.(tg.RichBlockParagraph); ok {
				anchors = append(anchors, rich101cFindText[tg.RichTextAnchor](t, p.Text)...)
				refLinks = append(refLinks, rich101cFindText[tg.RichTextReferenceLink](t, p.Text)...)
			}
		}
		require.Len(t, anchors, 1)
		require.Len(t, refLinks, 1)
		assert.NotEmpty(t, anchors[0].Name)
		assert.NotEmpty(t, refLinks[0].ReferenceName)
	})
	t.Run("03-html", func(t *testing.T) {
		msg, _ := rich101cDecode(t, rich101cProbeHTML)
		bs := msg.RichMessage.Blocks
		assert.Equal(t, tg.RichBlockAnchor{Type: "anchor", Name: "chapter-1"}, rich101cBlock[tg.RichBlockAnchor](t, bs, 0))
		p := rich101cBlock[tg.RichBlockParagraph](t, bs, 1)
		assert.Equal(t, []tg.RichTextAnchorLink{{Type: "anchor_link", Text: tg.RichText{Plain: "anchor link"}, AnchorName: "chapter-1"}},
			rich101cFindText[tg.RichTextAnchorLink](t, p.Text))
		credit := tg.RichText{Plain: "The Author"}
		assert.Equal(t, tg.RichBlockPullQuotation{Type: "pullquote", Text: tg.RichText{Plain: "Pull quote"}, Credit: &credit},
			rich101cBlock[tg.RichBlockPullQuotation](t, bs, 2))
		details := rich101cBlock[tg.RichBlockDetails](t, bs, 3)
		assert.False(t, details.IsOpen)
		assert.Equal(t, tg.RichText{Plain: "Closed"}, details.Summary)
		assert.Equal(t, tg.RichBlockFooter{Type: "footer", Text: tg.RichText{Plain: "Footer text"}}, rich101cBlock[tg.RichBlockFooter](t, bs, 4))
		assert.Equal(t, tg.RichBlockMap{Type: "map", Location: tg.Location{Latitude: 41.900006, Longitude: 12.500002}, Zoom: 14, Width: 800, Height: 400},
			rich101cBlock[tg.RichBlockMap](t, bs, 5))
	})
	t.Run("04-media", func(t *testing.T) {
		msg, _ := rich101cDecode(t, rich101cProbeMedia)
		bs := msg.RichMessage.Blocks
		photo := rich101cBlock[tg.RichBlockPhoto](t, bs, 0)
		require.Len(t, photo.Photo, 5)
		assert.Equal(t, tg.PhotoSize{FileID: "FILEID-5", FileUniqueID: "UNIQ-5", FileSize: 202060, Width: 1920, Height: 1080}, photo.Photo[4])
		require.NotNil(t, photo.Caption)
		assert.Equal(t, tg.RichBlockCaption{Text: tg.RichText{Plain: "Photo caption"}}, *photo.Caption)
		collage := rich101cBlock[tg.RichBlockCollage](t, bs, 1)
		require.Len(t, collage.Blocks, 2)
		rich101cBlock[tg.RichBlockPhoto](t, collage.Blocks, 0)
		rich101cBlock[tg.RichBlockPhoto](t, collage.Blocks, 1)
	})
	t.Run("05-rtl-plain", func(t *testing.T) {
		msg, _ := rich101cDecode(t, rich101cProbeRTLPlain)
		assert.Equal(t, &tg.RichMessage{IsRTL: true, Blocks: tg.RichBlocks{
			tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Plain: "plain only"}}}}, msg.RichMessage)
	})
}

// T-rm-4
func TestAPI101C_MessageWithoutRich(t *testing.T) {
	out, err := json.Marshal(tg.Message{MessageID: 1})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "rich_message")
	var msg tg.Message
	require.NoError(t, json.Unmarshal([]byte(`{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"text":"hi"}`), &msg))
	assert.Nil(t, msg.RichMessage)
	assert.Equal(t, "hi", msg.Text)
	out, err = json.Marshal(tg.Message{MessageID: 1, RichMessage: &tg.RichMessage{Blocks: tg.RichBlocks{}}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"rich_message":{"blocks":[]}`)
}

// T-rm-5
func TestAPI101C_Update(t *testing.T) {
	in := `{"update_id":5,"message":{"message_id":9,"date":1,"chat":{"id":1,"type":"private"},"rich_message":{"blocks":[` +
		`{"type":"expandable_blockquote","blocks":[]},{"type":"heading","text":"a","size":"big"},5]}}}`
	var u tg.Update
	require.NoError(t, json.Unmarshal([]byte(in), &u))
	require.NotNil(t, u.Message)
	require.NotNil(t, u.Message.RichMessage)
	assert.Equal(t, tg.RichBlocks{
		tg.RichBlockUnknown{Type: "expandable_blockquote", Raw: json.RawMessage(`{"type":"expandable_blockquote","blocks":[]}`)},
		tg.RichBlockUnknown{Type: "heading", Raw: json.RawMessage(`{"type":"heading","text":"a","size":"big"}`)},
		tg.RichBlockUnknown{Raw: json.RawMessage(`5`)},
	}, u.Message.RichMessage.Blocks)
}

// T-rm-6
func TestAPI101C_DecodeTwice(t *testing.T) {
	a, _ := rich101cDecode(t, rich101cProbeBlocks)
	b, _ := rich101cDecode(t, rich101cProbeBlocks)
	assert.Equal(t, a, b)
}

type rich101cHolder struct {
	T tg.RichText `json:"t"`
}

// T-rt-1
func TestAPI101C_RichTextForms(t *testing.T) {
	cases := []struct {
		in   string
		want tg.RichText
		enc  string // expected re-encode; "" means the input itself
	}{
		{`"abc"`, tg.RichText{Plain: "abc"}, ""},
		{`""`, tg.RichText{}, ""},
		{`[]`, tg.RichText{Parts: []tg.RichText{}}, ""},
		{`["a",{"type":"bold","text":"b"}]`, tg.RichText{Parts: []tg.RichText{{Plain: "a"},
			{Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "b"}}}}}, ""},
		{`[["x"],"y"]`, tg.RichText{Parts: []tg.RichText{{Parts: []tg.RichText{{Plain: "x"}}}, {Plain: "y"}}}, ""},
		{`{"type":"italic","text":["p",{"type":"code","text":"q"}]}`, tg.RichText{Node: tg.RichTextItalic{Type: "italic",
			Text: tg.RichText{Parts: []tg.RichText{{Plain: "p"}, {Node: tg.RichTextCode{Type: "code", Text: tg.RichText{Plain: "q"}}}}}}}, ""},
		{`null`, tg.RichText{}, `""`},
		{`["a",null,"b"]`, tg.RichText{Parts: []tg.RichText{{Plain: "a"}, {}, {Plain: "b"}}}, `["a","","b"]`}, // C1-01
		{`123`, tg.RichText{Node: tg.RichTextUnknown{Raw: json.RawMessage(`123`)}}, ""},
		{`true`, tg.RichText{Node: tg.RichTextUnknown{Raw: json.RawMessage(`true`)}}, ""},
		{`"%2f +x "`, tg.RichText{Plain: "%2f +x "}, ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			var h rich101cHolder
			require.NoError(t, json.Unmarshal([]byte(`{"t":`+c.in+`}`), &h))
			assert.Equal(t, c.want, h.T)
			out, err := json.Marshal(h)
			require.NoError(t, err)
			enc := c.enc
			if enc == "" {
				enc = c.in
			}
			assert.JSONEq(t, `{"t":`+enc+`}`, string(out))
		})
	}
}

// T-rt-2
func TestAPI101C_RichTextDirect(t *testing.T) {
	cases := []struct {
		name  string
		start tg.RichText
		in    string
		want  tg.RichText
	}{
		{"leading spaces", tg.RichText{}, `  "a"`, tg.RichText{Plain: "a"}},
		{"bad string", tg.RichText{}, `"\q"`, tg.RichText{Node: tg.RichTextUnknown{Raw: json.RawMessage(`"\q"`)}}},
		{"bad array", tg.RichText{}, `[1,`, tg.RichText{Node: tg.RichTextUnknown{Raw: json.RawMessage(`[1,`)}}},
		{"empty input", tg.RichText{}, ``, tg.RichText{Node: tg.RichTextUnknown{Raw: nil}}}, // a copy of no bytes is nil
		{"stale plain", tg.RichText{Plain: "stale"}, `{"type":"bold","text":"x"}`,
			tg.RichText{Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "x"}}}},
		{"stale node", tg.RichText{Node: tg.RichTextBold{Type: "bold"}}, `"s"`, tg.RichText{Plain: "s"}},
		{"stale parts", tg.RichText{Parts: []tg.RichText{{Plain: "p"}}}, `null`, tg.RichText{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.start
			require.NoError(t, r.UnmarshalJSON([]byte(c.in)))
			assert.Equal(t, c.want, r)
		})
	}
}

// T-rt-3
func TestAPI101C_RichTextNodeFallback(t *testing.T) {
	cases := []struct{ in, typ string }{
		{`{"type":"date_time","text":"x","unix_time":"soon","date_time_format":"f"}`, "date_time"},
		{`{"type":"button","text":"b","button":{}}`, "button"},
		{`{"type":5}`, ""},
		{`{}`, ""},
		{`{"text":"x"}`, ""},
		{`{"type":"Bold","text":"a"}`, "Bold"},
		{`{"type":"bold ","text":"a"}`, "bold "},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			var r tg.RichText
			require.NoError(t, json.Unmarshal([]byte(c.in), &r))
			assert.Equal(t, tg.RichText{Node: tg.RichTextUnknown{Type: c.typ, Raw: json.RawMessage(c.in)}}, r)
			out, err := json.Marshal(r)
			require.NoError(t, err)
			assert.True(t, bytes.Equal([]byte(c.in), out), "re-encode %s", out)
		})
	}
}

// rich101cKind is one spec-shape row: a JSON object with every field of the kind set, and the exact Go value.
type rich101cKind struct {
	name string
	live bool // the kind was seen in the live probe
	in   string
	want any
}

func rich101cText(s string) tg.RichText { return tg.RichText{Plain: s} }

func rich101cTextKinds() []rich101cKind {
	t := rich101cText("t")
	return []rich101cKind{
		{"bold", true, `{"type":"bold","text":"t"}`, tg.RichTextBold{Type: "bold", Text: t}},
		{"italic", true, `{"type":"italic","text":"t"}`, tg.RichTextItalic{Type: "italic", Text: t}},
		{"underline", true, `{"type":"underline","text":"t"}`, tg.RichTextUnderline{Type: "underline", Text: t}},
		{"strikethrough", true, `{"type":"strikethrough","text":"t"}`, tg.RichTextStrikethrough{Type: "strikethrough", Text: t}},
		{"spoiler", true, `{"type":"spoiler","text":"t"}`, tg.RichTextSpoiler{Type: "spoiler", Text: t}},
		{"date_time", true, `{"type":"date_time","text":"t","unix_time":2500000000,"date_time_format":"wDT"}`,
			tg.RichTextDateTime{Type: "date_time", Text: t, UnixTime: 2500000000, DateTimeFormat: "wDT"}},
		{"text_mention", false, `{"type":"text_mention","text":"t","user":{"id":42,"is_bot":false,"first_name":"Ann"}}`,
			tg.RichTextTextMention{Type: "text_mention", Text: t, User: tg.User{ID: 42, FirstName: "Ann"}}},
		{"subscript", true, `{"type":"subscript","text":"t"}`, tg.RichTextSubscript{Type: "subscript", Text: t}},
		{"superscript", true, `{"type":"superscript","text":"t"}`, tg.RichTextSuperscript{Type: "superscript", Text: t}},
		{"marked", true, `{"type":"marked","text":"t"}`, tg.RichTextMarked{Type: "marked", Text: t}},
		{"code", true, `{"type":"code","text":"t"}`, tg.RichTextCode{Type: "code", Text: t}},
		{"custom_emoji", false, `{"type":"custom_emoji","custom_emoji_id":"5368324170671202286","alternative_text":"👍"}`,
			tg.RichTextCustomEmoji{Type: "custom_emoji", CustomEmojiID: "5368324170671202286", AlternativeText: "👍"}},
		{"mathematical_expression", true, `{"type":"mathematical_expression","expression":"a + b"}`,
			tg.RichTextMathematicalExpression{Type: "mathematical_expression", Expression: "a + b"}},
		{"url", true, `{"type":"url","text":"t","url":"HTTPS://e.x/A%2fb?z=1&a=2#F"}`,
			tg.RichTextURL{Type: "url", Text: t, URL: "HTTPS://e.x/A%2fb?z=1&a=2#F"}},
		{"email_address", true, `{"type":"email_address","text":"t","email_address":"A@b.c"}`,
			tg.RichTextEmailAddress{Type: "email_address", Text: t, EmailAddress: "A@b.c"}},
		{"phone_number", true, `{"type":"phone_number","text":"t","phone_number":"+1 (23) 45"}`,
			tg.RichTextPhoneNumber{Type: "phone_number", Text: t, PhoneNumber: "+1 (23) 45"}},
		{"bank_card_number", false, `{"type":"bank_card_number","text":"t","bank_card_number":"4242 4242 4242 4242"}`,
			tg.RichTextBankCardNumber{Type: "bank_card_number", Text: t, BankCardNumber: "4242 4242 4242 4242"}},
		{"mention", true, `{"type":"mention","text":"t","username":"user_1"}`, tg.RichTextMention{Type: "mention", Text: t, Username: "user_1"}},
		{"hashtag", true, `{"type":"hashtag","text":"t","hashtag":"tag"}`, tg.RichTextHashtag{Type: "hashtag", Text: t, Hashtag: "tag"}},
		{"cashtag", true, `{"type":"cashtag","text":"t","cashtag":"USD"}`, tg.RichTextCashtag{Type: "cashtag", Text: t, Cashtag: "USD"}},
		{"bot_command", true, `{"type":"bot_command","text":"t","bot_command":"start"}`,
			tg.RichTextBotCommand{Type: "bot_command", Text: t, BotCommand: "start"}},
		{"anchor", true, `{"type":"anchor","name":"n 1"}`, tg.RichTextAnchor{Type: "anchor", Name: "n 1"}},
		{"anchor_link", true, `{"type":"anchor_link","text":"t","anchor_name":"n 1"}`,
			tg.RichTextAnchorLink{Type: "anchor_link", Text: t, AnchorName: "n 1"}},
		{"anchor_link empty name", true, `{"type":"anchor_link","text":"t","anchor_name":""}`,
			tg.RichTextAnchorLink{Type: "anchor_link", Text: t, AnchorName: ""}},
		{"reference", true, `{"type":"reference","text":"t","name":"r"}`, tg.RichTextReference{Type: "reference", Text: t, Name: "r"}},
		{"reference_link", true, `{"type":"reference_link","text":"t","reference_name":"r"}`,
			tg.RichTextReferenceLink{Type: "reference_link", Text: t, ReferenceName: "r"}},
	}
}

// rich101cDecodeFunc decodes one kind and encodes it again; it takes the (sub)test's t, so a failure stops that test.
type rich101cDecodeFunc func(t *testing.T, b []byte) (any, []byte)

func rich101cCheckKind(t *testing.T, k rich101cKind, decode rich101cDecodeFunc) {
	t.Helper()
	got, out := decode(t, []byte(k.in))
	assert.Equal(t, k.want, got)
	assert.JSONEq(t, k.in, string(out))
}

// T-rt-4
func TestAPI101C_RichTextKinds(t *testing.T) {
	decode := func(t *testing.T, b []byte) (any, []byte) {
		t.Helper()
		var r tg.RichText
		require.NoError(t, json.Unmarshal(b, &r))
		out, err := json.Marshal(r)
		require.NoError(t, err)
		return r.Node, out
	}
	for _, k := range rich101cTextKinds() {
		t.Run(k.name, func(t *testing.T) { rich101cCheckKind(t, k, decode) })
	}
	t.Run("extra field", func(t *testing.T) {
		got, _ := decode(t, []byte(`{"type":"bold","text":"x","future_field":123}`))
		assert.Equal(t, tg.RichTextBold{Type: "bold", Text: rich101cText("x")}, got)
	})
	t.Run("all kinds listed", func(t *testing.T) {
		seen := map[string]bool{}
		for _, k := range rich101cTextKinds() {
			seen[fmt.Sprintf("%T", k.want)] = true
		}
		assert.Len(t, seen, 25)
	})
}

// T-rt-5
func TestAPI101C_RichTextEncode(t *testing.T) {
	bold := tg.RichTextBold{Type: "bold", Text: rich101cText("b")}
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"zero", tg.RichText{}, `""`},
		{"plain", tg.RichText{Plain: "a"}, `"a"`},
		{"empty parts", tg.RichText{Parts: []tg.RichText{}}, `[]`},
		{"nil parts", tg.RichText{Parts: nil}, `""`},
		{"node wins over plain", tg.RichText{Node: bold, Plain: "x"}, `{"type":"bold","text":"b"}`},
		{"parts win over plain", tg.RichText{Parts: []tg.RichText{{Plain: "p"}}, Plain: "x"}, `["p"]`},
		{"node wins over parts", tg.RichText{Node: bold, Parts: []tg.RichText{{Plain: "p"}}}, `{"type":"bold","text":"b"}`},
		{"unknown without raw", tg.RichTextUnknown{Type: "x"}, `{"type":"x"}`},
		{"unknown with raw", tg.RichTextUnknown{Raw: json.RawMessage(`{"type":"y","k":[1]}`)}, `{"type":"y","k":[1]}`},
		{"unknown without raw in a RichText", tg.RichText{Node: tg.RichTextUnknown{Type: "x"}}, `{"type":"x"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := json.Marshal(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, string(out))
		})
	}
}

func rich101cCaption() *tg.RichBlockCaption {
	credit := rich101cText("c")
	return &tg.RichBlockCaption{Text: rich101cText("cap"), Credit: &credit}
}

const rich101cCaptionJSON = `"caption":{"text":"cap","credit":"c"}`

func rich101cBlockKinds() []rich101cKind {
	t := rich101cText("t")
	inner := tg.RichBlocks{tg.RichBlockParagraph{Type: "paragraph", Text: rich101cText("n")}}
	const innerJSON = `"blocks":[{"type":"paragraph","text":"n"}]`
	credit := rich101cText("c")
	cell := rich101cText("x")
	return []rich101cKind{
		{"paragraph", true, `{"type":"paragraph","text":"t"}`, tg.RichBlockParagraph{Type: "paragraph", Text: t}},
		{"heading", true, `{"type":"heading","text":"t","size":2}`, tg.RichBlockSectionHeading{Type: "heading", Text: t, Size: 2}},
		{"pre", true, `{"type":"pre","text":"t","language":"go"}`, tg.RichBlockPreformatted{Type: "pre", Text: t, Language: "go"}},
		{"footer", true, `{"type":"footer","text":"t"}`, tg.RichBlockFooter{Type: "footer", Text: t}},
		{"divider", true, `{"type":"divider"}`, tg.RichBlockDivider{Type: "divider"}},
		{"mathematical_expression", true, `{"type":"mathematical_expression","expression":"a = b"}`,
			tg.RichBlockMathematicalExpression{Type: "mathematical_expression", Expression: "a = b"}},
		{"anchor", true, `{"type":"anchor","name":"a"}`, tg.RichBlockAnchor{Type: "anchor", Name: "a"}},
		{"list", true, `{"type":"list","items":[{"label":"1.",` + innerJSON + `,"has_checkbox":true,"is_checked":true,"value":3,"type":"a"}]}`,
			tg.RichBlockList{Type: "list", Items: []tg.RichBlockListItem{{Label: "1.", Blocks: inner, HasCheckbox: true, IsChecked: true, Value: 3, Type: "a"}}}},
		{"blockquote", true, `{"type":"blockquote",` + innerJSON + `,"credit":"c"}`,
			tg.RichBlockBlockQuotation{Type: "blockquote", Blocks: inner, Credit: &credit}},
		{"pullquote", true, `{"type":"pullquote","text":"t","credit":"c"}`, tg.RichBlockPullQuotation{Type: "pullquote", Text: t, Credit: &credit}},
		{"collage", true, `{"type":"collage",` + innerJSON + `,` + rich101cCaptionJSON + `}`,
			tg.RichBlockCollage{Type: "collage", Blocks: inner, Caption: rich101cCaption()}},
		{"slideshow", false, `{"type":"slideshow",` + innerJSON + `,` + rich101cCaptionJSON + `}`,
			tg.RichBlockSlideshow{Type: "slideshow", Blocks: inner, Caption: rich101cCaption()}},
		{"table", true, `{"type":"table","cells":[[{"text":"x","is_header":true,"colspan":2,"rowspan":3,"align":"right","valign":"bottom"}]],` +
			`"is_bordered":true,"is_striped":true,"caption":"t"}`,
			tg.RichBlockTable{Type: "table", Cells: [][]tg.RichBlockTableCell{{{Text: &cell, IsHeader: true, Colspan: 2, Rowspan: 3,
				Align: "right", VAlign: "bottom"}}}, IsBordered: true, IsStriped: true, Caption: &t}},
		{"details", true, `{"type":"details","summary":"t",` + innerJSON + `,"is_open":true}`,
			tg.RichBlockDetails{Type: "details", Summary: t, Blocks: inner, IsOpen: true}},
		{"map", true, `{"type":"map","location":{"longitude":1.5,"latitude":2.5},"zoom":3,"width":4,"height":5,` + rich101cCaptionJSON + `}`,
			tg.RichBlockMap{Type: "map", Location: tg.Location{Longitude: 1.5, Latitude: 2.5}, Zoom: 3, Width: 4, Height: 5, Caption: rich101cCaption()}},
		{"animation", false, `{"type":"animation","animation":{"file_id":"F","file_unique_id":"U","width":1,"height":2,"duration":3},` +
			`"has_spoiler":true,` + rich101cCaptionJSON + `}`,
			tg.RichBlockAnimation{Type: "animation", Animation: tg.Animation{FileID: "F", FileUniqueID: "U", Width: 1, Height: 2, Duration: 3},
				HasSpoiler: true, Caption: rich101cCaption()}},
		{"audio", false, `{"type":"audio","audio":{"file_id":"F","file_unique_id":"U","duration":3},` + rich101cCaptionJSON + `}`,
			tg.RichBlockAudio{Type: "audio", Audio: tg.Audio{FileID: "F", FileUniqueID: "U", Duration: 3}, Caption: rich101cCaption()}},
		{"photo", true, `{"type":"photo","photo":[{"file_id":"F","file_unique_id":"U","width":1,"height":2}],"has_spoiler":true,` +
			rich101cCaptionJSON + `}`,
			tg.RichBlockPhoto{Type: "photo", Photo: []tg.PhotoSize{{FileID: "F", FileUniqueID: "U", Width: 1, Height: 2}}, HasSpoiler: true,
				Caption: rich101cCaption()}},
		{"video", false, `{"type":"video","video":{"file_id":"F","file_unique_id":"U","width":1,"height":2,"duration":3},"has_spoiler":true,` +
			rich101cCaptionJSON + `}`,
			tg.RichBlockVideo{Type: "video", Video: tg.Video{FileID: "F", FileUniqueID: "U", Width: 1, Height: 2, Duration: 3}, HasSpoiler: true,
				Caption: rich101cCaption()}},
		{"voice_note", false, `{"type":"voice_note","voice_note":{"file_id":"F","file_unique_id":"U","duration":3},` + rich101cCaptionJSON + `}`,
			tg.RichBlockVoiceNote{Type: "voice_note", VoiceNote: tg.Voice{FileID: "F", FileUniqueID: "U", Duration: 3}, Caption: rich101cCaption()}},
		{"thinking", false, `{"type":"thinking","text":"t"}`, tg.RichBlockThinking{Type: "thinking", Text: t}},
	}
}

func rich101cDecodeBlock(t *testing.T, b []byte) (any, []byte) {
	t.Helper()
	var bs tg.RichBlocks
	require.NoError(t, json.Unmarshal(append(append([]byte("["), b...), ']'), &bs))
	require.Len(t, bs, 1)
	out, err := json.Marshal(bs[0])
	require.NoError(t, err)
	return bs[0], out
}

// T-rb-1
func TestAPI101C_RichBlockKinds(t *testing.T) {
	for _, k := range rich101cBlockKinds() {
		t.Run(k.name, func(t *testing.T) { rich101cCheckKind(t, k, rich101cDecodeBlock) })
	}
	t.Run("extra field", func(t *testing.T) {
		got, _ := rich101cDecodeBlock(t, []byte(`{"type":"divider","future":true}`))
		assert.Equal(t, tg.RichBlockDivider{Type: "divider"}, got)
	})
	t.Run("all kinds listed", func(t *testing.T) {
		seen := map[string]bool{}
		for _, k := range rich101cBlockKinds() {
			seen[fmt.Sprintf("%T", k.want)] = true
		}
		assert.Len(t, seen, 21)
	})
}

type rich101cBlocksHolder struct {
	B tg.RichBlocks `json:"b"`
}

// T-rb-2
func TestAPI101C_RichBlocksForms(t *testing.T) {
	decode := func(t *testing.T, in string) tg.RichBlocks {
		t.Helper()
		var h rich101cBlocksHolder
		require.NoError(t, json.Unmarshal([]byte(`{"b":`+in+`}`), &h))
		return h.B
	}
	t.Run("empty", func(t *testing.T) {
		b := decode(t, `[]`)
		require.NotNil(t, b)
		assert.Empty(t, b)
		out, err := json.Marshal(rich101cBlocksHolder{B: b})
		require.NoError(t, err)
		assert.Equal(t, `{"b":[]}`, string(out))
	})
	t.Run("null", func(t *testing.T) { assert.Nil(t, decode(t, `null`)) })
	t.Run("string", func(t *testing.T) {
		assert.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Raw: json.RawMessage(`"x"`)}}, decode(t, `"x"`))
	})
	t.Run("object", func(t *testing.T) {
		in := `{"type":"paragraph","text":"a"}`
		assert.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Raw: json.RawMessage(in)}}, decode(t, in))
	})
	t.Run("mixed", func(t *testing.T) {
		assert.Equal(t, tg.RichBlocks{
			tg.RichBlockDivider{Type: "divider"},
			tg.RichBlockUnknown{Type: "document", Raw: json.RawMessage(`{"type":"document"}`)},
			tg.RichBlockUnknown{Raw: json.RawMessage(`5`)},
			tg.RichBlockUnknown{Type: "heading", Raw: json.RawMessage(`{"type":"heading","size":"x"}`)},
		}, decode(t, `[{"type":"divider"},{"type":"document"},5,{"type":"heading","size":"x"}]`))
	})
	t.Run("as-given type", func(t *testing.T) {
		assert.Equal(t, tg.RichBlocks{
			tg.RichBlockUnknown{Type: " paragraph", Raw: json.RawMessage(`{"type":" paragraph","text":"a"}`)},
			tg.RichBlockUnknown{Type: "DIVIDER", Raw: json.RawMessage(`{"type":"DIVIDER"}`)},
		}, decode(t, `[{"type":" paragraph","text":"a"},{"type":"DIVIDER"}]`))
	})
}

// T-rb-3
func TestAPI101C_NestedHolders(t *testing.T) {
	const blocks = `[{"type":"paragraph","text":"n"},{"type":"document"}]`
	want := tg.RichBlocks{
		tg.RichBlockParagraph{Type: "paragraph", Text: rich101cText("n")},
		tg.RichBlockUnknown{Type: "document", Raw: json.RawMessage(`{"type":"document"}`)},
	}
	var rm tg.RichMessage
	require.NoError(t, json.Unmarshal([]byte(`{"blocks":`+blocks+`}`), &rm))
	assert.Equal(t, want, rm.Blocks, "RichMessage")
	var item tg.RichBlockListItem
	require.NoError(t, json.Unmarshal([]byte(`{"label":"-","blocks":`+blocks+`}`), &item))
	assert.Equal(t, want, item.Blocks, "RichBlockListItem")
	for _, ty := range []string{"blockquote", "collage", "slideshow", "details"} {
		var bs tg.RichBlocks
		require.NoError(t, json.Unmarshal([]byte(`[{"type":"`+ty+`","blocks":`+blocks+`}]`), &bs))
		require.Len(t, bs, 1)
		got := reflect.ValueOf(bs[0]).FieldByName("Blocks")
		require.True(t, got.IsValid(), "%s is %T", ty, bs[0])
		assert.Equal(t, want, got.Interface(), ty)
	}
}

// T-rb-5
func TestAPI101C_RichBlockUnknownEncode(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"no raw", tg.RichBlockUnknown{Type: "custom"}, `{"type":"custom"}`},
		{"raw", tg.RichBlockUnknown{Raw: json.RawMessage(`{"type":"x","a":1}`)}, `{"type":"x","a":1}`},
		{"in blocks", tg.RichBlocks{tg.RichBlockUnknown{Type: "custom"}, tg.RichBlockDivider{Type: "divider"}},
			`[{"type":"custom"},{"type":"divider"}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := json.Marshal(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, string(out))
		})
	}
}

// rich101cFixtureValues returns, for each fixture, the JSON of every value under the given key, depth first.
func rich101cFixtureValues(f *testing.F, key string) [][]byte {
	f.Helper()
	var out [][]byte
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if k == key {
					b, err := json.Marshal(v[k])
					require.NoError(f, err)
					out = append(out, b)
				}
				walk(v[k])
			}
		case []any:
			for _, e := range v {
				walk(e)
			}
		}
	}
	for _, fx := range rich101cFixtures {
		var v any
		require.NoError(f, json.Unmarshal([]byte(fx.json), &v))
		walk(v.(map[string]any)["rich_message"])
	}
	require.NotEmpty(f, out, key)
	return out
}

// rich101cFuzzStable checks properties (2) and (3) of the fuzz rows: out1 is valid JSON, and decoding and encoding it
// again gives the same bytes.
func rich101cFuzzStable(t *testing.T, out1 []byte, decode func([]byte) any) {
	t.Helper()
	require.True(t, json.Valid(out1), "output %q", out1)
	out2, err := json.Marshal(decode(out1))
	require.NoError(t, err)
	require.Equal(t, string(out1), string(out2))
}

// FuzzRichText: TEST-PLAN section 4.
func FuzzRichText(f *testing.F) {
	for _, s := range []string{`"abc"`, `""`, `[]`, `["a",{"type":"bold","text":"b"}]`, `[["x"],"y"]`,
		`{"type":"italic","text":["p",{"type":"code","text":"q"}]}`, `null`, `123`, `true`, `"%2f +x "`,
		`  "a"`, `"\q"`, `[1,`, ``, `{"type":"bold","text":"x"}`, `"s"`, `{invalid`, `[`, `"A"`} {
		f.Add([]byte(s))
	}
	for _, b := range rich101cFixtureValues(f, "text") {
		f.Add(b)
	}
	decode := func(b []byte) any {
		var r tg.RichText
		_ = r.UnmarshalJSON(b)
		return r
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var r tg.RichText
		require.NoError(t, r.UnmarshalJSON(data))
		if !json.Valid(data) {
			return
		}
		out1, err := json.Marshal(r)
		require.NoError(t, err)
		rich101cFuzzStable(t, out1, decode)
		if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '"' {
			var v string
			require.NoError(t, json.Unmarshal(data, &v))
			want, err := json.Marshal(v)
			require.NoError(t, err)
			require.Equal(t, string(want), string(out1))
		}
	})
}

// FuzzRichBlocks: TEST-PLAN section 4.
func FuzzRichBlocks(f *testing.F) {
	for _, s := range []string{`[]`, `null`, `"x"`, `{"type":"paragraph","text":"a"}`,
		`[{"type":"divider"},{"type":"document"},5,{"type":"heading","size":"x"}]`,
		`[{"type":" paragraph","text":"a"},{"type":"DIVIDER"}]`, `{invalid`, `[`} {
		f.Add([]byte(s))
	}
	for _, b := range rich101cFixtureValues(f, "blocks") {
		f.Add(b)
	}
	decode := func(b []byte) any {
		var bs tg.RichBlocks
		_ = bs.UnmarshalJSON(b)
		return bs
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var bs tg.RichBlocks
		require.NoError(t, bs.UnmarshalJSON(data))
		if !json.Valid(data) {
			return
		}
		var raws []json.RawMessage
		if json.Unmarshal(data, &raws) != nil {
			// No array and not null: one Unknown with the input (D3). It encodes as a one-element array, which a
			// second decode may read as a known kind, so the stability property does not apply (A-08).
			require.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Raw: json.RawMessage(data)}}, bs)
			return
		}
		out1, err := json.Marshal(bs)
		require.NoError(t, err)
		rich101cFuzzStable(t, out1, decode)
		require.Len(t, bs, len(raws))
	})
}

type rich101cShape struct{ typ, field, goType, tag string }

// rich101cTypes is every struct of the field table, by name.
var rich101cTypes = map[string]reflect.Type{}

func init() {
	for _, v := range []any{
		tg.RichTextBold{}, tg.RichTextItalic{}, tg.RichTextUnderline{}, tg.RichTextStrikethrough{}, tg.RichTextSpoiler{},
		tg.RichTextDateTime{}, tg.RichTextTextMention{}, tg.RichTextSubscript{}, tg.RichTextSuperscript{}, tg.RichTextMarked{},
		tg.RichTextCode{}, tg.RichTextCustomEmoji{}, tg.RichTextMathematicalExpression{}, tg.RichTextURL{},
		tg.RichTextEmailAddress{}, tg.RichTextPhoneNumber{}, tg.RichTextBankCardNumber{}, tg.RichTextMention{},
		tg.RichTextHashtag{}, tg.RichTextCashtag{}, tg.RichTextBotCommand{}, tg.RichTextAnchor{}, tg.RichTextAnchorLink{},
		tg.RichTextReference{}, tg.RichTextReferenceLink{},
		tg.RichBlockCaption{}, tg.RichBlockTableCell{}, tg.RichBlockListItem{},
		tg.RichBlockParagraph{}, tg.RichBlockSectionHeading{}, tg.RichBlockPreformatted{}, tg.RichBlockFooter{},
		tg.RichBlockDivider{}, tg.RichBlockMathematicalExpression{}, tg.RichBlockAnchor{}, tg.RichBlockList{},
		tg.RichBlockBlockQuotation{}, tg.RichBlockPullQuotation{}, tg.RichBlockCollage{}, tg.RichBlockSlideshow{},
		tg.RichBlockTable{}, tg.RichBlockDetails{}, tg.RichBlockMap{}, tg.RichBlockAnimation{}, tg.RichBlockAudio{},
		tg.RichBlockPhoto{}, tg.RichBlockVideo{}, tg.RichBlockVoiceNote{}, tg.RichBlockThinking{},
		tg.RichMessage{},
	} {
		rt := reflect.TypeOf(v)
		rich101cTypes[rt.Name()] = rt
	}
}

// rich101cTypeString names a Go type by its name when it has one, so []RichBlock is not RichBlocks.
func rich101cTypeString(t reflect.Type) string {
	if t.Name() != "" {
		return t.Name()
	}
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + rich101cTypeString(t.Elem())
	case reflect.Slice:
		return "[]" + rich101cTypeString(t.Elem())
	default:
		return t.String()
	}
}

// T-shape
func TestAPI101C_Shapes(t *testing.T) {
	require.Len(t, rich101cShapes, 145)
	byType := map[string][]rich101cShape{}
	for _, s := range rich101cShapes {
		byType[s.typ] = append(byType[s.typ], s)
	}
	require.Len(t, byType, 50)
	require.Len(t, rich101cTypes, 50)
	for name, rows := range byType {
		rt, ok := rich101cTypes[name]
		require.True(t, ok, name)
		var got []rich101cShape
		for i := range rt.NumField() {
			f := rt.Field(i)
			if f.IsExported() {
				got = append(got, rich101cShape{name, f.Name, rich101cTypeString(f.Type), f.Tag.Get("json")})
			}
		}
		assert.Equal(t, rows, got, name)
	}
}

// T-rb-4
func TestAPI101C_ZeroEncode(t *testing.T) {
	byType := map[string][]string{}
	for _, s := range rich101cShapes {
		if _, ok := byType[s.typ]; !ok {
			byType[s.typ] = []string{}
		}
		if !bytes.HasSuffix([]byte(s.tag), []byte(",omitempty")) {
			byType[s.typ] = append(byType[s.typ], s.tag)
		}
	}
	for name, want := range byType {
		out, err := json.Marshal(reflect.Zero(rich101cTypes[name]).Interface())
		require.NoError(t, err, name)
		var keys map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &keys), name)
		var got []string
		for k := range keys {
			got = append(got, k)
		}
		sort.Strings(got)
		sort.Strings(want)
		assert.Equal(t, want, got, name)
	}
}

// T-seal
func TestAPI101C_Sealed(t *testing.T) {
	textNode := reflect.TypeOf((*tg.RichTextNode)(nil)).Elem()
	block := reflect.TypeOf((*tg.RichBlock)(nil)).Elem()
	var nText, nBlock int
	for name, rt := range rich101cTypes {
		switch {
		case name == "RichBlockCaption" || name == "RichBlockTableCell" || name == "RichBlockListItem" || name == "RichMessage":
			assert.False(t, rt.Implements(textNode), name)
			assert.False(t, rt.Implements(block), name)
		case len(name) > 8 && name[:8] == "RichText":
			assert.True(t, rt.Implements(textNode), name)
			assert.False(t, rt.Implements(block), name)
			nText++
		default:
			assert.True(t, rt.Implements(block), name)
			assert.False(t, rt.Implements(textNode), name)
			nBlock++
		}
	}
	assert.Equal(t, 25, nText)
	assert.Equal(t, 21, nBlock)
	assert.True(t, reflect.TypeOf(tg.RichTextUnknown{}).Implements(textNode))
	assert.True(t, reflect.TypeOf(tg.RichBlockUnknown{}).Implements(block))
	for _, v := range []any{tg.RichText{}, tg.RichBlocks{}} {
		assert.False(t, reflect.TypeOf(v).Implements(textNode))
		assert.False(t, reflect.TypeOf(v).Implements(block))
	}
}

// T-copy
func TestAPI101C_RawIsCopied(t *testing.T) {
	overwrite := func(b []byte) {
		for i := range b {
			b[i] = '#'
		}
	}
	// One row per copy site of (*RichText).UnmarshalJSON (MR-copy-1..7).
	for _, c := range []struct{ in, typ string }{
		{`"\q"`, ""}, {`[1,`, ""}, {`nan`, ""}, {`5`, ""}, {`{"type":5}`, ""}, {`{"type":"x"}`, "x"},
		{`{"type":"date_time","unix_time":"soon"}`, "date_time"},
	} {
		t.Run("text "+c.in, func(t *testing.T) {
			buf := []byte(c.in)
			var r tg.RichText
			require.NoError(t, r.UnmarshalJSON(buf))
			overwrite(buf)
			assert.Equal(t, tg.RichText{Node: tg.RichTextUnknown{Type: c.typ, Raw: json.RawMessage(c.in)}}, r)
		})
	}
	t.Run("text object in an object", func(t *testing.T) { // encoding/json passes the nested value as a slice of buf
		buf := []byte(`{"type":"bold","text":{"type":"x"}}`)
		var r tg.RichText
		require.NoError(t, r.UnmarshalJSON(buf))
		overwrite(buf)
		assert.Equal(t, tg.RichText{Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Node: tg.RichTextUnknown{Type: "x",
			Raw: json.RawMessage(`{"type":"x"}`)}}}}, r)
	})
	t.Run("block element", func(t *testing.T) { // copied by json.RawMessage, not by galigo: a guard
		buf := []byte(`[{"type":"x"}]`)
		var b tg.RichBlocks
		require.NoError(t, b.UnmarshalJSON(buf))
		overwrite(buf)
		assert.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Type: "x", Raw: json.RawMessage(`{"type":"x"}`)}}, b)
	})
	t.Run("blocks value", func(t *testing.T) {
		buf := []byte(`"x"`)
		var b tg.RichBlocks
		require.NoError(t, b.UnmarshalJSON(buf))
		overwrite(buf)
		assert.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Raw: json.RawMessage(`"x"`)}}, b)
	})
	t.Run("nested through json.Unmarshal", func(t *testing.T) {
		buf := []byte(`{"blocks":[{"type":"details","summary":{"type":"y"},"blocks":[{"type":"z"}]}]}`)
		var rm tg.RichMessage
		require.NoError(t, json.Unmarshal(buf, &rm))
		overwrite(buf)
		d := rich101cBlock[tg.RichBlockDetails](t, rm.Blocks, 0)
		assert.Equal(t, tg.RichText{Node: tg.RichTextUnknown{Type: "y", Raw: json.RawMessage(`{"type":"y"}`)}}, d.Summary)
		assert.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Type: "z", Raw: json.RawMessage(`{"type":"z"}`)}}, d.Blocks)
	})
}

// rich101cShapes is the field table of FIELDS-101c.md (generated from the Bot API 10.1 spec), one row per field.
var rich101cShapes = []rich101cShape{
	{"RichTextBold", "Type", "string", `type`},
	{"RichTextBold", "Text", "RichText", `text`},
	{"RichTextItalic", "Type", "string", `type`},
	{"RichTextItalic", "Text", "RichText", `text`},
	{"RichTextUnderline", "Type", "string", `type`},
	{"RichTextUnderline", "Text", "RichText", `text`},
	{"RichTextStrikethrough", "Type", "string", `type`},
	{"RichTextStrikethrough", "Text", "RichText", `text`},
	{"RichTextSpoiler", "Type", "string", `type`},
	{"RichTextSpoiler", "Text", "RichText", `text`},
	{"RichTextDateTime", "Type", "string", `type`},
	{"RichTextDateTime", "Text", "RichText", `text`},
	{"RichTextDateTime", "UnixTime", "int64", `unix_time`},
	{"RichTextDateTime", "DateTimeFormat", "string", `date_time_format`},
	{"RichTextTextMention", "Type", "string", `type`},
	{"RichTextTextMention", "Text", "RichText", `text`},
	{"RichTextTextMention", "User", "User", `user`},
	{"RichTextSubscript", "Type", "string", `type`},
	{"RichTextSubscript", "Text", "RichText", `text`},
	{"RichTextSuperscript", "Type", "string", `type`},
	{"RichTextSuperscript", "Text", "RichText", `text`},
	{"RichTextMarked", "Type", "string", `type`},
	{"RichTextMarked", "Text", "RichText", `text`},
	{"RichTextCode", "Type", "string", `type`},
	{"RichTextCode", "Text", "RichText", `text`},
	{"RichTextCustomEmoji", "Type", "string", `type`},
	{"RichTextCustomEmoji", "CustomEmojiID", "string", `custom_emoji_id`},
	{"RichTextCustomEmoji", "AlternativeText", "string", `alternative_text`},
	{"RichTextMathematicalExpression", "Type", "string", `type`},
	{"RichTextMathematicalExpression", "Expression", "string", `expression`},
	{"RichTextURL", "Type", "string", `type`},
	{"RichTextURL", "Text", "RichText", `text`},
	{"RichTextURL", "URL", "string", `url`},
	{"RichTextEmailAddress", "Type", "string", `type`},
	{"RichTextEmailAddress", "Text", "RichText", `text`},
	{"RichTextEmailAddress", "EmailAddress", "string", `email_address`},
	{"RichTextPhoneNumber", "Type", "string", `type`},
	{"RichTextPhoneNumber", "Text", "RichText", `text`},
	{"RichTextPhoneNumber", "PhoneNumber", "string", `phone_number`},
	{"RichTextBankCardNumber", "Type", "string", `type`},
	{"RichTextBankCardNumber", "Text", "RichText", `text`},
	{"RichTextBankCardNumber", "BankCardNumber", "string", `bank_card_number`},
	{"RichTextMention", "Type", "string", `type`},
	{"RichTextMention", "Text", "RichText", `text`},
	{"RichTextMention", "Username", "string", `username`},
	{"RichTextHashtag", "Type", "string", `type`},
	{"RichTextHashtag", "Text", "RichText", `text`},
	{"RichTextHashtag", "Hashtag", "string", `hashtag`},
	{"RichTextCashtag", "Type", "string", `type`},
	{"RichTextCashtag", "Text", "RichText", `text`},
	{"RichTextCashtag", "Cashtag", "string", `cashtag`},
	{"RichTextBotCommand", "Type", "string", `type`},
	{"RichTextBotCommand", "Text", "RichText", `text`},
	{"RichTextBotCommand", "BotCommand", "string", `bot_command`},
	{"RichTextAnchor", "Type", "string", `type`},
	{"RichTextAnchor", "Name", "string", `name`},
	{"RichTextAnchorLink", "Type", "string", `type`},
	{"RichTextAnchorLink", "Text", "RichText", `text`},
	{"RichTextAnchorLink", "AnchorName", "string", `anchor_name`},
	{"RichTextReference", "Type", "string", `type`},
	{"RichTextReference", "Text", "RichText", `text`},
	{"RichTextReference", "Name", "string", `name`},
	{"RichTextReferenceLink", "Type", "string", `type`},
	{"RichTextReferenceLink", "Text", "RichText", `text`},
	{"RichTextReferenceLink", "ReferenceName", "string", `reference_name`},
	{"RichBlockCaption", "Text", "RichText", `text`},
	{"RichBlockCaption", "Credit", "*RichText", `credit,omitempty`},
	{"RichBlockTableCell", "Text", "*RichText", `text,omitempty`},
	{"RichBlockTableCell", "IsHeader", "bool", `is_header,omitempty`},
	{"RichBlockTableCell", "Colspan", "int", `colspan,omitempty`},
	{"RichBlockTableCell", "Rowspan", "int", `rowspan,omitempty`},
	{"RichBlockTableCell", "Align", "string", `align`},
	{"RichBlockTableCell", "VAlign", "string", `valign`},
	{"RichBlockListItem", "Label", "string", `label`},
	{"RichBlockListItem", "Blocks", "RichBlocks", `blocks`},
	{"RichBlockListItem", "HasCheckbox", "bool", `has_checkbox,omitempty`},
	{"RichBlockListItem", "IsChecked", "bool", `is_checked,omitempty`},
	{"RichBlockListItem", "Value", "int", `value,omitempty`},
	{"RichBlockListItem", "Type", "string", `type,omitempty`},
	{"RichBlockParagraph", "Type", "string", `type`},
	{"RichBlockParagraph", "Text", "RichText", `text`},
	{"RichBlockSectionHeading", "Type", "string", `type`},
	{"RichBlockSectionHeading", "Text", "RichText", `text`},
	{"RichBlockSectionHeading", "Size", "int", `size`},
	{"RichBlockPreformatted", "Type", "string", `type`},
	{"RichBlockPreformatted", "Text", "RichText", `text`},
	{"RichBlockPreformatted", "Language", "string", `language,omitempty`},
	{"RichBlockFooter", "Type", "string", `type`},
	{"RichBlockFooter", "Text", "RichText", `text`},
	{"RichBlockDivider", "Type", "string", `type`},
	{"RichBlockMathematicalExpression", "Type", "string", `type`},
	{"RichBlockMathematicalExpression", "Expression", "string", `expression`},
	{"RichBlockAnchor", "Type", "string", `type`},
	{"RichBlockAnchor", "Name", "string", `name`},
	{"RichBlockList", "Type", "string", `type`},
	{"RichBlockList", "Items", "[]RichBlockListItem", `items`},
	{"RichBlockBlockQuotation", "Type", "string", `type`},
	{"RichBlockBlockQuotation", "Blocks", "RichBlocks", `blocks`},
	{"RichBlockBlockQuotation", "Credit", "*RichText", `credit,omitempty`},
	{"RichBlockPullQuotation", "Type", "string", `type`},
	{"RichBlockPullQuotation", "Text", "RichText", `text`},
	{"RichBlockPullQuotation", "Credit", "*RichText", `credit,omitempty`},
	{"RichBlockCollage", "Type", "string", `type`},
	{"RichBlockCollage", "Blocks", "RichBlocks", `blocks`},
	{"RichBlockCollage", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockSlideshow", "Type", "string", `type`},
	{"RichBlockSlideshow", "Blocks", "RichBlocks", `blocks`},
	{"RichBlockSlideshow", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockTable", "Type", "string", `type`},
	{"RichBlockTable", "Cells", "[][]RichBlockTableCell", `cells`},
	{"RichBlockTable", "IsBordered", "bool", `is_bordered,omitempty`},
	{"RichBlockTable", "IsStriped", "bool", `is_striped,omitempty`},
	{"RichBlockTable", "Caption", "*RichText", `caption,omitempty`},
	{"RichBlockDetails", "Type", "string", `type`},
	{"RichBlockDetails", "Summary", "RichText", `summary`},
	{"RichBlockDetails", "Blocks", "RichBlocks", `blocks`},
	{"RichBlockDetails", "IsOpen", "bool", `is_open,omitempty`},
	{"RichBlockMap", "Type", "string", `type`},
	{"RichBlockMap", "Location", "Location", `location`},
	{"RichBlockMap", "Zoom", "int", `zoom`},
	{"RichBlockMap", "Width", "int", `width`},
	{"RichBlockMap", "Height", "int", `height`},
	{"RichBlockMap", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockAnimation", "Type", "string", `type`},
	{"RichBlockAnimation", "Animation", "Animation", `animation`},
	{"RichBlockAnimation", "HasSpoiler", "bool", `has_spoiler,omitempty`},
	{"RichBlockAnimation", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockAudio", "Type", "string", `type`},
	{"RichBlockAudio", "Audio", "Audio", `audio`},
	{"RichBlockAudio", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockPhoto", "Type", "string", `type`},
	{"RichBlockPhoto", "Photo", "[]PhotoSize", `photo`},
	{"RichBlockPhoto", "HasSpoiler", "bool", `has_spoiler,omitempty`},
	{"RichBlockPhoto", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockVideo", "Type", "string", `type`},
	{"RichBlockVideo", "Video", "Video", `video`},
	{"RichBlockVideo", "HasSpoiler", "bool", `has_spoiler,omitempty`},
	{"RichBlockVideo", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockVoiceNote", "Type", "string", `type`},
	{"RichBlockVoiceNote", "VoiceNote", "Voice", `voice_note`},
	{"RichBlockVoiceNote", "Caption", "*RichBlockCaption", `caption,omitempty`},
	{"RichBlockThinking", "Type", "string", `type`},
	{"RichBlockThinking", "Text", "RichText", `text`},
	{"RichMessage", "Blocks", "RichBlocks", `blocks`},
	{"RichMessage", "IsRTL", "bool", `is_rtl,omitempty`},
}
