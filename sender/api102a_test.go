package sender_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
)

// T-gc-1: getChat returns ChatFullInfo.community (Bot API 10.2) when Telegram sends it, and nil when it does not.
// Ids are fake.
func TestAPI102A_GetChatCommunity(t *testing.T) {
	const none = `{"id":-1000000000102,"title":"Test Group","type":"supergroup","max_reaction_count":11,"accent_color_id":1}`
	const with = `{"id":-1000000000102,"title":"Test Group","type":"supergroup","max_reaction_count":11,"accent_color_id":1,` +
		`"community":{"id":4503599627370495,"name":"Test Community"}}`
	for _, tc := range []struct {
		name, body string
		want       bool
	}{{"with", with, true}, {"without", none, false}} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyOK(w, json.RawMessage(tc.body))
			})
			chat, err := testutil.NewTestClient(t, server.BaseURL()).GetChat(context.Background(), int64(-1000000000102))
			require.NoError(t, err)
			require.NotNil(t, chat)
			if !tc.want {
				assert.Nil(t, chat.Community)
				return
			}
			require.NotNil(t, chat.Community)
			assert.Equal(t, int64(4503599627370495), int64(chat.Community.ID))
			assert.Equal(t, "Test Community", chat.Community.Name)
		})
	}
}
