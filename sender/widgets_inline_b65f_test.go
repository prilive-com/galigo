package sender_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// C3 (B-65 65f): each 10.4 inline content goes in input_message_content of each existing result that has one.
func TestB65F_C3_InlineRequest(t *testing.T) {
	contents := []struct {
		name string
		c    tg.InputMessageContent
		want string
	}{
		{"widget", tg.InputWidgetMessageContent{HTML: "<b>w</b>", Width: 300, PublicData: "pub-1"},
			`{"html":"<b>w</b>","width":300,"public_data":"pub-1"}`},
		{"shared", tg.InputSharedWidgetMessageContent{WidgetID: "wid-4", ChatData: "chat-2"},
			`{"widget_id":"wid-4","chat_data":"chat-2"}`},
	}
	results := []struct {
		name string
		make func(tg.InputMessageContent) tg.InlineQueryResult
	}{
		{"article", func(c tg.InputMessageContent) tg.InlineQueryResult {
			return tg.InlineQueryResultArticle{ID: "1", Title: "t", InputMessageContent: c}
		}},
		{"photo", func(c tg.InputMessageContent) tg.InlineQueryResult {
			return tg.InlineQueryResultPhoto{ID: "1", PhotoURL: "https://e.invalid/p.jpg", ThumbnailURL: "https://e.invalid/t.jpg", InputMessageContent: c}
		}},
		{"document", func(c tg.InputMessageContent) tg.InlineQueryResult {
			return tg.InlineQueryResultDocument{ID: "1", Title: "t", DocumentURL: "https://e.invalid/d.pdf", MimeType: "application/pdf", InputMessageContent: c}
		}},
	}
	for _, r := range results {
		for _, c := range contents {
			t.Run(r.name+"/"+c.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				api101bOK(server, "answerInlineQuery")
				client := testutil.NewTestClient(t, server.BaseURL())
				require.NoError(t, client.AnswerInlineQuery(context.Background(), sender.AnswerInlineQueryRequest{
					InlineQueryID: "q", Results: []tg.InlineQueryResult{r.make(c.c)}}))
				require.Equal(t, 1, server.CaptureCount())
				var body struct {
					Results []struct {
						Content json.RawMessage `json:"input_message_content"`
					} `json:"results"`
				}
				server.LastCapture().BodyJSON(t, &body)
				require.Len(t, body.Results, 1)
				assert.JSONEq(t, c.want, string(body.Results[0].Content))
			})
		}
	}
}
