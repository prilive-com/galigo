package tg_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-65 65g: Bot API 10.4 rich quotes (TextQuote.rich_texts, ReplyParameters.quote_rich_texts), the ton_address text
// kind, and Message.paid_messages_waived. TEST-PLAN section 2. The quote fixtures are galigo's re-encodes of what the
// live probe got back (galigo-b65g-rich-quotes-2026-11-22/probes/q1), not server bytes. Ids are fake.

// Position 18 and the fixture without the text key are derived fixtures (the probe sent 19 and re-encoded its replies,
// PLAN section 3).
const (
	b65gQuotePlain = `{"text":"","rich_texts":["quote"],"position":7,"is_manual":true}`
	b65gQuoteBold  = `{"text":"","rich_texts":[["quote ",{"type":"bold","text":"bold"}]],"position":7,"is_manual":true}`
	b65gQuoteTwo   = `{"text":"","rich_texts":["end","second"],"position":18,"is_manual":true}`
	b65gQuoteNoTxt = `{"rich_texts":[["quote ",{"type":"bold","text":"bold"}]],"position":7,"is_manual":true}`
)

func b65gBoldTexts() []tg.RichText {
	return []tg.RichText{{Parts: []tg.RichText{{Plain: "quote "}, {Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "bold"}}}}}}
}

func b65gMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// T-quote-decode: the three probe values decode to the whole TextQuote and re-encode to the same bytes.
func TestB65G_QuoteDecode(t *testing.T) {
	for _, tc := range []struct {
		name, in, out string
		want          tg.TextQuote
	}{
		{"plain", b65gQuotePlain, b65gQuotePlain, tg.TextQuote{RichTexts: []tg.RichText{{Plain: "quote"}}, Position: 7, IsManual: true}},
		{"bold", b65gQuoteBold, b65gQuoteBold, tg.TextQuote{RichTexts: b65gBoldTexts(), Position: 7, IsManual: true}},
		{"two", b65gQuoteTwo, b65gQuoteTwo, tg.TextQuote{RichTexts: []tg.RichText{{Plain: "end"}, {Plain: "second"}}, Position: 18, IsManual: true}},
		{"no_text_key", b65gQuoteNoTxt, b65gQuoteBold, tg.TextQuote{RichTexts: b65gBoldTexts(), Position: 7, IsManual: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q tg.TextQuote
			require.NoError(t, api96Strict(t, []byte(tc.in), &q))
			assert.Equal(t, tc.want, q)
			assert.Equal(t, tc.out, b65gMarshal(t, q))
		})
	}
}

// T-quote-msg: the quote of a reply in a whole Message.
func TestB65G_QuoteMessage(t *testing.T) {
	in := `{"message_id":7,"date":1,"chat":{"id":-1009876543210,"type":"supergroup","title":"G"},` +
		`"reply_to_message":{"message_id":1,"date":1,"chat":{"id":-1009876543210,"type":"supergroup","title":"G"}},"quote":` + b65gQuoteBold + `}`
	var m tg.Message
	require.NoError(t, api96Strict(t, []byte(in), &m))
	require.NotNil(t, m.Quote)
	assert.Equal(t, tg.TextQuote{RichTexts: b65gBoldTexts(), Position: 7, IsManual: true}, *m.Quote)
	require.NotNil(t, m.ReplyToMessage)
	assert.Equal(t, 1, m.ReplyToMessage.MessageID)
}

// T-quote-elem (E-0120): element-level decode keeps each element in place.
func TestB65G_QuoteElements(t *testing.T) {
	for _, tc := range []struct {
		name, in, out string
		want          []tg.RichText
		nilSlice      bool
	}{
		{"null_element", `["a",null,"b"]`, `{"text":"","rich_texts":["a","","b"],"position":0}`, []tg.RichText{{Plain: "a"}, {}, {Plain: "b"}}, false},
		{"unknown_kind", `[{"type":"z_future","k":1}]`, `{"text":"","rich_texts":[{"type":"z_future","k":1}],"position":0}`,
			[]tg.RichText{{Node: tg.RichTextUnknown{Type: "z_future", Raw: json.RawMessage(`{"type":"z_future","k":1}`)}}}, false},
		{"empty", `[]`, `{"text":"","position":0}`, []tg.RichText{}, false},
		{"null", `null`, `{"text":"","position":0}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tg.TextQuote{RichTexts: []tg.RichText{{Plain: "seed"}}} // a decode that skips the key keeps the seed
			require.NoError(t, json.Unmarshal([]byte(`{"text":"","position":0,"rich_texts":`+tc.in+`}`), &q))
			assert.Equal(t, tc.want, q.RichTexts)
			assert.Equal(t, tc.nilSlice, q.RichTexts == nil)
			assert.Equal(t, tc.out, b65gMarshal(t, q))
		})
	}
}

// T-quote-bad: a non-null value that is not an array fails the decode (PLAN section 4, the documented strict rule).
func TestB65G_QuoteBad(t *testing.T) {
	for _, v := range []string{`"x"`, `{}`, `1`, `true`} {
		t.Run("quote "+v, func(t *testing.T) {
			var q tg.TextQuote
			require.Error(t, json.Unmarshal([]byte(`{"text":"","position":0,"rich_texts":`+v+`}`), &q))
		})
		t.Run("message "+v, func(t *testing.T) {
			var m tg.Message
			err := json.Unmarshal([]byte(`{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"quote":{"text":"","position":0,"rich_texts":`+v+`}}`), &m)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "rich_texts")
		})
	}
}

// T-rp-encode: ReplyParameters.QuoteRichTexts on the send side.
func TestB65G_ReplyParametersEncode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		texts []tg.RichText
		want  string
	}{
		{"bold", b65gBoldTexts(), `{"message_id":1,"quote_rich_texts":[["quote ",{"type":"bold","text":"bold"}]],"quote_position":7}`},
		{"empty", []tg.RichText{}, `{"message_id":1,"quote_position":7}`},
		{"nil", nil, `{"message_id":1,"quote_position":7}`},
		{"zero_element", []tg.RichText{{}}, `{"message_id":1,"quote_rich_texts":[""],"quote_position":7}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, b65gMarshal(t, tg.ReplyParameters{MessageID: 1, QuoteRichTexts: tc.texts, QuotePosition: 7}))
		})
	}
}

// T-ton: the ton_address kind decodes to its type (not RichTextUnknown), in a RichText and in a whole Message.
// (The plain ton_address decode and the zero encode are rich101c census rows: TestAPI101C_*.)
func TestB65G_TonAddress(t *testing.T) {
	want := tg.RichTextTonAddress{Type: "ton_address", Text: tg.RichText{Plain: "t"}, Address: "EQfake1"}
	t.Run("message_nested_text", func(t *testing.T) { // X-T1: Text is a RichText, so an array with a node must survive
		node := `{"type":"ton_address","text":["a ",{"type":"bold","text":"b"}],"address":"EQfake1"}`
		in := `{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},"rich_message":{"blocks":[{"type":"paragraph","text":[` + node + `]}]}}`
		var m tg.Message
		require.NoError(t, json.Unmarshal([]byte(in), &m))
		require.NotNil(t, m.RichMessage)
		require.Len(t, m.RichMessage.Blocks, 1)
		nested := tg.RichTextTonAddress{Type: "ton_address", Address: "EQfake1",
			Text: tg.RichText{Parts: []tg.RichText{{Plain: "a "}, {Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "b"}}}}}}
		assert.Equal(t, tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Parts: []tg.RichText{{Node: nested}}}}, m.RichMessage.Blocks[0])
		assert.Equal(t, `[`+node+`]`, b65gMarshal(t, m.RichMessage.Blocks[0].(tg.RichBlockParagraph).Text))
	})
	t.Run("extra_key_dropped", func(t *testing.T) { // PLAN section 9: the typed node keeps only its fields
		var r tg.RichText
		require.NoError(t, json.Unmarshal([]byte(`{"type":"ton_address","text":"t","address":"EQfake1","future_key":1}`), &r))
		assert.Equal(t, tg.RichText{Node: want}, r)
		assert.Equal(t, `{"type":"ton_address","text":"t","address":"EQfake1"}`, b65gMarshal(t, r))
	})
}

// T-waived: Message.paid_messages_waived.
func TestB65G_PaidMessagesWaived(t *testing.T) {
	msg := func(v string) string {
		return `{"message_id":7,"date":1,"chat":{"id":1,"type":"private"}` + v + `}`
	}
	for _, tc := range []struct {
		name, in string
		set      bool
	}{
		{"empty_object", `,"paid_messages_waived":{}`, true},
		{"unknown_key", `,"paid_messages_waived":{"future":1}`, true},
		{"null", `,"paid_messages_waived":null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m tg.Message // the positive rows start at nil, so a decode that skips the key fails them
			if !tc.set {
				m.PaidMessagesWaived = &tg.PaidMessagesWaived{} // seed: null must clear it
			}
			require.NoError(t, json.Unmarshal([]byte(msg(tc.in)), &m))
			if tc.set {
				assert.Equal(t, &tg.PaidMessagesWaived{}, m.PaidMessagesWaived)
			} else {
				assert.Nil(t, m.PaidMessagesWaived)
			}
		})
	}
	for _, v := range []string{`[]`, `5`, `"s"`, `true`} {
		t.Run("bad "+v, func(t *testing.T) {
			var m tg.Message
			require.Error(t, json.Unmarshal([]byte(msg(`,"paid_messages_waived":`+v)), &m))
		})
	}
	t.Run("encode", func(t *testing.T) {
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(msg(`,"paid_messages_waived":{}`)), &m))
		assert.Equal(t, msg(`,"paid_messages_waived":{}`), b65gMarshal(t, m))
	})
}

type b65gField struct {
	Name, Type, Tag string
	Anonymous       bool
}

func b65gFields(v any) []b65gField {
	rt := reflect.TypeOf(v)
	var out []b65gField
	for i := range rt.NumField() {
		f := rt.Field(i)
		out = append(out, b65gField{f.Name, f.Type.String(), f.Tag.Get("json"), f.Anonymous})
	}
	return out
}

// b65gAround returns the field before, the field itself, and the field after name in v's type.
func b65gAround(t *testing.T, v any, name string) []b65gField {
	t.Helper()
	all := b65gFields(v)
	for i, f := range all {
		if f.Name == name {
			require.Positive(t, i)
			require.Less(t, i+1, len(all))
			return all[i-1 : i+2]
		}
	}
	require.Failf(t, "no field", "%T has no field %s", v, name)
	return nil
}

// T-shapes (E-0126): the new types and holder fields, with their neighbours.
func TestB65G_Shapes(t *testing.T) {
	var _ tg.RichTextNode = tg.RichTextTonAddress{}
	t.Run("RichTextTonAddress", func(t *testing.T) {
		assert.Equal(t, []b65gField{{"Type", "string", "type", false}, {"Text", "tg.RichText", "text", false}, {"Address", "string", "address", false}},
			b65gFields(tg.RichTextTonAddress{}))
		assert.Equal(t, 0, reflect.TypeFor[tg.RichTextTonAddress]().NumMethod(), "no exported method (the marker is unexported)")
		assert.Equal(t, 0, reflect.TypeFor[*tg.RichTextTonAddress]().NumMethod(), "no exported pointer method")
	})
	t.Run("PaidMessagesWaived", func(t *testing.T) {
		assert.Empty(t, b65gFields(tg.PaidMessagesWaived{}))
		assert.Equal(t, 0, reflect.TypeFor[tg.PaidMessagesWaived]().NumMethod())
		assert.Equal(t, 0, reflect.TypeFor[*tg.PaidMessagesWaived]().NumMethod())
	})
	t.Run("holder_TextQuote.RichTexts", func(t *testing.T) {
		assert.Equal(t, []b65gField{{"Entities", "[]tg.MessageEntity", "entities,omitempty", false},
			{"RichTexts", "[]tg.RichText", "rich_texts,omitempty", false}, {"Position", "int", "position", false}},
			b65gAround(t, tg.TextQuote{}, "RichTexts"))
	})
	t.Run("holder_ReplyParameters.QuoteRichTexts", func(t *testing.T) {
		assert.Equal(t, []b65gField{{"QuoteEntities", "[]tg.MessageEntity", "quote_entities,omitempty", false},
			{"QuoteRichTexts", "[]tg.RichText", "quote_rich_texts,omitempty", false}, {"QuotePosition", "int", "quote_position,omitempty", false}},
			b65gAround(t, tg.ReplyParameters{}, "QuoteRichTexts"))
	})
	t.Run("holder_Message.PaidMessagesWaived", func(t *testing.T) {
		assert.Equal(t, []b65gField{{"ManagedBotCreated", "*tg.ManagedBotCreated", "managed_bot_created,omitempty", false},
			{"PaidMessagesWaived", "*tg.PaidMessagesWaived", "paid_messages_waived,omitempty", false},
			{"PollOptionAdded", "*tg.PollOptionAdded", "poll_option_added,omitempty", false}},
			b65gAround(t, tg.Message{}, "PaidMessagesWaived"))
	})
}

// T-old-zero: a zero Message encodes as at base a01e095 (no new key leaks; base bytes measured, E65g
// trials/old-zero-base.txt). The TextQuote and ReplyParameters zero bytes are pinned by existing tests
// (tg/reply_b48a_test.go, tg/api102b_test.go).
func TestB65G_OldZero(t *testing.T) {
	assert.Equal(t, `{"message_id":0,"date":0,"chat":null}`, b65gMarshal(t, tg.Message{}))
}
