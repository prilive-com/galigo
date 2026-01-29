package tg

// ForumTopic represents a forum topic.
type ForumTopic struct {
	MessageThreadID   int    `json:"message_thread_id"`
	Name              string `json:"name"`
	IconColor         int    `json:"icon_color"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// Forum topic icon colors (official Telegram colors).
const (
	ForumColorBlue   = 0x6FB9F0 // 7322096
	ForumColorYellow = 0xFFD67E // 16766590
	ForumColorViolet = 0xCB86DB // 13338331
	ForumColorGreen  = 0x8EEE98 // 9367192
	ForumColorRose   = 0xFF93B2 // 16749490
	ForumColorRed    = 0xFB6F5F // 16478047
)

// ForumTopicCreated represents a service message about a new forum topic.
type ForumTopicCreated struct {
	Name              string `json:"name"`
	IconColor         int    `json:"icon_color"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// ForumTopicEdited represents a service message about an edited topic.
type ForumTopicEdited struct {
	Name              string `json:"name,omitempty"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// ForumTopicClosed represents a service message about a closed topic.
type ForumTopicClosed struct{}

// ForumTopicReopened represents a service message about a reopened topic.
type ForumTopicReopened struct{}

// GeneralForumTopicHidden represents a service message about hidden General topic.
type GeneralForumTopicHidden struct{}

// GeneralForumTopicUnhidden represents a service message about unhidden General topic.
type GeneralForumTopicUnhidden struct{}
