package sender_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SavePreparedInlineMessage ====================

func TestSavePreparedInlineMessage(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/savePreparedInlineMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{"id": "prep_123", "expiration_date": 1700000000})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	result, err := client.SavePreparedInlineMessage(context.Background(), sender.SavePreparedInlineMessageRequest{
		UserID:         456,
		Result:         tg.InlineQueryResultArticle{ID: "1", Title: "Test", InputMessageContent: tg.InputTextMessageContent{MessageText: "hi"}},
		AllowUserChats: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "prep_123", result.ID)
}

func TestSavePreparedInlineMessage_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SavePreparedInlineMessageRequest
		want string
	}{
		{"missing user_id", sender.SavePreparedInlineMessageRequest{Result: tg.InlineQueryResultArticle{}}, "user_id"},
		{"missing result", sender.SavePreparedInlineMessageRequest{UserID: 1}, "result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.SavePreparedInlineMessage(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== GetUserChatBoosts ====================

func TestGetUserChatBoosts(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getUserChatBoosts", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{"boosts": []any{}})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	boosts, err := client.GetUserChatBoosts(context.Background(), sender.GetUserChatBoostsRequest{
		ChatID: int64(123),
		UserID: 456,
	})
	require.NoError(t, err)
	assert.NotNil(t, boosts)
}

func TestGetUserChatBoosts_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.GetUserChatBoostsRequest
		want string
	}{
		{"missing chat_id", sender.GetUserChatBoostsRequest{UserID: 1}, "chat_id"},
		{"missing user_id", sender.GetUserChatBoostsRequest{ChatID: int64(1)}, "user_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.GetUserChatBoosts(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== SetCustomEmojiStickerSetThumbnail ====================

func TestSetCustomEmojiStickerSetThumbnail(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setCustomEmojiStickerSetThumbnail", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetCustomEmojiStickerSetThumbnail(context.Background(), sender.SetCustomEmojiStickerSetThumbnailRequest{
		Name:          "test_set",
		CustomEmojiID: "emoji_123",
	})
	require.NoError(t, err)
}

func TestSetCustomEmojiStickerSetThumbnail_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetCustomEmojiStickerSetThumbnail(context.Background(), sender.SetCustomEmojiStickerSetThumbnailRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

// ==================== CreateChatSubscriptionInviteLink ====================

func TestCreateChatSubscriptionInviteLink(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/createChatSubscriptionInviteLink", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"invite_link": "https://t.me/+abc123",
			"creator":     map[string]any{"id": int64(1), "is_bot": true, "first_name": "Bot"},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	link, err := client.CreateChatSubscriptionInviteLink(context.Background(), sender.CreateChatSubscriptionInviteLinkRequest{
		ChatID:             int64(123),
		SubscriptionPeriod: 2592000,
		SubscriptionPrice:  100,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://t.me/+abc123", link.InviteLink)
}

func TestCreateChatSubscriptionInviteLink_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.CreateChatSubscriptionInviteLinkRequest
		want string
	}{
		{"missing chat_id", sender.CreateChatSubscriptionInviteLinkRequest{SubscriptionPeriod: 1, SubscriptionPrice: 1}, "chat_id"},
		{"missing period", sender.CreateChatSubscriptionInviteLinkRequest{ChatID: int64(1), SubscriptionPrice: 1}, "subscription_period"},
		{"missing price", sender.CreateChatSubscriptionInviteLinkRequest{ChatID: int64(1), SubscriptionPeriod: 1}, "subscription_price"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateChatSubscriptionInviteLink(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== EditChatSubscriptionInviteLink ====================

func TestEditChatSubscriptionInviteLink(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editChatSubscriptionInviteLink", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"invite_link": "https://t.me/+abc123",
			"creator":     map[string]any{"id": int64(1), "is_bot": true, "first_name": "Bot"},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	link, err := client.EditChatSubscriptionInviteLink(context.Background(), sender.EditChatSubscriptionInviteLinkRequest{
		ChatID:     int64(123),
		InviteLink: "https://t.me/+abc123",
		Name:       "Updated",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://t.me/+abc123", link.InviteLink)
}

func TestEditChatSubscriptionInviteLink_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.EditChatSubscriptionInviteLinkRequest
		want string
	}{
		{"missing chat_id", sender.EditChatSubscriptionInviteLinkRequest{InviteLink: "link"}, "chat_id"},
		{"missing invite_link", sender.EditChatSubscriptionInviteLinkRequest{ChatID: int64(1)}, "invite_link"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.EditChatSubscriptionInviteLink(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== GetOwnedGifts ====================

func TestGetOwnedGifts(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getOwnedGifts", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"total_count": 1,
			"gifts": []map[string]any{
				{"type": "regular", "send_date": 1700000000},
			},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	gifts, err := client.GetOwnedGifts(context.Background(), sender.GetOwnedGiftsRequest{UserID: 456})
	require.NoError(t, err)
	assert.Equal(t, 1, gifts.TotalCount)
	require.Len(t, gifts.Gifts, 1)
}

func TestGetOwnedGifts_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetOwnedGifts(context.Background(), sender.GetOwnedGiftsRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}
