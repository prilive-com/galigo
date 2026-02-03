package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Pin Requests ==================

// PinChatMessageRequest represents a pinChatMessage request.
type PinChatMessageRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	MessageID           int       `json:"message_id"`
	DisableNotification bool      `json:"disable_notification,omitempty"`
}

// UnpinChatMessageRequest represents an unpinChatMessage request.
type UnpinChatMessageRequest struct {
	ChatID    tg.ChatID `json:"chat_id"`
	MessageID *int      `json:"message_id,omitempty"` // nil = unpin most recent
}

// UnpinAllChatMessagesRequest represents an unpinAllChatMessages request.
type UnpinAllChatMessagesRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
}

// LeaveChatRequest represents a leaveChat request.
type LeaveChatRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
}

// ================== Pin Methods ==================

// PinChatMessage pins a message in a chat.
// The bot must be an administrator with can_pin_messages rights.
func (c *Client) PinChatMessage(ctx context.Context, chatID tg.ChatID, messageID int, opts ...PinOption) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateMessageID(messageID); err != nil {
		return err
	}

	req := PinChatMessageRequest{
		ChatID:    chatID,
		MessageID: messageID,
	}
	for _, opt := range opts {
		opt(&req)
	}

	return c.callJSON(ctx, "pinChatMessage", req, nil, extractChatID(chatID))
}

// UnpinChatMessage unpins a message in a chat.
// If messageID is 0, unpins the most recent pinned message.
func (c *Client) UnpinChatMessage(ctx context.Context, chatID tg.ChatID, messageID int) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	req := UnpinChatMessageRequest{ChatID: chatID}
	if messageID > 0 {
		req.MessageID = &messageID
	}

	return c.callJSON(ctx, "unpinChatMessage", req, nil, extractChatID(chatID))
}

// UnpinAllChatMessages unpins all pinned messages in a chat.
func (c *Client) UnpinAllChatMessages(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "unpinAllChatMessages", UnpinAllChatMessagesRequest{
		ChatID: chatID,
	}, nil, extractChatID(chatID))
}

// LeaveChat makes the bot leave a group, supergroup, or channel.
func (c *Client) LeaveChat(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "leaveChat", LeaveChatRequest{
		ChatID: chatID,
	}, nil, extractChatID(chatID))
}

// ================== Options ==================

// PinOption configures PinChatMessage.
type PinOption func(*PinChatMessageRequest)

// WithSilentPin pins the message without notification.
func WithSilentPin() PinOption {
	return func(r *PinChatMessageRequest) {
		r.DisableNotification = true
	}
}
