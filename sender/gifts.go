package sender

import (
	"context"
	"fmt"

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

// DeleteBusinessAccountGiftRequest represents a deleteBusinessAccountGift request (Bot API 10.4). Both ids are
// required and sent as given.
type DeleteBusinessAccountGiftRequest struct {
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

// GetUniqueGift returns a unique gift by its name (Bot API 10.4), for example "PlushPepe-1". The name is sent as
// given. A result without a gift_id (null, {} or an empty gift_id) is an error. Not retried.
func (c *Client) GetUniqueGift(ctx context.Context, name string) (*tg.UniqueGift, error) {
	if name == "" {
		return nil, tg.NewValidationError("name", "required")
	}
	var result tg.UniqueGift
	if err := c.callJSON(ctx, "getUniqueGift", struct {
		Name string `json:"name"`
	}{name}, &result); err != nil {
		return nil, err
	}
	if result.GiftID == "" {
		return nil, fmt.Errorf("galigo: getUniqueGift: empty gift_id in response")
	}
	return &result, nil
}

// DeleteBusinessAccountGift deletes a gift from a managed business account (Bot API 10.4). It needs the business
// bot right can_convert_gifts_to_stars. Only a regular gift that is not limited and not refunded can be deleted;
// the deletion gives no Telegram Stars.
// NO RETRY — value operation: after a lost answer the gift may already be gone.
func (c *Client) DeleteBusinessAccountGift(ctx context.Context, req DeleteBusinessAccountGiftRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.OwnedGiftID == "" {
		return tg.NewValidationError("owned_gift_id", "required")
	}
	// NO RETRY — value operation
	return c.callJSON(ctx, "deleteBusinessAccountGift", req, nil)
}

// GetUserGiftsRequest represents a getUserGifts request (Bot API 9.3).
type GetUserGiftsRequest struct {
	UserID                      int64  `json:"user_id"`
	ExcludeUnlimited            bool   `json:"exclude_unlimited,omitempty"`
	ExcludeLimitedUpgradable    bool   `json:"exclude_limited_upgradable,omitempty"`
	ExcludeLimitedNonUpgradable bool   `json:"exclude_limited_non_upgradable,omitempty"`
	ExcludeFromBlockchain       bool   `json:"exclude_from_blockchain,omitempty"`
	ExcludeUnique               bool   `json:"exclude_unique,omitempty"`
	SortByPrice                 bool   `json:"sort_by_price,omitempty"`
	Offset                      string `json:"offset,omitempty"`
	Limit                       int    `json:"limit,omitempty"` // 1-100; 0 is not sent (the server default, 100)
}

// GetChatGiftsRequest represents a getChatGifts request (Bot API 9.3).
type GetChatGiftsRequest struct {
	ChatID                      tg.ChatID `json:"chat_id"`
	ExcludeUnsaved              bool      `json:"exclude_unsaved,omitempty"`
	ExcludeSaved                bool      `json:"exclude_saved,omitempty"`
	ExcludeUnlimited            bool      `json:"exclude_unlimited,omitempty"`
	ExcludeLimitedUpgradable    bool      `json:"exclude_limited_upgradable,omitempty"`
	ExcludeLimitedNonUpgradable bool      `json:"exclude_limited_non_upgradable,omitempty"`
	ExcludeFromBlockchain       bool      `json:"exclude_from_blockchain,omitempty"`
	ExcludeUnique               bool      `json:"exclude_unique,omitempty"`
	SortByPrice                 bool      `json:"sort_by_price,omitempty"`
	Offset                      string    `json:"offset,omitempty"`
	Limit                       int       `json:"limit,omitempty"` // 1-100; 0 is not sent (the server default, 100)
}

// GetBusinessAccountGiftsRequest represents a getBusinessAccountGifts request (Bot API 9.0).
type GetBusinessAccountGiftsRequest struct {
	BusinessConnectionID        string `json:"business_connection_id"`
	ExcludeUnsaved              bool   `json:"exclude_unsaved,omitempty"`
	ExcludeSaved                bool   `json:"exclude_saved,omitempty"`
	ExcludeUnlimited            bool   `json:"exclude_unlimited,omitempty"`
	ExcludeLimitedUpgradable    bool   `json:"exclude_limited_upgradable,omitempty"`
	ExcludeLimitedNonUpgradable bool   `json:"exclude_limited_non_upgradable,omitempty"`
	ExcludeUnique               bool   `json:"exclude_unique,omitempty"`
	ExcludeFromBlockchain       bool   `json:"exclude_from_blockchain,omitempty"`
	SortByPrice                 bool   `json:"sort_by_price,omitempty"`
	Offset                      string `json:"offset,omitempty"`
	Limit                       int    `json:"limit,omitempty"` // 1-100; 0 is not sent (the server default, 100)
}

// validateGiftsLimit allows 0 (not sent) or 1-100.
func validateGiftsLimit(limit int) error {
	if limit < 0 || limit > 100 {
		return tg.NewValidationError("limit", "must be 1-100")
	}
	return nil
}

// GetUserGifts returns the gifts owned and hosted by a user (Bot API 9.3). Not retried.
func (c *Client) GetUserGifts(ctx context.Context, req GetUserGiftsRequest) (*tg.OwnedGifts, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	if err := validateGiftsLimit(req.Limit); err != nil {
		return nil, err
	}
	var result tg.OwnedGifts
	if err := c.callJSON(ctx, "getUserGifts", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetChatGifts returns the gifts owned by a chat (Bot API 9.3). Not retried; it uses the chat's rate limiter. In a
// channel, exclude_unsaved is always true and exclude_saved always false unless the bot has the can_post_messages
// administrator right there (the spec's rule; not checked locally).
func (c *Client) GetChatGifts(ctx context.Context, req GetChatGiftsRequest) (*tg.OwnedGifts, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if err := validateGiftsLimit(req.Limit); err != nil {
		return nil, err
	}
	var result tg.OwnedGifts
	if err := c.callJSON(ctx, "getChatGifts", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetBusinessAccountGifts returns the gifts received and owned by a managed business account (Bot API 9.0). The bot
// needs the can_view_gifts_and_stars business bot right (not checked locally). Not retried.
func (c *Client) GetBusinessAccountGifts(ctx context.Context, req GetBusinessAccountGiftsRequest) (*tg.OwnedGifts, error) {
	if req.BusinessConnectionID == "" {
		return nil, tg.NewValidationError("business_connection_id", "required")
	}
	if err := validateGiftsLimit(req.Limit); err != nil {
		return nil, err
	}
	var result tg.OwnedGifts
	if err := c.callJSON(ctx, "getBusinessAccountGifts", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
