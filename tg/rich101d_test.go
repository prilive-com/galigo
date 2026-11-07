package tg_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.1 rich messages, send side (B-25 10.1d, TEST-PLAN §2, rows T-irm-1 … T-irc-2).

// rich101dAsGiven is a value each standard normaliser would change (E-0117): spaces and a tab at both ends, a
// newline, markup characters that encoding/json escapes, mixed case, "%2f", "+" and a non-ASCII rune.
const rich101dAsGiven = " **B** \t\n<b>&amp;</b> HTTPS://E.x/A%2fb?a=+ ü "

// T-irm-1
func TestAPI101D_InputRichMessageEncode(t *testing.T) {
	rows := []struct {
		name string
		in   tg.InputRichMessage
		want string
	}{
		{"zero", tg.InputRichMessage{}, `{}`},
		{"markdown", tg.InputRichMessage{Markdown: "m"}, `{"markdown":"m"}`},
		{"html", tg.InputRichMessage{HTML: "h"}, `{"html":"h"}`},
		{"is_rtl", tg.InputRichMessage{IsRTL: true}, `{"is_rtl":true}`},
		{"skip_entity_detection", tg.InputRichMessage{SkipEntityDetection: true}, `{"skip_entity_detection":true}`},
		{"all", tg.InputRichMessage{HTML: "h", Markdown: "m", IsRTL: true, SkipEntityDetection: true},
			`{"html":"h","markdown":"m","is_rtl":true,"skip_entity_detection":true}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.JSONEq(t, r.want, string(out))
		})
	}
}

// T-irm-2
func TestAPI101D_InputRichMessageAsGiven(t *testing.T) {
	out, err := json.Marshal(tg.InputRichMessage{Markdown: rich101dAsGiven, HTML: rich101dAsGiven})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, rich101dAsGiven, got["markdown"])
	assert.Equal(t, rich101dAsGiven, got["html"])
}

type rich101dField struct {
	name, typ, tag string
}

func rich101dFields(t *testing.T, v any) []rich101dField {
	t.Helper()
	rt := reflect.TypeOf(v)
	var out []rich101dField
	for i := range rt.NumField() {
		f := rt.Field(i)
		out = append(out, rich101dField{f.Name, f.Type.String(), f.Tag.Get("json")})
	}
	return out
}

// T-irm-3
func TestAPI101D_InputRichMessageShape(t *testing.T) {
	assert.Equal(t, []rich101dField{
		{"HTML", "string", "html,omitempty"},
		{"Markdown", "string", "markdown,omitempty"},
		{"IsRTL", "bool", "is_rtl,omitempty"},
		{"SkipEntityDetection", "bool", "skip_entity_detection,omitempty"},
	}, rich101dFields(t, tg.InputRichMessage{}))
}

// T-irc-1
func TestAPI101D_InputRichMessageContent(t *testing.T) {
	out, err := json.Marshal(tg.InputRichMessageContent{RichMessage: tg.InputRichMessage{Markdown: "m"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"rich_message":{"markdown":"m"}}`, string(out))

	out, err = json.Marshal(tg.InputRichMessageContent{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"rich_message":{}}`, string(out), "the required key is kept")

	assert.Equal(t, []rich101dField{{"RichMessage", "tg.InputRichMessage", "rich_message"}},
		rich101dFields(t, tg.InputRichMessageContent{}))
}

// T-irc-2: the content in each tg result type that holds an InputMessageContent.
func TestAPI101D_InputRichMessageContentInResults(t *testing.T) {
	var c tg.InputMessageContent = tg.InputRichMessageContent{RichMessage: tg.InputRichMessage{HTML: "<p>x</p>"}}
	_, open := any(struct{}{}).(tg.InputMessageContent)
	assert.False(t, open, "InputMessageContent stays a closed union (C1-03)")
	results := map[string]any{
		"article":  &tg.InlineQueryResultArticle{Type: "article", ID: "1", Title: "t", InputMessageContent: c},
		"photo":    &tg.InlineQueryResultPhoto{Type: "photo", ID: "2", InputMessageContent: c},
		"document": &tg.InlineQueryResultDocument{Type: "document", ID: "3", InputMessageContent: c},
	}
	for name, r := range results {
		t.Run(name, func(t *testing.T) {
			out, err := json.Marshal(r)
			require.NoError(t, err)
			var got struct {
				InputMessageContent json.RawMessage `json:"input_message_content"`
			}
			require.NoError(t, json.Unmarshal(out, &got))
			assert.JSONEq(t, `{"rich_message":{"html":"<p>x</p>"}}`, string(got.InputMessageContent))
		})
	}
}
