package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Gift Request Types ==================

// SendGiftRequest represents a sendGift request.
type SendGiftRequest struct {
	UserID        int64              `json:"user_id"`
	GiftID        string             `json:"gift_id"`
	PayForUpgrade bool               `json:"pay_for_upgrade,omitempty"`
	Text          string             `json:"text,omitempty"`
	TextParseMode string             `json:"text_parse_mode,omitempty"`
	TextEntities  []tg.MessageEntity `json:"text_entities,omitempty"`
}

// TransferGiftRequest represents a transferGift request.
type TransferGiftRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	OwnedGiftID          string `json:"owned_gift_id"`
	NewOwnerChatID       int64  `json:"new_owner_chat_id"`
	StarCount            int    `json:"star_count,omitempty"`
}

// UpgradeGiftRequest represents an upgradeGift request.
type UpgradeGiftRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	OwnedGiftID          string `json:"owned_gift_id"`
	KeepOriginalDetails  bool   `json:"keep_original_details,omitempty"`
	StarCount            int    `json:"star_count,omitempty"`
}

// ConvertGiftToStarsRequest represents a convertGiftToStars request.
type ConvertGiftToStarsRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	OwnedGiftID          string `json:"owned_gift_id"`
}

// ================== Gift Methods ==================

// SendGift sends a gift to a user.
// NO RETRY — value operation to prevent double-send.
func (c *Client) SendGift(ctx context.Context, req SendGiftRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	if req.GiftID == "" {
		return tg.NewValidationError("gift_id", "required")
	}

	// NO RETRY — value operation
	return c.callJSON(ctx, "sendGift", req, nil, extractChatID(tg.ChatID(req.UserID)))
}

// GetAvailableGifts returns the list of gifts that can be sent by the bot.
func (c *Client) GetAvailableGifts(ctx context.Context) (*tg.Gifts, error) {
	var result tg.Gifts
	if err := c.callJSON(ctx, "getAvailableGifts", struct{}{}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TransferGift transfers an owned gift to another user.
// NO RETRY — value operation to prevent double-transfer.
func (c *Client) TransferGift(ctx context.Context, req TransferGiftRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.OwnedGiftID == "" {
		return tg.NewValidationError("owned_gift_id", "required")
	}
	if req.NewOwnerChatID <= 0 {
		return tg.NewValidationError("new_owner_chat_id", "required")
	}

	// NO RETRY — value operation
	return c.callJSON(ctx, "transferGift", req, nil)
}

// UpgradeGift upgrades an owned gift.
// NO RETRY — value operation to prevent double-upgrade.
func (c *Client) UpgradeGift(ctx context.Context, req UpgradeGiftRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.OwnedGiftID == "" {
		return tg.NewValidationError("owned_gift_id", "required")
	}

	// NO RETRY — value operation
	return c.callJSON(ctx, "upgradeGift", req, nil)
}

// ConvertGiftToStars converts an owned gift to Telegram Stars.
// NO RETRY — value operation to prevent double-conversion.
func (c *Client) ConvertGiftToStars(ctx context.Context, req ConvertGiftToStarsRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.OwnedGiftID == "" {
		return tg.NewValidationError("owned_gift_id", "required")
	}

	// NO RETRY — value operation
	return c.callJSON(ctx, "convertGiftToStars", req, nil)
}
