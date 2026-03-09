package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== PromoteChatMember ====================

func TestPromoteChatMember(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["can_delete_messages"])
		assert.Equal(t, true, req["can_restrict_members"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PromoteChatMember(context.Background(), int64(-100123), 456,
		sender.WithCanDeleteMessages(true),
		sender.WithCanRestrictMembers(true),
	)
	assert.NoError(t, err)
}

func TestPromoteChatMemberWithRights(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["can_manage_chat"])
		assert.Equal(t, true, req["can_delete_messages"])
		assert.Equal(t, true, req["can_restrict_members"])
		assert.Equal(t, false, req["can_promote_members"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PromoteChatMemberWithRights(context.Background(), int64(-100123), 456,
		tg.ModeratorRights(),
	)
	assert.NoError(t, err)
}

func TestPromoteChatMember_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PromoteChatMember(context.Background(), nil, 456)
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestPromoteChatMember_Validation_InvalidUserID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PromoteChatMember(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestPromoteChatMember_Error_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "not enough rights to promote")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.PromoteChatMember(context.Background(), int64(-100123), 456)

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 403, apiErr.Code)
}

// ==================== DemoteChatMember ====================

func TestDemoteChatMember(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, false, req["can_manage_chat"])
		assert.Equal(t, false, req["can_delete_messages"])
		assert.Equal(t, false, req["can_restrict_members"])
		assert.Equal(t, false, req["can_promote_members"])
		assert.Equal(t, false, req["can_change_info"])
		assert.Equal(t, false, req["can_invite_users"])
		assert.Equal(t, false, req["can_pin_messages"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DemoteChatMember(context.Background(), int64(-100123), 456)
	assert.NoError(t, err)
}

// ==================== SetChatAdministratorCustomTitle ====================

func TestSetChatAdministratorCustomTitle(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatAdministratorCustomTitle", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "Moderator", req["custom_title"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatAdministratorCustomTitle(context.Background(), int64(-100123), 456, "Moderator")
	assert.NoError(t, err)
}

func TestSetChatAdministratorCustomTitle_Validation_TooLong(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatAdministratorCustomTitle(context.Background(), int64(-100123), 456,
		"This title is way too long!",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "16 characters")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSetChatAdministratorCustomTitle_Validation_MultibyteChars(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatAdministratorCustomTitle", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})
	client := testutil.NewTestClient(t, server.BaseURL())

	// 16 runes but >16 bytes in UTF-8 — must be accepted
	title := "Ünîcödë Àdmïn!!" // 16 runes
	err := client.SetChatAdministratorCustomTitle(context.Background(), int64(-100123), 456, title)
	assert.NoError(t, err, "multi-byte title within 16-char limit should be accepted")
}

func TestSetChatAdministratorCustomTitle_Empty(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatAdministratorCustomTitle", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	// Empty string should be allowed (clears the title)
	err := client.SetChatAdministratorCustomTitle(context.Background(), int64(-100123), 456, "")
	assert.NoError(t, err)
}

func TestPromoteOptions(t *testing.T) {
	opts := []sender.PromoteOption{
		sender.WithAnonymous(true),
		sender.WithCanManageChat(true),
		sender.WithCanDeleteMessages(true),
		sender.WithCanManageVideoChats(true),
		sender.WithCanRestrictMembers(true),
		sender.WithCanPromoteMembers(true),
		sender.WithCanChangeInfo(true),
		sender.WithCanInviteUsers(true),
		sender.WithCanPostMessages(true),
		sender.WithCanEditMessages(true),
		sender.WithCanPinMessages(true),
		sender.WithCanManageTopics(true),
		sender.WithCanManageTags(true),
	}
	assert.Len(t, opts, 13)
}

// ==================== Bot API 9.5: CanManageTags ====================

func TestPromoteChatMember_WithCanManageTags(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["can_manage_tags"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PromoteChatMember(context.Background(), int64(-100123), int64(789),
		sender.WithCanManageTags(true),
	)
	assert.NoError(t, err)
}

func TestDemoteChatMember_ClearsCanManageTags(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, false, req["can_manage_tags"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DemoteChatMember(context.Background(), int64(-100123), int64(789))
	assert.NoError(t, err)
}
