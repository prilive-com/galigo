package sender

import (
	"context"
	"time"

	"github.com/prilive-com/galigo/tg"
)

// ================== Moderation Requests ==================

// BanChatMemberRequest represents a banChatMember request.
type BanChatMemberRequest struct {
	ChatID         tg.ChatID `json:"chat_id"`
	UserID         int64     `json:"user_id"`
	UntilDate      int64     `json:"until_date,omitempty"`
	RevokeMessages bool      `json:"revoke_messages,omitempty"`
}

// UnbanChatMemberRequest represents an unbanChatMember request.
type UnbanChatMemberRequest struct {
	ChatID       tg.ChatID `json:"chat_id"`
	UserID       int64     `json:"user_id"`
	OnlyIfBanned bool      `json:"only_if_banned,omitempty"`
}

// RestrictChatMemberRequest represents a restrictChatMember request.
type RestrictChatMemberRequest struct {
	ChatID                        tg.ChatID          `json:"chat_id"`
	UserID                        int64              `json:"user_id"`
	Permissions                   tg.ChatPermissions `json:"permissions"`
	UseIndependentChatPermissions bool               `json:"use_independent_chat_permissions,omitempty"`
	UntilDate                     int64              `json:"until_date,omitempty"`
}

// BanChatSenderChatRequest represents a banChatSenderChat request.
type BanChatSenderChatRequest struct {
	ChatID       tg.ChatID `json:"chat_id"`
	SenderChatID int64     `json:"sender_chat_id"`
}

// UnbanChatSenderChatRequest represents an unbanChatSenderChat request.
type UnbanChatSenderChatRequest struct {
	ChatID       tg.ChatID `json:"chat_id"`
	SenderChatID int64     `json:"sender_chat_id"`
}

// ================== Moderation Methods ==================

// BanChatMember bans a user in a group, supergroup, or channel.
// The user will not be able to return to the chat on their own using invite links.
func (c *Client) BanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...BanOption) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}

	req := BanChatMemberRequest{
		ChatID: chatID,
		UserID: userID,
	}
	for _, opt := range opts {
		opt(&req)
	}

	return c.callJSON(ctx, "banChatMember", req, nil)
}

// UnbanChatMember unbans a previously banned user in a supergroup or channel.
func (c *Client) UnbanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...UnbanOption) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}

	req := UnbanChatMemberRequest{
		ChatID: chatID,
		UserID: userID,
	}
	for _, opt := range opts {
		opt(&req)
	}

	return c.callJSON(ctx, "unbanChatMember", req, nil)
}

// RestrictChatMember restricts a user in a supergroup.
// The bot must be an administrator with can_restrict_members rights.
func (c *Client) RestrictChatMember(ctx context.Context, chatID tg.ChatID, userID int64, permissions tg.ChatPermissions, opts ...RestrictOption) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}

	req := RestrictChatMemberRequest{
		ChatID:      chatID,
		UserID:      userID,
		Permissions: permissions,
	}
	for _, opt := range opts {
		opt(&req)
	}

	return c.callJSON(ctx, "restrictChatMember", req, nil)
}

// BanChatSenderChat bans a channel chat in a supergroup or channel.
func (c *Client) BanChatSenderChat(ctx context.Context, chatID tg.ChatID, senderChatID int64) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "banChatSenderChat", BanChatSenderChatRequest{
		ChatID:       chatID,
		SenderChatID: senderChatID,
	}, nil)
}

// UnbanChatSenderChat unbans a previously banned channel chat.
func (c *Client) UnbanChatSenderChat(ctx context.Context, chatID tg.ChatID, senderChatID int64) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "unbanChatSenderChat", UnbanChatSenderChatRequest{
		ChatID:       chatID,
		SenderChatID: senderChatID,
	}, nil)
}

// ================== Options ==================

// BanOption configures BanChatMember.
type BanOption func(*BanChatMemberRequest)

// WithBanUntil sets the ban expiration time.
// Users banned for less than 366 days will be automatically unbanned.
func WithBanUntil(until time.Time) BanOption {
	return func(r *BanChatMemberRequest) {
		r.UntilDate = until.Unix()
	}
}

// WithBanDuration sets the ban duration from now.
func WithBanDuration(d time.Duration) BanOption {
	return func(r *BanChatMemberRequest) {
		r.UntilDate = time.Now().Add(d).Unix()
	}
}

// WithRevokeMessages revokes all messages from the user in the chat.
func WithRevokeMessages() BanOption {
	return func(r *BanChatMemberRequest) {
		r.RevokeMessages = true
	}
}

// UnbanOption configures UnbanChatMember.
type UnbanOption func(*UnbanChatMemberRequest)

// WithOnlyIfBanned only unbans the user if they are currently banned.
func WithOnlyIfBanned() UnbanOption {
	return func(r *UnbanChatMemberRequest) {
		r.OnlyIfBanned = true
	}
}

// RestrictOption configures RestrictChatMember.
type RestrictOption func(*RestrictChatMemberRequest)

// WithRestrictUntil sets the restriction expiration time.
func WithRestrictUntil(until time.Time) RestrictOption {
	return func(r *RestrictChatMemberRequest) {
		r.UntilDate = until.Unix()
	}
}

// WithRestrictDuration sets the restriction duration from now.
func WithRestrictDuration(d time.Duration) RestrictOption {
	return func(r *RestrictChatMemberRequest) {
		r.UntilDate = time.Now().Add(d).Unix()
	}
}

// WithIndependentPermissions uses independent chat permissions.
func WithIndependentPermissions() RestrictOption {
	return func(r *RestrictChatMemberRequest) {
		r.UseIndependentChatPermissions = true
	}
}
