package sender_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 part b on the wire: a reply to an ephemeral message has no message_id (JSON and multipart), and
// BotCommand.is_ephemeral is sent and read. Commands with is_ephemeral come from literal JSON, never a Go literal.

func api102bCommands(t *testing.T, raw string) []tg.BotCommand {
	t.Helper()
	var cmds []tg.BotCommand
	require.NoError(t, json.Unmarshal([]byte(raw), &cmds))
	return cmds
}

// T-sm-1: sendMessage with a reply to an ephemeral message (and to a quote only) sends no message_id.
func TestAPI102B_SendMessageEphemeralReply(t *testing.T) {
	for _, tc := range []struct {
		name string
		rp   *tg.ReplyParameters
		want string
	}{
		{"ephemeral", &tg.ReplyParameters{EphemeralMessageID: 7}, `{"chat_id":123456789,"text":"t","reply_parameters":{"ephemeral_message_id":7}}`},
		{"quote", &tg.ReplyParameters{Quote: "q"}, `{"chat_id":123456789,"text":"t","reply_parameters":{"quote":"q"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/sendMessage", replyMessage)
			client := testutil.NewTestClient(t, server.BaseURL())
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
				ChatID: testutil.TestChatID, Text: "t", ReplyParameters: tc.rp,
			})
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			c := server.LastCapture()
			assert.Equal(t, "/bot"+testutil.TestToken+"/sendMessage", c.Path)
			assert.JSONEq(t, tc.want, string(c.Body))
		})
	}
}

// T-slp-1: sendLivePhoto with two uploads goes through the multipart builder and sends no message_id.
func TestAPI102B_SendLivePhotoEphemeralReply(t *testing.T) {
	mediaType, args, _, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
		ChatID:          testutil.TestChatID,
		LivePhoto:       sender.FromBytes(lpVideoBytes, "v.mp4"),
		Photo:           sender.FromBytes(lpPhotoBytes, "p.jpg"),
		ReplyParameters: &tg.ReplyParameters{EphemeralMessageID: 7},
	}))
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id":          wireChatID,
		"reply_parameters": `{"ephemeral_message_id":7}`,
	}, args)
}

// T-smc-1: setMyCommands sends is_ephemeral for an ephemeral command and no key for a normal one.
func TestAPI102B_SetMyCommandsEphemeral(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyCommands", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, true)
	})
	client := testutil.NewTestClient(t, server.BaseURL())
	cmds := api102bCommands(t, `[{"command":"c102b","description":"E","is_ephemeral":true},{"command":"c102bn","description":"N"}]`)
	require.NoError(t, client.SetMyCommands(context.Background(), cmds, sender.WithCommandLanguage("eo")))
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t,
		`{"commands":[{"command":"c102b","description":"E","is_ephemeral":true},{"command":"c102bn","description":"N"}],"language_code":"eo"}`,
		string(server.LastCapture().Body))
}

// T-gmc-1: getMyCommands reads is_ephemeral in its true, false and absent forms.
func TestAPI102B_GetMyCommandsEphemeral(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMyCommands", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, json.RawMessage(`[{"command":"c102b","description":"E","is_ephemeral":true},`+
			`{"command":"c102bn","description":"N","is_ephemeral":false},{"command":"x","description":"X"}]`))
	})
	client := testutil.NewTestClient(t, server.BaseURL())
	cmds, err := client.GetMyCommands(context.Background(), sender.WithCommandLanguage("eo"))
	require.NoError(t, err)
	require.Len(t, cmds, 3)
	assert.Equal(t, true, cmds[0].IsEphemeral)
	assert.Equal(t, false, cmds[1].IsEphemeral)
	assert.Equal(t, false, cmds[2].IsEphemeral)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"language_code":"eo"}`, string(server.LastCapture().Body))
}
