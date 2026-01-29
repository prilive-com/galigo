package tg

// ChatPermissions describes actions that a non-administrator user is allowed to take in a chat.
// Pointer fields distinguish between "not set" (nil) and "explicitly false" (*false).
type ChatPermissions struct {
	CanSendMessages       *bool `json:"can_send_messages,omitempty"`
	CanSendAudios         *bool `json:"can_send_audios,omitempty"`
	CanSendDocuments      *bool `json:"can_send_documents,omitempty"`
	CanSendPhotos         *bool `json:"can_send_photos,omitempty"`
	CanSendVideos         *bool `json:"can_send_videos,omitempty"`
	CanSendVideoNotes     *bool `json:"can_send_video_notes,omitempty"`
	CanSendVoiceNotes     *bool `json:"can_send_voice_notes,omitempty"`
	CanSendPolls          *bool `json:"can_send_polls,omitempty"`
	CanSendOtherMessages  *bool `json:"can_send_other_messages,omitempty"`
	CanAddWebPagePreviews *bool `json:"can_add_web_page_previews,omitempty"`
	CanChangeInfo         *bool `json:"can_change_info,omitempty"`
	CanInviteUsers        *bool `json:"can_invite_users,omitempty"`
	CanPinMessages        *bool `json:"can_pin_messages,omitempty"`
	CanManageTopics       *bool `json:"can_manage_topics,omitempty"`
}

// boolPtr returns a pointer to a bool value.
func boolPtr(v bool) *bool { return &v }

// AllPermissions returns ChatPermissions with all permissions enabled.
func AllPermissions() ChatPermissions {
	return ChatPermissions{
		CanSendMessages:       boolPtr(true),
		CanSendAudios:         boolPtr(true),
		CanSendDocuments:      boolPtr(true),
		CanSendPhotos:         boolPtr(true),
		CanSendVideos:         boolPtr(true),
		CanSendVideoNotes:     boolPtr(true),
		CanSendVoiceNotes:     boolPtr(true),
		CanSendPolls:          boolPtr(true),
		CanSendOtherMessages:  boolPtr(true),
		CanAddWebPagePreviews: boolPtr(true),
		CanChangeInfo:         boolPtr(true),
		CanInviteUsers:        boolPtr(true),
		CanPinMessages:        boolPtr(true),
		CanManageTopics:       boolPtr(true),
	}
}

// NoPermissions returns ChatPermissions with all permissions disabled.
func NoPermissions() ChatPermissions {
	return ChatPermissions{
		CanSendMessages:       boolPtr(false),
		CanSendAudios:         boolPtr(false),
		CanSendDocuments:      boolPtr(false),
		CanSendPhotos:         boolPtr(false),
		CanSendVideos:         boolPtr(false),
		CanSendVideoNotes:     boolPtr(false),
		CanSendVoiceNotes:     boolPtr(false),
		CanSendPolls:          boolPtr(false),
		CanSendOtherMessages:  boolPtr(false),
		CanAddWebPagePreviews: boolPtr(false),
		CanChangeInfo:         boolPtr(false),
		CanInviteUsers:        boolPtr(false),
		CanPinMessages:        boolPtr(false),
		CanManageTopics:       boolPtr(false),
	}
}

// ReadOnlyPermissions returns permissions for read-only access (no sending).
func ReadOnlyPermissions() ChatPermissions {
	return ChatPermissions{
		CanSendMessages:       boolPtr(false),
		CanSendAudios:         boolPtr(false),
		CanSendDocuments:      boolPtr(false),
		CanSendPhotos:         boolPtr(false),
		CanSendVideos:         boolPtr(false),
		CanSendVideoNotes:     boolPtr(false),
		CanSendVoiceNotes:     boolPtr(false),
		CanSendPolls:          boolPtr(false),
		CanSendOtherMessages:  boolPtr(false),
		CanAddWebPagePreviews: boolPtr(false),
	}
}

// TextOnlyPermissions returns permissions for text-only messaging.
func TextOnlyPermissions() ChatPermissions {
	return ChatPermissions{
		CanSendMessages:       boolPtr(true),
		CanSendAudios:         boolPtr(false),
		CanSendDocuments:      boolPtr(false),
		CanSendPhotos:         boolPtr(false),
		CanSendVideos:         boolPtr(false),
		CanSendVideoNotes:     boolPtr(false),
		CanSendVoiceNotes:     boolPtr(false),
		CanSendPolls:          boolPtr(false),
		CanSendOtherMessages:  boolPtr(false),
		CanAddWebPagePreviews: boolPtr(false),
	}
}
