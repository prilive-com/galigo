package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Admin Promotion Requests ==================

// PromoteChatMemberRequest represents a promoteChatMember request.
type PromoteChatMemberRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	UserID              int64     `json:"user_id"`
	IsAnonymous         *bool     `json:"is_anonymous,omitempty"`
	CanManageChat       *bool     `json:"can_manage_chat,omitempty"`
	CanDeleteMessages   *bool     `json:"can_delete_messages,omitempty"`
	CanManageVideoChats *bool     `json:"can_manage_video_chats,omitempty"`
	CanRestrictMembers  *bool     `json:"can_restrict_members,omitempty"`
	CanPromoteMembers   *bool     `json:"can_promote_members,omitempty"`
	CanChangeInfo       *bool     `json:"can_change_info,omitempty"`
	CanInviteUsers      *bool     `json:"can_invite_users,omitempty"`
	CanPostMessages     *bool     `json:"can_post_messages,omitempty"`
	CanEditMessages     *bool     `json:"can_edit_messages,omitempty"`
	CanPinMessages      *bool     `json:"can_pin_messages,omitempty"`
	CanPostStories      *bool     `json:"can_post_stories,omitempty"`
	CanEditStories      *bool     `json:"can_edit_stories,omitempty"`
	CanDeleteStories    *bool     `json:"can_delete_stories,omitempty"`
	CanManageTopics     *bool     `json:"can_manage_topics,omitempty"`
}

// SetChatAdministratorCustomTitleRequest represents a setChatAdministratorCustomTitle request.
type SetChatAdministratorCustomTitleRequest struct {
	ChatID      tg.ChatID `json:"chat_id"`
	UserID      int64     `json:"user_id"`
	CustomTitle string    `json:"custom_title"`
}

// ================== Admin Methods ==================

// PromoteChatMember promotes or demotes a user in a supergroup or channel.
// Pass all boolean parameters as false to demote a user.
func (c *Client) PromoteChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...PromoteOption) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}

	req := PromoteChatMemberRequest{
		ChatID: chatID,
		UserID: userID,
	}
	for _, opt := range opts {
		opt(&req)
	}

	return c.callJSON(ctx, "promoteChatMember", req, nil)
}

// PromoteChatMemberWithRights promotes a user with the given rights.
// This is a convenience method that applies ChatAdministratorRights.
func (c *Client) PromoteChatMemberWithRights(ctx context.Context, chatID tg.ChatID, userID int64, rights tg.ChatAdministratorRights) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}

	req := PromoteChatMemberRequest{
		ChatID:              chatID,
		UserID:              userID,
		IsAnonymous:         &rights.IsAnonymous,
		CanManageChat:       &rights.CanManageChat,
		CanDeleteMessages:   &rights.CanDeleteMessages,
		CanManageVideoChats: &rights.CanManageVideoChats,
		CanRestrictMembers:  &rights.CanRestrictMembers,
		CanPromoteMembers:   &rights.CanPromoteMembers,
		CanChangeInfo:       &rights.CanChangeInfo,
		CanInviteUsers:      &rights.CanInviteUsers,
		CanPostMessages:     rights.CanPostMessages,
		CanEditMessages:     rights.CanEditMessages,
		CanPinMessages:      rights.CanPinMessages,
		CanPostStories:      rights.CanPostStories,
		CanEditStories:      rights.CanEditStories,
		CanDeleteStories:    rights.CanDeleteStories,
		CanManageTopics:     rights.CanManageTopics,
	}

	return c.callJSON(ctx, "promoteChatMember", req, nil)
}

// DemoteChatMember removes all admin privileges from a user.
func (c *Client) DemoteChatMember(ctx context.Context, chatID tg.ChatID, userID int64) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}

	f := false
	req := PromoteChatMemberRequest{
		ChatID:              chatID,
		UserID:              userID,
		CanManageChat:       &f,
		CanDeleteMessages:   &f,
		CanManageVideoChats: &f,
		CanRestrictMembers:  &f,
		CanPromoteMembers:   &f,
		CanChangeInfo:       &f,
		CanInviteUsers:      &f,
		CanPostMessages:     &f,
		CanEditMessages:     &f,
		CanPinMessages:      &f,
	}

	return c.callJSON(ctx, "promoteChatMember", req, nil)
}

// SetChatAdministratorCustomTitle sets a custom title for an administrator.
// Max length: 16 characters, emoji are not allowed.
func (c *Client) SetChatAdministratorCustomTitle(ctx context.Context, chatID tg.ChatID, userID int64, customTitle string) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}
	if len(customTitle) > 16 {
		return tg.NewValidationError("custom_title", "must be at most 16 characters")
	}

	return c.callJSON(ctx, "setChatAdministratorCustomTitle", SetChatAdministratorCustomTitleRequest{
		ChatID:      chatID,
		UserID:      userID,
		CustomTitle: customTitle,
	}, nil)
}

// ================== Options ==================

// PromoteOption configures PromoteChatMember.
type PromoteOption func(*PromoteChatMemberRequest)

// WithAnonymous sets whether the admin's presence is hidden.
func WithAnonymous(anonymous bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.IsAnonymous = &anonymous
	}
}

// WithCanManageChat grants ability to access chat settings.
func WithCanManageChat(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanManageChat = &can
	}
}

// WithCanDeleteMessages grants ability to delete messages.
func WithCanDeleteMessages(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanDeleteMessages = &can
	}
}

// WithCanManageVideoChats grants ability to manage video chats.
func WithCanManageVideoChats(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanManageVideoChats = &can
	}
}

// WithCanRestrictMembers grants ability to restrict members.
func WithCanRestrictMembers(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanRestrictMembers = &can
	}
}

// WithCanPromoteMembers grants ability to add new admins.
func WithCanPromoteMembers(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanPromoteMembers = &can
	}
}

// WithCanChangeInfo grants ability to change chat info.
func WithCanChangeInfo(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanChangeInfo = &can
	}
}

// WithCanInviteUsers grants ability to invite users.
func WithCanInviteUsers(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanInviteUsers = &can
	}
}

// WithCanPostMessages grants ability to post in channels.
func WithCanPostMessages(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanPostMessages = &can
	}
}

// WithCanEditMessages grants ability to edit messages in channels.
func WithCanEditMessages(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanEditMessages = &can
	}
}

// WithCanPinMessages grants ability to pin messages.
func WithCanPinMessages(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanPinMessages = &can
	}
}

// WithCanManageTopics grants ability to manage forum topics.
func WithCanManageTopics(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanManageTopics = &can
	}
}
