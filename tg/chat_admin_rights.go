package tg

// ChatAdministratorRights represents the rights of an administrator in a chat.
type ChatAdministratorRights struct {
	IsAnonymous             bool  `json:"is_anonymous"`
	CanManageChat           bool  `json:"can_manage_chat"`
	CanDeleteMessages       bool  `json:"can_delete_messages"`
	CanManageVideoChats     bool  `json:"can_manage_video_chats"`
	CanRestrictMembers      bool  `json:"can_restrict_members"`
	CanPromoteMembers       bool  `json:"can_promote_members"`
	CanChangeInfo           bool  `json:"can_change_info"`
	CanInviteUsers          bool  `json:"can_invite_users"`
	CanPostMessages         *bool `json:"can_post_messages,omitempty"`
	CanEditMessages         *bool `json:"can_edit_messages,omitempty"`
	CanPinMessages          *bool `json:"can_pin_messages,omitempty"`
	CanPostStories          *bool `json:"can_post_stories,omitempty"`
	CanEditStories          *bool `json:"can_edit_stories,omitempty"`
	CanDeleteStories        *bool `json:"can_delete_stories,omitempty"`
	CanManageTopics         *bool `json:"can_manage_topics,omitempty"`
	CanManageDirectMessages *bool `json:"can_manage_direct_messages,omitempty"`
}

// FullAdminRights returns administrator rights with all permissions enabled.
func FullAdminRights() ChatAdministratorRights {
	return ChatAdministratorRights{
		IsAnonymous:             false,
		CanManageChat:           true,
		CanDeleteMessages:       true,
		CanManageVideoChats:     true,
		CanRestrictMembers:      true,
		CanPromoteMembers:       true,
		CanChangeInfo:           true,
		CanInviteUsers:          true,
		CanPostMessages:         boolPtr(true),
		CanEditMessages:         boolPtr(true),
		CanPinMessages:          boolPtr(true),
		CanPostStories:          boolPtr(true),
		CanEditStories:          boolPtr(true),
		CanDeleteStories:        boolPtr(true),
		CanManageTopics:         boolPtr(true),
		CanManageDirectMessages: boolPtr(true),
	}
}

// ModeratorRights returns typical moderator permissions (no promote, no change info).
func ModeratorRights() ChatAdministratorRights {
	return ChatAdministratorRights{
		CanManageChat:      true,
		CanDeleteMessages:  true,
		CanRestrictMembers: true,
		CanInviteUsers:     true,
		CanPinMessages:     boolPtr(true),
	}
}

// ContentManagerRights returns permissions for content management only.
func ContentManagerRights() ChatAdministratorRights {
	return ChatAdministratorRights{
		CanManageChat:     true,
		CanDeleteMessages: true,
		CanChangeInfo:     true,
		CanPostMessages:   boolPtr(true),
		CanEditMessages:   boolPtr(true),
		CanPinMessages:    boolPtr(true),
	}
}
