package tg

// ParseMode defines the text formatting mode for messages.
type ParseMode string

// Supported parse modes.
const (
	ParseModeHTML       ParseMode = "HTML"
	ParseModeMarkdown   ParseMode = "Markdown"
	ParseModeMarkdownV2 ParseMode = "MarkdownV2"
)

// String returns the parse mode string value.
func (p ParseMode) String() string {
	return string(p)
}

// IsValid returns true if the parse mode is supported by Telegram.
func (p ParseMode) IsValid() bool {
	switch p {
	case ParseModeHTML, ParseModeMarkdown, ParseModeMarkdownV2, "":
		return true
	default:
		return false
	}
}

// ChatType represents the type of a Telegram chat.
type ChatType string

// Supported chat types.
const (
	ChatTypePrivate    ChatType = "private"
	ChatTypeGroup      ChatType = "group"
	ChatTypeSupergroup ChatType = "supergroup"
	ChatTypeChannel    ChatType = "channel"
)

// String returns the chat type string value.
func (c ChatType) String() string {
	return string(c)
}

// IsGroup returns true if the chat type is a group or supergroup.
func (c ChatType) IsGroup() bool {
	return c == ChatTypeGroup || c == ChatTypeSupergroup
}
