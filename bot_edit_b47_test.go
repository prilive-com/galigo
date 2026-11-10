package galigo

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// T-6 (root): Bot.Edit passes the inline (nil, nil), an inline parse error and a chat Message through (B-47).
func TestB47_BotEdit(t *testing.T) {
	rows := []struct {
		name, body, want string
		target           tg.Editable
		wantMsg          *tg.Message
		wantErr          string
	}{
		{"inline", `{"ok":true,"result":true}`, `{"inline_message_id":"im-1","text":"x"}`, tg.InlineMessage{InlineMessageID: "im-1"}, nil, ""},
		{"inline-false", `{"ok":true,"result":false}`, `{"inline_message_id":"im-1","text":"x"}`, tg.InlineMessage{InlineMessageID: "im-1"}, nil,
			"failed to parse message: json: cannot unmarshal bool into Go value of type tg.Message"},
		{"chat", `{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":100,"type":"private"}}}`, `{"chat_id":100,"message_id":7,"text":"x"}`,
			&tg.Message{MessageID: 7, Chat: &tg.Chat{ID: 100}}, &tg.Message{MessageID: 7, Date: 1, Chat: &tg.Chat{ID: 100, Type: "private"}}, ""},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(row.body))
			})
			b := &Bot{sender: testutil.NewTestClient(t, server.BaseURL())}
			msg, err := b.Edit(context.Background(), row.target, "x")
			if row.wantErr != "" {
				require.EqualError(t, err, row.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, row.wantMsg, msg)
			require.Equal(t, 1, server.CaptureCount())
			assert.JSONEq(t, row.want, server.LastCapture().BodyString())
		})
	}
}
