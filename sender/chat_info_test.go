package sender_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== GetChat ====================

func TestGetChat(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":    int64(-1001234567890),
			"type":  "supergroup",
			"title": "Test Group",
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	chat, err := client.GetChat(context.Background(), int64(-1001234567890))
	require.NoError(t, err)
	assert.Equal(t, int64(-1001234567890), chat.ID)
	assert.Equal(t, "supergroup", chat.Type)
	assert.Equal(t, "Test Group", chat.Title)
}

func TestGetChat_WithUsername(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":    int64(-100123),
			"type":  "channel",
			"title": "Channel",
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	chat, err := client.GetChat(context.Background(), "@testchannel")
	require.NoError(t, err)
	assert.Equal(t, "channel", chat.Type)
}

func TestGetChat_Validation_NilChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChat(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestGetChat_Validation_ZeroChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChat(context.Background(), int64(0))
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestGetChat_Error_NotFound(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBadRequest(w, "chat not found")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.GetChat(context.Background(), int64(999999))

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.Code)
}

// ==================== GetChatAdministrators ====================

func TestGetChatAdministrators(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChatAdministrators", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]any{
			{"status": "creator", "user": map[string]any{"id": 123, "first_name": "Owner", "is_bot": false}},
			{"status": "administrator", "user": map[string]any{"id": 456, "first_name": "Admin", "is_bot": false}, "can_delete_messages": true},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	admins, err := client.GetChatAdministrators(context.Background(), int64(-100123))
	require.NoError(t, err)
	require.Len(t, admins, 2)
	assert.True(t, tg.IsOwner(admins[0]))
	assert.True(t, tg.IsAdmin(admins[1]))
	assert.False(t, tg.IsOwner(admins[1]))
}

func TestGetChatAdministrators_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChatAdministrators(context.Background(), int64(0))
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount())
}

// ==================== GetChatMemberCount ====================

func TestGetChatMemberCount(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChatMemberCount", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, 42)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	count, err := client.GetChatMemberCount(context.Background(), int64(-100123))
	require.NoError(t, err)
	assert.Equal(t, 42, count)
}

// ==================== GetChatMember ====================

func TestGetChatMember_AllStatuses(t *testing.T) {
	statuses := []string{"creator", "administrator", "member", "restricted", "left", "kicked"}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/getChatMember", func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyOK(w, map[string]any{
					"status": status,
					"user":   map[string]any{"id": 123, "first_name": "Test", "is_bot": false},
				})
			})

			client := testutil.NewTestClient(t, server.BaseURL())
			member, err := client.GetChatMember(context.Background(), int64(-100123), 123)

			require.NoError(t, err)
			assert.Equal(t, status, member.Status())
		})
	}
}

func TestGetChatMember_Validation_InvalidUserID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChatMember(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id must be positive")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== Bucket: Error ====================

func TestChatInfo_Error_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChatAdministrators", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "bot is not a member of the chat")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.GetChatAdministrators(context.Background(), int64(-100123))

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 403, apiErr.Code)
}
