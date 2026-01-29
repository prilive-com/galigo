package tg

// ChatFullInfo contains full information about a chat.
// Returned by the getChat method.
type ChatFullInfo struct {
	// Basic info (always present)
	ID        int64  `json:"id"`
	Type      string `json:"type"` // "private", "group", "supergroup", "channel"
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`

	// Optional fields
	IsForum                            bool              `json:"is_forum,omitempty"`
	AccentColorID                      int               `json:"accent_color_id,omitempty"`
	MaxReactionCount                   int               `json:"max_reaction_count,omitempty"`
	Photo                              *ChatPhoto        `json:"photo,omitempty"`
	ActiveUsernames                    []string          `json:"active_usernames,omitempty"`
	Birthdate                          *Birthdate        `json:"birthdate,omitempty"`
	BusinessIntro                      *BusinessIntro    `json:"business_intro,omitempty"`
	BusinessLocation                   *BusinessLocation `json:"business_location,omitempty"`
	BusinessOpeningHours               *BusinessHours    `json:"business_opening_hours,omitempty"`
	PersonalChat                       *Chat             `json:"personal_chat,omitempty"`
	AvailableReactions                 []ReactionType    `json:"available_reactions,omitempty"`
	BackgroundCustomEmojiID            string            `json:"background_custom_emoji_id,omitempty"`
	ProfileAccentColorID               *int              `json:"profile_accent_color_id,omitempty"`
	ProfileBackgroundCustomEmojiID     string            `json:"profile_background_custom_emoji_id,omitempty"`
	EmojiStatusCustomEmojiID           string            `json:"emoji_status_custom_emoji_id,omitempty"`
	EmojiStatusExpirationDate          int64             `json:"emoji_status_expiration_date,omitempty"`
	Bio                                string            `json:"bio,omitempty"`
	HasPrivateForwards                 bool              `json:"has_private_forwards,omitempty"`
	HasRestrictedVoiceAndVideoMessages bool              `json:"has_restricted_voice_and_video_messages,omitempty"`
	JoinToSendMessages                 bool              `json:"join_to_send_messages,omitempty"`
	JoinByRequest                      bool              `json:"join_by_request,omitempty"`
	Description                        string            `json:"description,omitempty"`
	InviteLink                         string            `json:"invite_link,omitempty"`
	PinnedMessage                      *Message          `json:"pinned_message,omitempty"`
	Permissions                        *ChatPermissions  `json:"permissions,omitempty"`
	CanSendPaidMedia                   bool              `json:"can_send_paid_media,omitempty"`
	SlowModeDelay                      int               `json:"slow_mode_delay,omitempty"`
	UnrestrictBoostCount               int               `json:"unrestrict_boost_count,omitempty"`
	MessageAutoDeleteTime              int               `json:"message_auto_delete_time,omitempty"`
	HasAggressiveAntiSpamEnabled       bool              `json:"has_aggressive_anti_spam_enabled,omitempty"`
	HasHiddenMembers                   bool              `json:"has_hidden_members,omitempty"`
	HasProtectedContent                bool              `json:"has_protected_content,omitempty"`
	HasVisibleHistory                  bool              `json:"has_visible_history,omitempty"`
	StickerSetName                     string            `json:"sticker_set_name,omitempty"`
	CanSetStickerSet                   bool              `json:"can_set_sticker_set,omitempty"`
	CustomEmojiStickerSetName          string            `json:"custom_emoji_sticker_set_name,omitempty"`
	LinkedChatID                       int64             `json:"linked_chat_id,omitempty"`
	Location                           *ChatLocation     `json:"location,omitempty"`
}

// ChatLocation represents a location to which a chat is connected.
type ChatLocation struct {
	Location *Location `json:"location"`
	Address  string    `json:"address"`
}

// Birthdate represents a user's birthdate.
type Birthdate struct {
	Day   int  `json:"day"`
	Month int  `json:"month"`
	Year  *int `json:"year,omitempty"`
}

// BusinessIntro represents a business intro.
type BusinessIntro struct {
	Title   string   `json:"title,omitempty"`
	Message string   `json:"message,omitempty"`
	Sticker *Sticker `json:"sticker,omitempty"`
}

// BusinessLocation represents a business location.
type BusinessLocation struct {
	Address  string    `json:"address"`
	Location *Location `json:"location,omitempty"`
}

// BusinessHours represents business opening hours.
type BusinessHours struct {
	TimeZoneName string                         `json:"time_zone_name"`
	OpeningHours []BusinessOpeningHoursInterval `json:"opening_hours"`
}

// BusinessOpeningHoursInterval represents a time interval.
type BusinessOpeningHoursInterval struct {
	OpeningMinute int `json:"opening_minute"`
	ClosingMinute int `json:"closing_minute"`
}

// ReactionType represents a reaction type.
type ReactionType struct {
	Type        string `json:"type"` // "emoji" or "custom_emoji"
	Emoji       string `json:"emoji,omitempty"`
	CustomEmoji string `json:"custom_emoji_id,omitempty"`
}
