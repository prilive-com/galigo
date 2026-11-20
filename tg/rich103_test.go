package tg_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 rich message changes (B-65 65c, TEST-PLAN §2): buttons, button nodes, compact tables, expandable
// quotations and document blocks. The decode inputs are the server's replies to this slice's probes, with fake ids.

func rt103(s string) tg.RichText { return tg.RichText{Plain: s} }

func rtp103(s string) *tg.RichText { r := rt103(s); return &r }

func sp103(s string) *string { return &s }

// rich103Blocks decodes a rich_message JSON value inside a Message, the way the server sends it.
func rich103Blocks(t *testing.T, rm string) tg.RichBlocks {
	t.Helper()
	var msg tg.Message
	require.NoError(t, json.Unmarshal([]byte(`{"message_id":1,"rich_message":`+rm+`}`), &msg))
	require.NotNil(t, msg.RichMessage)
	return msg.RichMessage.Blocks
}

const (
	rich103Probe02 = `{"blocks":[{"type":"paragraph","text":"galigo 65c buttons"},{"type":"buttons","buttons":[{"text":"cb","style":"primary","callback_data":"b65c"},{"text":"url","url":"https://t.me/"},{"text":"copy","copy_text":{"text":"galigo"}},{"text":"off","disabled":{}}],"align":"center"},{"type":"buttons","buttons":[{"text":"link","callback_data":"b65c2"}]}]}`
	rich103Probe03 = `{"blocks":[{"type":"paragraph","text":["galigo 65c text button ",{"type":"button","button":{"text":"cb","style":"primary","callback_data":"b65c"}}]}]}`
	rich103Probe04 = `{"blocks":[{"type":"table","cells":[[{"text":"a","align":"left","valign":"top"},{"text":"b","align":"left","valign":"top"}]],"is_striped":true,"is_compact":true}]}`
	rich103Probe05 = `{"blocks":[{"type":"expandable_blockquote","text":"galigo 65c expandable","credit":"credit"}]}`
	rich103Probe06 = `{"blocks":[{"type":"paragraph","text":["galigo 65c html ",{"type":"button","button":{"text":"Copy","copy_text":{"text":"x"}}}]},{"type":"buttons","buttons":[{"text":"Off","disabled":{}},{"text":"url","url":"https://t.me/"}],"align":"right"}]}`
	rich103Probe07 = `{"blocks":[{"type":"paragraph","text":"galigo 65c document"},{"type":"document","document":{"file_name":"f0.txt","mime_type":"text/plain","file_id":"BQAC-fake-doc","file_unique_id":"AgAD-fake","file_size":26}}]}`
	rich103Probe08 = `{"blocks":[{"type":"document","document":{"file_name":"f0.txt","mime_type":"text/plain","file_id":"BQAC-fake-doc","file_unique_id":"AgAD-fake","file_size":26},"caption":{"text":"galigo 65c doc caption"}}]}`
	rich103Raw2_02 = `{"blocks":[{"type":"buttons","buttons":[{"text":"s","switch_inline_query":""},{"text":"c","switch_inline_query_current_chat":""}]}]}`
	rich103Raw2_03 = `{"blocks":[{"type":"buttons","buttons":[{"text":"h","switch_inline_query_chosen_chat":{"query":"","allow_user_chats":true,"allow_bot_chats":false,"allow_group_chats":false,"allow_channel_chats":false}}]}]}`
)

var rich103Doc = tg.Document{FileID: "BQAC-fake-doc", FileUniqueID: "AgAD-fake", FileName: "f0.txt", MimeType: "text/plain", FileSize: 26}

// T-dec-buttons, T-dec-switch, T-dec-kinds-spec, T-dec-textbutton, T-dec-table, T-dec-expand, T-dec-document: each
// reply decodes into exactly this value (whole value, E-0176).
func TestB65C_DecodeProbes(t *testing.T) {
	rows := []struct {
		name string
		in   string
		want tg.RichBlocks
	}{
		{"buttons/p02", rich103Probe02, tg.RichBlocks{
			tg.RichBlockParagraph{Type: "paragraph", Text: rt103("galigo 65c buttons")},
			tg.RichBlockButtons{Type: "buttons", Align: "center", Buttons: []tg.RichMessageButton{
				{Text: rt103("cb"), Style: tg.ButtonStylePrimary, CallbackData: "b65c"},
				{Text: rt103("url"), URL: "https://t.me/"},
				{Text: rt103("copy"), CopyText: &tg.CopyTextButton{Text: "galigo"}},
				{Text: rt103("off"), Disabled: &tg.DisabledButton{}},
			}},
			tg.RichBlockButtons{Type: "buttons", Buttons: []tg.RichMessageButton{{Text: rt103("link"), CallbackData: "b65c2"}}},
		}},
		{"switch/raw2-02", rich103Raw2_02, tg.RichBlocks{
			tg.RichBlockButtons{Type: "buttons", Buttons: []tg.RichMessageButton{
				{Text: rt103("s"), SwitchInlineQuery: sp103("")},
				{Text: rt103("c"), SwitchInlineQueryCurrentChat: sp103("")},
			}},
		}},
		{"switch/raw2-03", rich103Raw2_03, tg.RichBlocks{
			tg.RichBlockButtons{Type: "buttons", Buttons: []tg.RichMessageButton{
				{Text: rt103("h"), SwitchInlineQueryChosenChat: &tg.SwitchInlineQueryChosenChat{AllowUserChats: true}},
			}},
		}},
		// Spec shape: no probe sent these button kinds or a button text with nodes (lookups/spec-65c.txt).
		{"kinds-spec", `{"blocks":[{"type":"buttons","buttons":[` +
			`{"text":"w","web_app":{"url":"https://e.example/w"}},` +
			`{"text":"l","login_url":{"url":"https://e.example/l","forward_text":"f"}},` +
			`{"text":["a",{"type":"date_time","text":"d","unix_time":1,"date_time_format":"f"}],"callback_data":"x"}]}]}`,
			tg.RichBlocks{tg.RichBlockButtons{Type: "buttons", Buttons: []tg.RichMessageButton{
				{Text: rt103("w"), WebApp: &tg.WebAppInfo{URL: "https://e.example/w"}},
				{Text: rt103("l"), LoginURL: &tg.LoginURL{URL: "https://e.example/l", ForwardText: "f"}},
				{Text: tg.RichText{Parts: []tg.RichText{rt103("a"), {Node: tg.RichTextDateTime{
					Type: "date_time", Text: rt103("d"), UnixTime: 1, DateTimeFormat: "f"}}}}, CallbackData: "x"},
			}}}},
		{"textbutton/p03", rich103Probe03, tg.RichBlocks{
			tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Parts: []tg.RichText{
				rt103("galigo 65c text button "),
				{Node: tg.RichTextButton{Type: "button", Button: tg.RichMessageButton{
					Text: rt103("cb"), Style: tg.ButtonStylePrimary, CallbackData: "b65c"}}},
			}}},
		}},
		{"textbutton/p06", rich103Probe06, tg.RichBlocks{
			tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Parts: []tg.RichText{
				rt103("galigo 65c html "),
				{Node: tg.RichTextButton{Type: "button", Button: tg.RichMessageButton{
					Text: rt103("Copy"), CopyText: &tg.CopyTextButton{Text: "x"}}}},
			}}},
			tg.RichBlockButtons{Type: "buttons", Align: "right", Buttons: []tg.RichMessageButton{
				{Text: rt103("Off"), Disabled: &tg.DisabledButton{}},
				{Text: rt103("url"), URL: "https://t.me/"},
			}},
		}},
		{"table/p04", rich103Probe04, tg.RichBlocks{
			tg.RichBlockTable{Type: "table", Cells: [][]tg.RichBlockTableCell{{
				{Text: rtp103("a"), Align: "left", VAlign: "top"},
				{Text: rtp103("b"), Align: "left", VAlign: "top"},
			}}, IsStriped: true, IsCompact: true},
		}},
		{"expand/p05", rich103Probe05, tg.RichBlocks{
			tg.RichBlockExpandableBlockQuotation{Type: "expandable_blockquote", Text: rt103("galigo 65c expandable"),
				Credit: rtp103("credit")},
		}},
		{"document/p08", rich103Probe08, tg.RichBlocks{
			tg.RichBlockDocument{Type: "document", Document: rich103Doc,
				Caption: &tg.RichBlockCaption{Text: rt103("galigo 65c doc caption")}},
		}},
		{"document/p07", rich103Probe07, tg.RichBlocks{
			tg.RichBlockParagraph{Type: "paragraph", Text: rt103("galigo 65c document")},
			tg.RichBlockDocument{Type: "document", Document: rich103Doc},
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			assert.Equal(t, r.want, rich103Blocks(t, r.in))
		})
	}
}

// T-dec-nested: the new kinds decode at depth too.
func TestB65C_DecodeNested(t *testing.T) {
	const (
		doc  = `{"type":"document","document":{"file_id":"BQAC-fake-doc","file_unique_id":"AgAD-fake"}}`
		node = `{"type":"button","button":{"text":"n","callback_data":"x"}}`
		row  = `{"type":"buttons","buttons":[{"text":"k","url":"https://t.me/"}]}`
	)
	in := `{"blocks":[{"type":"collage","blocks":[` + doc + `]},` +
		`{"type":"list","items":[{"label":"1","blocks":[` + doc + `,{"type":"paragraph","text":{"type":"bold","text":` + node + `}}]}]},` +
		`{"type":"details","summary":"s","blocks":[` + row + `]}]}`
	docBlock := tg.RichBlockDocument{Type: "document", Document: tg.Document{FileID: "BQAC-fake-doc", FileUniqueID: "AgAD-fake"}}
	want := tg.RichBlocks{
		tg.RichBlockCollage{Type: "collage", Blocks: tg.RichBlocks{docBlock}},
		tg.RichBlockList{Type: "list", Items: []tg.RichBlockListItem{{Label: "1", Blocks: tg.RichBlocks{
			docBlock,
			tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Node: tg.RichTextBold{Type: "bold",
				Text: tg.RichText{Node: tg.RichTextButton{Type: "button",
					Button: tg.RichMessageButton{Text: rt103("n"), CallbackData: "x"}}}}}},
		}}}},
		tg.RichBlockDetails{Type: "details", Summary: rt103("s"), Blocks: tg.RichBlocks{
			tg.RichBlockButtons{Type: "buttons", Buttons: []tg.RichMessageButton{{Text: rt103("k"), URL: "https://t.me/"}}},
		}},
	}
	assert.Equal(t, want, rich103Blocks(t, in))
}

// T-dec-old-inputs: the unknown-kind inputs the older tests used now decode into their 10.3 kind (PLAN §9).
func TestB65C_DecodeOldInputs(t *testing.T) {
	assert.Equal(t, tg.RichBlocks{
		tg.RichBlockExpandableBlockQuotation{Type: "expandable_blockquote"},
		tg.RichBlockDocument{Type: "document"},
	}, rich103Blocks(t, `{"blocks":[{"type":"expandable_blockquote","blocks":[]},{"type":"document"}]}`))
	var rt tg.RichText
	require.NoError(t, json.Unmarshal([]byte(`{"type":"button","text":"b","button":{}}`), &rt))
	assert.Equal(t, tg.RichText{Node: tg.RichTextButton{Type: "button"}}, rt)
}

// T-dec-wrong-nonrich: a field that is not RichText and has the wrong JSON type makes the whole block (or node)
// unknown, with its bytes kept.
func TestB65C_DecodeWrongNonRich(t *testing.T) {
	blocks := []string{
		`{"type":"buttons","buttons":{}}`,
		`{"type":"buttons","buttons":[5]}`,
		`{"type":"buttons","buttons":[],"align":1}`,
		`{"type":"document","document":"x"}`,
		`{"type":"document","document":{},"caption":5}`,
		`{"type":"table","cells":[],"is_compact":"yes"}`,
	}
	for _, f := range []string{`"disabled":true`, `"copy_text":"x"`, `"web_app":1`, `"login_url":[]`,
		`"switch_inline_query":5`, `"switch_inline_query_current_chat":false`, `"switch_inline_query_chosen_chat":"x"`,
		`"url":1`, `"callback_data":1`, `"style":1`} {
		blocks = append(blocks, `{"type":"buttons","buttons":[{"text":"t",`+f+`}]}`)
	}
	for _, in := range blocks {
		t.Run(in, func(t *testing.T) {
			var probe struct{ Type string }
			require.NoError(t, json.Unmarshal([]byte(in), &probe))
			got := rich103Blocks(t, `{"blocks":[`+in+`]}`)
			assert.Equal(t, tg.RichBlocks{tg.RichBlockUnknown{Type: probe.Type, Raw: json.RawMessage(in)}}, got)
			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.Equal(t, `[`+in+`]`, string(out))
		})
	}
	for _, in := range []string{`{"type":"button","button":[]}`, `{"type":"button","button":5}`} {
		t.Run(in, func(t *testing.T) {
			var rt tg.RichText
			require.NoError(t, json.Unmarshal([]byte(in), &rt))
			assert.Equal(t, tg.RichText{Node: tg.RichTextUnknown{Type: "button", Raw: json.RawMessage(in)}}, rt)
			out, err := json.Marshal(rt)
			require.NoError(t, err)
			assert.Equal(t, in, string(out))
		})
	}
}

// T-dec-wrong-rich: a RichText field never fails, so the kind decodes and the field holds unknowns.
func TestB65C_DecodeWrongRich(t *testing.T) {
	got := rich103Blocks(t, `{"blocks":[{"type":"expandable_blockquote","text":5,"credit":[1,{"x":1}]},`+
		`{"type":"buttons","buttons":[{"text":5,"url":"u"}]}]}`)
	assert.Equal(t, tg.RichBlocks{
		tg.RichBlockExpandableBlockQuotation{Type: "expandable_blockquote",
			Text: tg.RichText{Node: tg.RichTextUnknown{Raw: json.RawMessage(`5`)}},
			Credit: &tg.RichText{Parts: []tg.RichText{
				{Node: tg.RichTextUnknown{Raw: json.RawMessage(`1`)}},
				{Node: tg.RichTextUnknown{Raw: json.RawMessage(`{"x":1}`)}},
			}}},
		tg.RichBlockButtons{Type: "buttons", Buttons: []tg.RichMessageButton{
			{Text: tg.RichText{Node: tg.RichTextUnknown{Raw: json.RawMessage(`5`)}}, URL: "u"},
		}},
	}, got)
}

// T-reencode: galigo's own encoding of a decoded reply, for the two replies where it differs from the server's bytes
// (probe 08: Document field order; raw2/03: the false bools are dropped by SwitchInlineQueryChosenChat's tags).
func TestB65C_Reencode(t *testing.T) {
	rows := []struct{ name, in, want string }{
		{"p08", rich103Probe08, `{"blocks":[{"type":"document","document":{"file_id":"BQAC-fake-doc","file_unique_id":"AgAD-fake","file_name":"f0.txt","mime_type":"text/plain","file_size":26},"caption":{"text":"galigo 65c doc caption"}}]}`},
		{"raw2-03", rich103Raw2_03, `{"blocks":[{"type":"buttons","buttons":[{"text":"h","switch_inline_query_chosen_chat":{"allow_user_chats":true}}]}]}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			require.NotEqual(t, r.in, r.want)
			out, err := json.Marshal(tg.RichMessage{Blocks: rich103Blocks(t, r.in)})
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
}

// T-enc-buttons: the buttons input block with every button kind.
func TestB65C_EncodeButtons(t *testing.T) {
	buttons := []tg.RichMessageButton{
		{Text: rt103("cb"), Style: tg.ButtonStylePrimary, CallbackData: "c"},
		{Text: rt103("u"), URL: "https://t.me/"},
		{Text: rt103("w"), WebApp: &tg.WebAppInfo{URL: "https://e.example/w"}},
		{Text: rt103("l"), LoginURL: &tg.LoginURL{URL: "https://e.example/l"}},
		{Text: rt103("s"), SwitchInlineQuery: sp103("")},
		{Text: rt103("sc"), SwitchInlineQueryCurrentChat: sp103("q")},
		{Text: rt103("ch"), SwitchInlineQueryChosenChat: &tg.SwitchInlineQueryChosenChat{Query: "q", AllowBotChats: true}},
		{Text: rt103("cp"), CopyText: &tg.CopyTextButton{Text: "galigo"}},
		{Text: rt103("off"), Disabled: &tg.DisabledButton{}},
	}
	const list = `[{"text":"cb","style":"primary","callback_data":"c"},{"text":"u","url":"https://t.me/"},` +
		`{"text":"w","web_app":{"url":"https://e.example/w"}},{"text":"l","login_url":{"url":"https://e.example/l"}},` +
		`{"text":"s","switch_inline_query":""},{"text":"sc","switch_inline_query_current_chat":"q"},` +
		`{"text":"ch","switch_inline_query_chosen_chat":{"query":"q","allow_bot_chats":true}},` +
		`{"text":"cp","copy_text":{"text":"galigo"}},{"text":"off","disabled":{}}]`
	out, err := json.Marshal(tg.InputRichBlockButtons{Buttons: buttons, Align: "center"})
	require.NoError(t, err)
	assert.Equal(t, `{"type":"buttons","buttons":`+list+`,"align":"center"}`, string(out))
	out, err = json.Marshal(tg.InputRichBlockButtons{Buttons: buttons[:1]})
	require.NoError(t, err)
	assert.Equal(t, `{"type":"buttons","buttons":[{"text":"cb","style":"primary","callback_data":"c"}]}`, string(out))
}

// T-enc-button-fields: each RichMessageButton field, set alone beside the text, adds exactly its spec key (E-0132).
func TestB65C_ButtonFields(t *testing.T) {
	keys := []string{"text", "style", "url", "callback_data", "web_app", "login_url", "switch_inline_query",
		"switch_inline_query_current_chat", "switch_inline_query_chosen_chat", "copy_text", "disabled"} // spec order
	typ := reflect.TypeFor[tg.RichMessageButton]()
	require.Equal(t, len(keys), typ.NumField(), "a RichMessageButton field without a row")
	out, err := json.Marshal(tg.RichMessageButton{})
	require.NoError(t, err)
	assert.Equal(t, `{"text":""}`, string(out))
	for i, key := range keys {
		t.Run(key, func(t *testing.T) {
			b := tg.RichMessageButton{Text: rt103("t")}
			f := reflect.ValueOf(&b).Elem().Field(i)
			switch f.Kind() {
			case reflect.String:
				f.SetString("x")
			case reflect.Pointer:
				f.Set(reflect.New(f.Type().Elem()))
			case reflect.Struct: // Text, already set
			default:
				t.Fatalf("field %s: kind %s has no row", typ.Field(i).Name, f.Kind())
			}
			out, err := json.Marshal(b)
			require.NoError(t, err)
			var m map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(out, &m))
			want := []string{"text"}
			if key != "text" {
				want = append(want, key)
			}
			got := make([]string, 0, len(m))
			for k := range m {
				got = append(got, k)
			}
			slices.Sort(got)
			slices.Sort(want)
			assert.Equal(t, want, got, string(out))
		})
	}
}

// T-enc-switch: the empty switch queries and the disabled marker are sent; nil pointers are not.
func TestB65C_EncodeSwitch(t *testing.T) {
	out, err := json.Marshal(tg.RichMessageButton{Text: rt103("t"), SwitchInlineQuery: sp103(""),
		SwitchInlineQueryCurrentChat: sp103(""), Disabled: &tg.DisabledButton{}})
	require.NoError(t, err)
	assert.Equal(t, `{"text":"t","switch_inline_query":"","switch_inline_query_current_chat":"","disabled":{}}`, string(out))
	out, err = json.Marshal(tg.RichMessageButton{Text: rt103("t")})
	require.NoError(t, err)
	assert.Equal(t, `{"text":"t"}`, string(out))
}

// T-enc-expand, T-enc-document, T-enc-table, T-enc-textbutton.
func TestB65C_EncodeBlocks(t *testing.T) {
	rows := []struct {
		name string
		in   any
		want string
	}{
		{"expand", tg.InputRichBlockExpandableBlockQuotation{Text: rt103("q")}, `{"type":"expandable_blockquote","text":"q"}`},
		{"expand/credit", tg.InputRichBlockExpandableBlockQuotation{Text: rt103("q"), Credit: rtp103("c")},
			`{"type":"expandable_blockquote","text":"q","credit":"c"}`},
		{"document", tg.InputRichBlockDocument{Document: fake102e{Type: "document", Media: "BQAC-fake-doc"},
			Caption: &tg.RichBlockCaption{Text: rt103("c")}},
			`{"type":"document","document":{"type":"document","media":"BQAC-fake-doc"},"caption":{"text":"c"}}`},
		{"document/min", tg.InputRichBlockDocument{Document: fake102e{Type: "document", Media: "BQAC-fake-doc"}},
			`{"type":"document","document":{"type":"document","media":"BQAC-fake-doc"}}`},
		{"table/compact", tg.InputRichBlockTable{Cells: [][]tg.RichBlockTableCell{}, IsStriped: true, IsCompact: true},
			`{"type":"table","cells":[],"is_striped":true,"is_compact":true}`},
		{"table/not-compact", tg.InputRichBlockTable{Cells: [][]tg.RichBlockTableCell{}}, `{"type":"table","cells":[]}`},
		{"textbutton", tg.InputRichBlockParagraph{Text: tg.RichText{Parts: []tg.RichText{rt103("a"),
			{Node: tg.RichTextButton{Type: "button", Button: tg.RichMessageButton{Text: rt103("b"), CallbackData: "c"}}}}}},
			`{"type":"paragraph","text":["a",{"type":"button","button":{"text":"b","callback_data":"c"}}]}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
}

// T-kbd-shapes: the two keyboard types RichMessageButton needs (A-P2-02, E-0126).
func TestB65C_KeyboardShapes(t *testing.T) {
	ct := reflect.TypeFor[tg.CopyTextButton]()
	require.Equal(t, 1, ct.NumField())
	f := ct.Field(0)
	assert.Equal(t, []any{"Text", "string", `json:"text"`, false},
		[]any{f.Name, f.Type.String(), string(f.Tag), f.Anonymous})
	assert.Equal(t, 0, reflect.TypeFor[tg.DisabledButton]().NumField())
}
