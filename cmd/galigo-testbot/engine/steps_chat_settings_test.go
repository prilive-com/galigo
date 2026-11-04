package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
)

// T-tb-6 (B-25 slice 2a, Bot API 10.0, plan §8 and AC4(d)): S32's save and restore keep can_react_to_messages
// exactly, end to end (decode, copy, encode) through the real adapter and sender.
func TestChatPermissionsRestore100(t *testing.T) {
	// API100 T-tb-6
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, map[string]any{"id": testutil.TestChatID, "type": "supergroup",
			"permissions": map[string]any{"can_send_messages": true, "can_react_to_messages": false}})
	})
	server.On("/bot"+testutil.TestToken+"/setChatPermissions", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyBool(w, true)
	})
	rt := NewRuntime(NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), testutil.TestChatID, testutil.TestUserID)
	rt.ChatCtx = &ChatContext{BotIsAdmin: true, CanRestrictMembers: true}

	_, err := (&SaveChatPermissionsStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	_, err = (&RestoreChatPermissionsStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)

	require.Equal(t, 2, server.CaptureCount())
	set := server.CaptureAt(1)
	set.AssertPath(t, "/bot"+testutil.TestToken+"/setChatPermissions")
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(set.Body, &body))
	assert.JSONEq(t, `{"can_send_messages":true,"can_react_to_messages":false}`, string(body["permissions"]))
}
