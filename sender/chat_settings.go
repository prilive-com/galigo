package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Chat Settings Requests ==================

// SetChatPermissionsRequest represents a setChatPermissions request.
type SetChatPermissionsRequest struct {
	ChatID                        tg.ChatID          `json:"chat_id"`
	Permissions                   tg.ChatPermissions `json:"permissions"`
	UseIndependentChatPermissions bool               `json:"use_independent_chat_permissions,omitempty"`
}

// SetChatPhotoRequest represents a setChatPhoto request.
type SetChatPhotoRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
	Photo  InputFile `json:"photo"`
}

// DeleteChatPhotoRequest represents a deleteChatPhoto request.
type DeleteChatPhotoRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
}

// SetChatTitleRequest represents a setChatTitle request.
type SetChatTitleRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
	Title  string    `json:"title"`
}

// SetChatDescriptionRequest represents a setChatDescription request.
type SetChatDescriptionRequest struct {
	ChatID      tg.ChatID `json:"chat_id"`
	Description string    `json:"description,omitempty"`
}

// ================== Chat Settings Methods ==================

// SetChatPermissions sets default chat permissions for all members.
// The bot must be an administrator with can_restrict_members rights.
func (c *Client) SetChatPermissions(ctx context.Context, chatID tg.ChatID, permissions tg.ChatPermissions, opts ...SetPermissionsOption) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	req := SetChatPermissionsRequest{
		ChatID:      chatID,
		Permissions: permissions,
	}
	for _, opt := range opts {
		opt(&req)
	}

	return c.callJSON(ctx, "setChatPermissions", req, nil, extractChatID(chatID))
}

// SetChatPhoto sets a new chat photo.
// The bot must be an administrator with can_change_info rights.
func (c *Client) SetChatPhoto(ctx context.Context, chatID tg.ChatID, photo InputFile) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "setChatPhoto", SetChatPhotoRequest{
		ChatID: chatID,
		Photo:  photo,
	}, nil, extractChatID(chatID))
}

// DeleteChatPhoto deletes the chat photo.
// The bot must be an administrator with can_change_info rights.
func (c *Client) DeleteChatPhoto(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "deleteChatPhoto", DeleteChatPhotoRequest{
		ChatID: chatID,
	}, nil, extractChatID(chatID))
}

// SetChatTitle changes the title of a chat.
// Title length: 1-128 characters.
func (c *Client) SetChatTitle(ctx context.Context, chatID tg.ChatID, title string) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if title == "" {
		return tg.NewValidationError("title", "cannot be empty")
	}
	if len(title) > 128 {
		return tg.NewValidationError("title", "must be at most 128 characters")
	}

	return c.callJSON(ctx, "setChatTitle", SetChatTitleRequest{
		ChatID: chatID,
		Title:  title,
	}, nil, extractChatID(chatID))
}

// SetChatDescription changes the description of a chat.
// Description length: 0-255 characters.
func (c *Client) SetChatDescription(ctx context.Context, chatID tg.ChatID, description string) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if len(description) > 255 {
		return tg.NewValidationError("description", "must be at most 255 characters")
	}

	return c.callJSON(ctx, "setChatDescription", SetChatDescriptionRequest{
		ChatID:      chatID,
		Description: description,
	}, nil, extractChatID(chatID))
}

// ================== Options ==================

// SetPermissionsOption configures SetChatPermissions.
type SetPermissionsOption func(*SetChatPermissionsRequest)

// WithIndependentPermissionsForChat uses independent chat permissions.
func WithIndependentPermissionsForChat() SetPermissionsOption {
	return func(r *SetChatPermissionsRequest) {
		r.UseIndependentChatPermissions = true
	}
}
