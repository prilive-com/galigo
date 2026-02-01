package sender_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SendGift ====================

func TestSendGift(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendGift", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SendGift(context.Background(), sender.SendGiftRequest{
		UserID: 456,
		GiftID: "gift_abc",
	})
	require.NoError(t, err)
}

func TestSendGift_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SendGiftRequest
		want string
	}{
		{"missing user_id", sender.SendGiftRequest{GiftID: "g"}, "user_id"},
		{"missing gift_id", sender.SendGiftRequest{UserID: 1}, "gift_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SendGift(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== GetAvailableGifts ====================

func TestGetAvailableGifts(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getAvailableGifts", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"gifts": []map[string]any{
				{"id": "gift_1", "sticker": map[string]any{"file_id": "f1", "file_unique_id": "u1", "type": "regular", "width": 512, "height": 512, "is_animated": false, "is_video": false}, "star_count": 50},
			},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	gifts, err := client.GetAvailableGifts(context.Background())
	require.NoError(t, err)
	require.Len(t, gifts.Gifts, 1)
	assert.Equal(t, "gift_1", gifts.Gifts[0].ID)
	assert.Equal(t, 50, gifts.Gifts[0].StarCount)
}

// ==================== TransferGift ====================

func TestTransferGift(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/transferGift", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.TransferGift(context.Background(), sender.TransferGiftRequest{
		BusinessConnectionID: "bc_123",
		OwnedGiftID:          "og_1",
		NewOwnerChatID:       789,
	})
	require.NoError(t, err)
}

func TestTransferGift_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.TransferGiftRequest
		want string
	}{
		{"missing connection_id", sender.TransferGiftRequest{OwnedGiftID: "og", NewOwnerChatID: 1}, "business_connection_id"},
		{"missing owned_gift_id", sender.TransferGiftRequest{BusinessConnectionID: "bc", NewOwnerChatID: 1}, "owned_gift_id"},
		{"missing new_owner", sender.TransferGiftRequest{BusinessConnectionID: "bc", OwnedGiftID: "og"}, "new_owner_chat_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.TransferGift(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== UpgradeGift ====================

func TestUpgradeGift(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/upgradeGift", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.UpgradeGift(context.Background(), sender.UpgradeGiftRequest{
		BusinessConnectionID: "bc_123",
		OwnedGiftID:          "og_1",
	})
	require.NoError(t, err)
}

func TestUpgradeGift_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.UpgradeGiftRequest
		want string
	}{
		{"missing connection_id", sender.UpgradeGiftRequest{OwnedGiftID: "og"}, "business_connection_id"},
		{"missing owned_gift_id", sender.UpgradeGiftRequest{BusinessConnectionID: "bc"}, "owned_gift_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.UpgradeGift(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== ConvertGiftToStars ====================

func TestConvertGiftToStars(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/convertGiftToStars", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.ConvertGiftToStars(context.Background(), sender.ConvertGiftToStarsRequest{
		BusinessConnectionID: "bc_123",
		OwnedGiftID:          "og_1",
	})
	require.NoError(t, err)
}

func TestConvertGiftToStars_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.ConvertGiftToStarsRequest
		want string
	}{
		{"missing connection_id", sender.ConvertGiftToStarsRequest{OwnedGiftID: "og"}, "business_connection_id"},
		{"missing owned_gift_id", sender.ConvertGiftToStarsRequest{BusinessConnectionID: "bc"}, "owned_gift_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.ConvertGiftToStars(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
