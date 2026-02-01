package tg

// BusinessConnection represents a connection with a business account.
type BusinessConnection struct {
	ID         string            `json:"id"`
	User       User              `json:"user"`
	UserChatID int64             `json:"user_chat_id"`
	Date       int64             `json:"date"`
	Rights     BusinessBotRights `json:"rights"`
	IsEnabled  bool              `json:"is_enabled"`
}

// BusinessBotRights describes the rights of a bot in a business account.
type BusinessBotRights struct {
	CanReply                   bool `json:"can_reply"`
	CanReadMessages            bool `json:"can_read_messages"`
	CanDeleteAllMessages       bool `json:"can_delete_all_messages"`
	CanEditName                bool `json:"can_edit_name"`
	CanEditBio                 bool `json:"can_edit_bio"`
	CanEditProfilePhoto        bool `json:"can_edit_profile_photo"`
	CanEditUsername            bool `json:"can_edit_username"`
	CanChangeGiftSettings      bool `json:"can_change_gift_settings"`
	CanViewGiftsAndStars       bool `json:"can_view_gifts_and_stars"`
	CanConvertGiftsToStars     bool `json:"can_convert_gifts_to_stars"`
	CanTransferAndUpgradeGifts bool `json:"can_transfer_and_upgrade_gifts"`
	CanTransferStars           bool `json:"can_transfer_stars"`
	CanManageStories           bool `json:"can_manage_stories"`
}

// Story represents a story posted to a chat.
type Story struct {
	ID   int   `json:"id"`
	Chat Chat  `json:"chat"`
	Date int64 `json:"date"`
}
