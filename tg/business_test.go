package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBusinessConnection_Unmarshal(t *testing.T) {
	data := `{"id":"bc_1","user":{"id":456,"is_bot":false,"first_name":"Alice"},"user_chat_id":789,"date":1700000000,"rights":{"can_reply":true,"can_read_messages":true},"is_enabled":true}`

	var bc BusinessConnection
	require.NoError(t, json.Unmarshal([]byte(data), &bc))
	assert.Equal(t, "bc_1", bc.ID)
	assert.Equal(t, int64(456), bc.User.ID)
	assert.True(t, bc.Rights.CanReply)
	assert.True(t, bc.IsEnabled)
}

func TestStory_Unmarshal(t *testing.T) {
	data := `{"id":1,"chat":{"id":123,"type":"private"},"date":1700000000}`

	var s Story
	require.NoError(t, json.Unmarshal([]byte(data), &s))
	assert.Equal(t, 1, s.ID)
	assert.Equal(t, int64(123), s.Chat.ID)
}
