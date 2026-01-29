package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Forum Requests ==================

// CreateForumTopicRequest represents a createForumTopic request.
type CreateForumTopicRequest struct {
	ChatID            tg.ChatID `json:"chat_id"`
	Name              string    `json:"name"`
	IconColor         *int      `json:"icon_color,omitempty"`
	IconCustomEmojiID string    `json:"icon_custom_emoji_id,omitempty"`
}

// EditForumTopicRequest represents an editForumTopic request.
type EditForumTopicRequest struct {
	ChatID            tg.ChatID `json:"chat_id"`
	MessageThreadID   int       `json:"message_thread_id"`
	Name              *string   `json:"name,omitempty"`
	IconCustomEmojiID *string   `json:"icon_custom_emoji_id,omitempty"`
}

// ForumTopicRequest represents a request that operates on a forum topic.
type ForumTopicRequest struct {
	ChatID          tg.ChatID `json:"chat_id"`
	MessageThreadID int       `json:"message_thread_id"`
}

// ================== Forum Methods ==================

// CreateForumTopic creates a topic in a forum supergroup chat.
// The bot must be an administrator with can_manage_topics rights.
func (c *Client) CreateForumTopic(ctx context.Context, chatID tg.ChatID, name string, opts ...CreateTopicOption) (*tg.ForumTopic, error) {
	if err := validateChatID(chatID); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, tg.NewValidationError("name", "cannot be empty")
	}
	if len(name) > 128 {
		return nil, tg.NewValidationError("name", "must be at most 128 characters")
	}

	req := CreateForumTopicRequest{
		ChatID: chatID,
		Name:   name,
	}
	for _, opt := range opts {
		opt(&req)
	}

	var topic tg.ForumTopic
	if err := c.callJSON(ctx, "createForumTopic", req, &topic); err != nil {
		return nil, err
	}
	return &topic, nil
}

// EditForumTopic edits name and icon of a topic.
func (c *Client) EditForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int, opts ...EditTopicOption) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateThreadID(messageThreadID); err != nil {
		return err
	}

	req := EditForumTopicRequest{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
	}
	for _, opt := range opts {
		opt(&req)
	}

	return c.callJSON(ctx, "editForumTopic", req, nil)
}

// CloseForumTopic closes an open topic.
func (c *Client) CloseForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateThreadID(messageThreadID); err != nil {
		return err
	}

	return c.callJSON(ctx, "closeForumTopic", ForumTopicRequest{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
	}, nil)
}

// ReopenForumTopic reopens a closed topic.
func (c *Client) ReopenForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateThreadID(messageThreadID); err != nil {
		return err
	}

	return c.callJSON(ctx, "reopenForumTopic", ForumTopicRequest{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
	}, nil)
}

// DeleteForumTopic deletes a topic along with all its messages.
func (c *Client) DeleteForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateThreadID(messageThreadID); err != nil {
		return err
	}

	return c.callJSON(ctx, "deleteForumTopic", ForumTopicRequest{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
	}, nil)
}

// UnpinAllForumTopicMessages unpins all messages in a topic.
func (c *Client) UnpinAllForumTopicMessages(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateThreadID(messageThreadID); err != nil {
		return err
	}

	return c.callJSON(ctx, "unpinAllForumTopicMessages", ForumTopicRequest{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
	}, nil)
}

// EditGeneralForumTopic edits the name of the General topic.
func (c *Client) EditGeneralForumTopic(ctx context.Context, chatID tg.ChatID, name string) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "editGeneralForumTopic", struct {
		ChatID tg.ChatID `json:"chat_id"`
		Name   string    `json:"name"`
	}{ChatID: chatID, Name: name}, nil)
}

// CloseGeneralForumTopic closes the General topic.
func (c *Client) CloseGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "closeGeneralForumTopic", struct {
		ChatID tg.ChatID `json:"chat_id"`
	}{ChatID: chatID}, nil)
}

// ReopenGeneralForumTopic reopens the General topic.
func (c *Client) ReopenGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "reopenGeneralForumTopic", struct {
		ChatID tg.ChatID `json:"chat_id"`
	}{ChatID: chatID}, nil)
}

// HideGeneralForumTopic hides the General topic.
func (c *Client) HideGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "hideGeneralForumTopic", struct {
		ChatID tg.ChatID `json:"chat_id"`
	}{ChatID: chatID}, nil)
}

// UnhideGeneralForumTopic unhides the General topic.
func (c *Client) UnhideGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "unhideGeneralForumTopic", struct {
		ChatID tg.ChatID `json:"chat_id"`
	}{ChatID: chatID}, nil)
}

// UnpinAllGeneralForumTopicMessages unpins all messages in the General topic.
func (c *Client) UnpinAllGeneralForumTopicMessages(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "unpinAllGeneralForumTopicMessages", struct {
		ChatID tg.ChatID `json:"chat_id"`
	}{ChatID: chatID}, nil)
}

// GetForumTopicIconStickers returns custom emoji stickers usable as topic icons.
func (c *Client) GetForumTopicIconStickers(ctx context.Context) ([]*tg.Sticker, error) {
	var stickers []*tg.Sticker
	if err := c.callJSON(ctx, "getForumTopicIconStickers", struct{}{}, &stickers); err != nil {
		return nil, err
	}
	return stickers, nil
}

// ================== Options ==================

// CreateTopicOption configures CreateForumTopic.
type CreateTopicOption func(*CreateForumTopicRequest)

// WithTopicColor sets the icon color (use tg.ForumColor* constants).
func WithTopicColor(color int) CreateTopicOption {
	return func(r *CreateForumTopicRequest) {
		r.IconColor = &color
	}
}

// WithTopicEmoji sets a custom emoji for the topic icon.
func WithTopicEmoji(emojiID string) CreateTopicOption {
	return func(r *CreateForumTopicRequest) {
		r.IconCustomEmojiID = emojiID
	}
}

// EditTopicOption configures EditForumTopic.
type EditTopicOption func(*EditForumTopicRequest)

// WithEditTopicName changes the topic name.
func WithEditTopicName(name string) EditTopicOption {
	return func(r *EditForumTopicRequest) {
		r.Name = &name
	}
}

// WithEditTopicEmoji changes the topic icon emoji.
func WithEditTopicEmoji(emojiID string) EditTopicOption {
	return func(r *EditForumTopicRequest) {
		r.IconCustomEmojiID = &emojiID
	}
}
