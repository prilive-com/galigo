package tg

// Bot API 10.4 chat share themes: ChatFullInfo.ShareTheme and the getChatShareThemeBackdrops result.

// EmojiType is an emoji (Bot API 10.4). Type is "emoji" (Emoji is set) or "custom_emoji" (CustomEmojiID is set).
type EmojiType struct {
	Type          string `json:"type"`
	Emoji         string `json:"emoji,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// ChatShareTheme is the theme of a chat's share page (Bot API 10.4). Type is "emoji" (Emojis, and
// BackgroundBackdropID if set) or "unique_gift" (Gift).
type ChatShareTheme struct {
	Type   string      `json:"type"`
	Emojis []EmojiType `json:"emojis,omitzero"` // omitzero: an empty list is sent, a nil list is not
	// BackgroundBackdropID is a ChatShareThemeBackdrop.ID; a pointer, because 0 is a real backdrop. Nil in an "emoji"
	// theme: the default background. A "unique_gift" theme has no backdrop id.
	BackgroundBackdropID *int        `json:"background_backdrop_id,omitempty"`
	Gift                 *UniqueGift `json:"gift,omitempty"`
}

// ChatShareThemeBackdrop is a backdrop for a chat share theme (Bot API 10.4).
type ChatShareThemeBackdrop struct {
	ID     int                      `json:"id"`
	Name   string                   `json:"name"`
	Colors UniqueGiftBackdropColors `json:"colors"`
}

// ChatShareThemeBackdrops is the result of getChatShareThemeBackdrops (Bot API 10.4).
type ChatShareThemeBackdrops struct {
	Backdrops []ChatShareThemeBackdrop `json:"backdrops"`
}
