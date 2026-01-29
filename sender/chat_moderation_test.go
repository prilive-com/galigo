package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== BanChatMember ====================

func TestBanChatMember(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/banChatMember", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.BanChatMember(context.Background(), int64(-100123), 456)
	assert.NoError(t, err)
}

func TestBanChatMember_WithOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/banChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["revoke_messages"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.BanChatMember(context.Background(), int64(-100123), 456,
		sender.WithRevokeMessages(),
	)
	assert.NoError(t, err)
}

func TestBanChatMember_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.BanChatMember(context.Background(), nil, 456)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestBanChatMember_Validation_InvalidUserID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.BanChatMember(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestBanChatMember_Error_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/banChatMember", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "not enough rights to ban")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.BanChatMember(context.Background(), int64(-100123), 456)

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 403, apiErr.Code)
}

// ==================== UnbanChatMember ====================

func TestUnbanChatMember(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/unbanChatMember", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnbanChatMember(context.Background(), int64(-100123), 456,
		sender.WithOnlyIfBanned(),
	)
	assert.NoError(t, err)
}

// ==================== RestrictChatMember ====================

func TestRestrictChatMember(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/restrictChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		perms, ok := req["permissions"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, false, perms["can_send_messages"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.RestrictChatMember(context.Background(), int64(-100123), 456,
		tg.ReadOnlyPermissions(),
	)
	assert.NoError(t, err)
}

func TestRestrictChatMember_WithOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/restrictChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["use_independent_chat_permissions"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.RestrictChatMember(context.Background(), int64(-100123), 456,
		tg.NoPermissions(),
		sender.WithIndependentPermissions(),
	)
	assert.NoError(t, err)
}

// ==================== BanChatSenderChat ====================

func TestBanChatSenderChat(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/banChatSenderChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.BanChatSenderChat(context.Background(), int64(-100123), int64(-100456))
	assert.NoError(t, err)
}

// ==================== UnbanChatSenderChat ====================

func TestUnbanChatSenderChat(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/unbanChatSenderChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnbanChatSenderChat(context.Background(), int64(-100123), int64(-100456))
	assert.NoError(t, err)
}
