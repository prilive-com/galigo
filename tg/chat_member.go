package tg

import (
	"encoding/json"
	"fmt"
)

// ChatMember represents a member of a chat.
// This is a sealed interface — the concrete types are:
//   - ChatMemberOwner
//   - ChatMemberAdministrator
//   - ChatMemberMember
//   - ChatMemberRestricted
//   - ChatMemberLeft
//   - ChatMemberBanned
type ChatMember interface {
	// chatMember is a marker method to seal the interface.
	chatMember()

	// Status returns the member's status string.
	Status() string

	// GetUser returns the user information.
	GetUser() *User
}

// chatMemberBase contains fields common to all ChatMember types.
type chatMemberBase struct {
	User *User `json:"user"`
}

func (b chatMemberBase) GetUser() *User { return b.User }

// ChatMemberOwner represents a chat owner.
type ChatMemberOwner struct {
	chatMemberBase
	IsAnonymous bool   `json:"is_anonymous"`
	CustomTitle string `json:"custom_title,omitempty"`
}

func (ChatMemberOwner) chatMember()    {}
func (ChatMemberOwner) Status() string { return "creator" }

// ChatMemberAdministrator represents a chat administrator.
type ChatMemberAdministrator struct {
	chatMemberBase
	CanBeEdited             bool   `json:"can_be_edited"`
	IsAnonymous             bool   `json:"is_anonymous"`
	CanManageChat           bool   `json:"can_manage_chat"`
	CanDeleteMessages       bool   `json:"can_delete_messages"`
	CanManageVideoChats     bool   `json:"can_manage_video_chats"`
	CanRestrictMembers      bool   `json:"can_restrict_members"`
	CanPromoteMembers       bool   `json:"can_promote_members"`
	CanChangeInfo           bool   `json:"can_change_info"`
	CanInviteUsers          bool   `json:"can_invite_users"`
	CanPostMessages         *bool  `json:"can_post_messages,omitempty"`
	CanEditMessages         *bool  `json:"can_edit_messages,omitempty"`
	CanPinMessages          *bool  `json:"can_pin_messages,omitempty"`
	CanPostStories          *bool  `json:"can_post_stories,omitempty"`
	CanEditStories          *bool  `json:"can_edit_stories,omitempty"`
	CanDeleteStories        *bool  `json:"can_delete_stories,omitempty"`
	CanManageTopics         *bool  `json:"can_manage_topics,omitempty"`
	CanManageDirectMessages *bool  `json:"can_manage_direct_messages,omitempty"`
	CustomTitle             string `json:"custom_title,omitempty"`
}

func (ChatMemberAdministrator) chatMember()    {}
func (ChatMemberAdministrator) Status() string { return "administrator" }

// ChatMemberMember represents a regular chat member.
type ChatMemberMember struct {
	chatMemberBase
	UntilDate int64 `json:"until_date,omitempty"`
}

func (ChatMemberMember) chatMember()    {}
func (ChatMemberMember) Status() string { return "member" }

// ChatMemberRestricted represents a restricted user.
type ChatMemberRestricted struct {
	chatMemberBase
	IsMember              bool  `json:"is_member"`
	CanSendMessages       bool  `json:"can_send_messages"`
	CanSendAudios         bool  `json:"can_send_audios"`
	CanSendDocuments      bool  `json:"can_send_documents"`
	CanSendPhotos         bool  `json:"can_send_photos"`
	CanSendVideos         bool  `json:"can_send_videos"`
	CanSendVideoNotes     bool  `json:"can_send_video_notes"`
	CanSendVoiceNotes     bool  `json:"can_send_voice_notes"`
	CanSendPolls          bool  `json:"can_send_polls"`
	CanSendOtherMessages  bool  `json:"can_send_other_messages"`
	CanAddWebPagePreviews bool  `json:"can_add_web_page_previews"`
	CanChangeInfo         bool  `json:"can_change_info"`
	CanInviteUsers        bool  `json:"can_invite_users"`
	CanPinMessages        bool  `json:"can_pin_messages"`
	CanManageTopics       bool  `json:"can_manage_topics"`
	UntilDate             int64 `json:"until_date"`
}

func (ChatMemberRestricted) chatMember()    {}
func (ChatMemberRestricted) Status() string { return "restricted" }

// ChatMemberLeft represents a user who left the chat.
type ChatMemberLeft struct {
	chatMemberBase
}

func (ChatMemberLeft) chatMember()    {}
func (ChatMemberLeft) Status() string { return "left" }

// ChatMemberBanned represents a banned user.
type ChatMemberBanned struct {
	chatMemberBase
	UntilDate int64 `json:"until_date"`
}

func (ChatMemberBanned) chatMember()    {}
func (ChatMemberBanned) Status() string { return "kicked" }

// UnmarshalChatMember deserializes JSON into the correct ChatMember concrete type.
func UnmarshalChatMember(data []byte) (ChatMember, error) {
	var probe struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("failed to probe chat member status: %w", err)
	}

	var result ChatMember
	var err error

	switch probe.Status {
	case "creator":
		var m ChatMemberOwner
		err = json.Unmarshal(data, &m)
		result = m
	case "administrator":
		var m ChatMemberAdministrator
		err = json.Unmarshal(data, &m)
		result = m
	case "member":
		var m ChatMemberMember
		err = json.Unmarshal(data, &m)
		result = m
	case "restricted":
		var m ChatMemberRestricted
		err = json.Unmarshal(data, &m)
		result = m
	case "left":
		var m ChatMemberLeft
		err = json.Unmarshal(data, &m)
		result = m
	case "kicked":
		var m ChatMemberBanned
		err = json.Unmarshal(data, &m)
		result = m
	default:
		return nil, fmt.Errorf("unknown chat member status: %q", probe.Status)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal chat member (%s): %w", probe.Status, err)
	}
	return result, nil
}

// IsOwner returns true if the member is the chat owner.
func IsOwner(m ChatMember) bool {
	_, ok := m.(ChatMemberOwner)
	return ok
}

// IsAdmin returns true if the member is an administrator (including owner).
func IsAdmin(m ChatMember) bool {
	switch m.(type) {
	case ChatMemberOwner, ChatMemberAdministrator:
		return true
	default:
		return false
	}
}

// IsMember returns true if the member is a regular member.
func IsMember(m ChatMember) bool {
	_, ok := m.(ChatMemberMember)
	return ok
}

// IsRestricted returns true if the member is restricted.
func IsRestricted(m ChatMember) bool {
	_, ok := m.(ChatMemberRestricted)
	return ok
}

// IsBanned returns true if the member is banned.
func IsBanned(m ChatMember) bool {
	_, ok := m.(ChatMemberBanned)
	return ok
}

// HasLeft returns true if the member left the chat.
func HasLeft(m ChatMember) bool {
	_, ok := m.(ChatMemberLeft)
	return ok
}
