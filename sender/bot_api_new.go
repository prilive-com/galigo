package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Bot API 9.1-9.3 Request Types ==================

// SavePreparedInlineMessageRequest represents a savePreparedInlineMessage request.
type SavePreparedInlineMessageRequest struct {
	UserID              int64                `json:"user_id"`
	Result              tg.InlineQueryResult `json:"result"`
	AllowUserChats      bool                 `json:"allow_user_chats,omitempty"`
	AllowBotChats       bool                 `json:"allow_bot_chats,omitempty"`
	AllowGroupChats     bool                 `json:"allow_group_chats,omitempty"`
	AllowChannelChats   bool                 `json:"allow_channel_chats,omitempty"`
}

// GetUserChatBoostsRequest represents a getUserChatBoosts request.
type GetUserChatBoostsRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
	UserID int64     `json:"user_id"`
}

// SetCustomEmojiStickerSetThumbnailRequest represents a setCustomEmojiStickerSetThumbnail request.
type SetCustomEmojiStickerSetThumbnailRequest struct {
	Name          string `json:"name"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// CreateChatSubscriptionInviteLinkRequest represents a createChatSubscriptionInviteLink request.
type CreateChatSubscriptionInviteLinkRequest struct {
	ChatID             tg.ChatID `json:"chat_id"`
	Name               string    `json:"name,omitempty"`
	SubscriptionPeriod int       `json:"subscription_period"`
	SubscriptionPrice  int       `json:"subscription_price"`
}

// EditChatSubscriptionInviteLinkRequest represents an editChatSubscriptionInviteLink request.
type EditChatSubscriptionInviteLinkRequest struct {
	ChatID     tg.ChatID `json:"chat_id"`
	InviteLink string    `json:"invite_link"`
	Name       string    `json:"name,omitempty"`
}

// GetOwnedGiftsRequest represents a getOwnedGifts request.
type GetOwnedGiftsRequest struct {
	UserID         int64  `json:"user_id"`
	Offset         string `json:"offset,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

// ================== Bot API 9.1-9.3 Methods ==================

// SavePreparedInlineMessage stores a message that can be sent by a user of a Mini App.
func (c *Client) SavePreparedInlineMessage(ctx context.Context, req SavePreparedInlineMessageRequest) (*tg.PreparedInlineMessage, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	if req.Result == nil {
		return nil, tg.NewValidationError("result", "required")
	}

	var result tg.PreparedInlineMessage
	if err := c.callJSON(ctx, "savePreparedInlineMessage", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetUserChatBoosts returns the list of boosts added to a chat by a user.
func (c *Client) GetUserChatBoosts(ctx context.Context, req GetUserChatBoostsRequest) (*tg.UserChatBoosts, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}

	var result tg.UserChatBoosts
	if err := c.callJSON(ctx, "getUserChatBoosts", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetCustomEmojiStickerSetThumbnail sets the thumbnail of a custom emoji sticker set.
func (c *Client) SetCustomEmojiStickerSetThumbnail(ctx context.Context, req SetCustomEmojiStickerSetThumbnailRequest) error {
	if req.Name == "" {
		return tg.NewValidationError("name", "required")
	}

	return c.callJSON(ctx, "setCustomEmojiStickerSetThumbnail", req, nil)
}

// CreateChatSubscriptionInviteLink creates a subscription invite link for a channel chat.
func (c *Client) CreateChatSubscriptionInviteLink(ctx context.Context, req CreateChatSubscriptionInviteLinkRequest) (*tg.ChatInviteLink, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.SubscriptionPeriod <= 0 {
		return nil, tg.NewValidationError("subscription_period", "must be positive")
	}
	if req.SubscriptionPrice <= 0 {
		return nil, tg.NewValidationError("subscription_price", "must be positive")
	}

	var result tg.ChatInviteLink
	if err := c.callJSON(ctx, "createChatSubscriptionInviteLink", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EditChatSubscriptionInviteLink edits a subscription invite link created by the bot.
func (c *Client) EditChatSubscriptionInviteLink(ctx context.Context, req EditChatSubscriptionInviteLinkRequest) (*tg.ChatInviteLink, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.InviteLink == "" {
		return nil, tg.NewValidationError("invite_link", "required")
	}

	var result tg.ChatInviteLink
	if err := c.callJSON(ctx, "editChatSubscriptionInviteLink", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOwnedGifts returns the gifts owned by the specified user.
func (c *Client) GetOwnedGifts(ctx context.Context, req GetOwnedGiftsRequest) (*tg.OwnedGifts, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}

	var result tg.OwnedGifts
	if err := c.callJSON(ctx, "getOwnedGifts", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
