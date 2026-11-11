package tg

// MessageOrigin describes the origin of a message (Bot API 7.0). It is one of the spec's
// MessageOriginUser ("user"), MessageOriginHiddenUser ("hidden_user"), MessageOriginChat ("chat")
// and MessageOriginChannel ("channel"); Type says which, and only that variant's fields are set.
type MessageOrigin struct {
	Type            string `json:"type"`                       // "user", "hidden_user", "chat" or "channel"
	Date            int64  `json:"date"`                       // the date the message was sent originally, Unix time
	SenderUser      *User  `json:"sender_user,omitempty"`      // "user": the user that sent the message originally
	SenderUserName  string `json:"sender_user_name,omitempty"` // "hidden_user": the name of the user that sent it
	SenderChat      *Chat  `json:"sender_chat,omitempty"`      // "chat": the chat that sent the message originally
	AuthorSignature string `json:"author_signature,omitempty"` // "chat", "channel": the original author's signature
	Chat            *Chat  `json:"chat,omitempty"`             // "channel": the channel the message was sent to
	MessageID       int    `json:"message_id,omitempty"`       // "channel": the message id inside that channel
}

// TextQuote is the quoted part of the message that a message replies to (Bot API 7.0).
// The 10.4 field rich_texts is not supported yet.
type TextQuote struct {
	Text     string          `json:"text"` // may be empty if the quote was chosen from a rich message
	Entities []MessageEntity `json:"entities,omitempty"`
	Position int             `json:"position"` // approximate position in the original message, in UTF-16 code units
	IsManual bool            `json:"is_manual,omitempty"`
}

// ExternalReplyInfo describes the message a message replies to, which may come from another chat
// or forum topic (Bot API 7.0). The field widget (10.4) is not supported yet.
// Story and Checklist use the existing tg.Story and tg.Checklist types, which do not yet match the spec in
// every field.
type ExternalReplyInfo struct {
	Origin             MessageOrigin       `json:"origin"`
	Chat               *Chat               `json:"chat,omitempty"`       // only for a supergroup or a channel
	MessageID          int                 `json:"message_id,omitempty"` // only for a supergroup or a channel
	LinkPreviewOptions *LinkPreviewOptions `json:"link_preview_options,omitempty"`
	Animation          *Animation          `json:"animation,omitempty"`
	Audio              *Audio              `json:"audio,omitempty"`
	Document           *Document           `json:"document,omitempty"`
	LivePhoto          *LivePhoto          `json:"live_photo,omitempty"` // 10.0
	PaidMedia          *PaidMediaInfo      `json:"paid_media,omitempty"` // 7.6
	Photo              []PhotoSize         `json:"photo,omitempty"`
	Sticker            *Sticker            `json:"sticker,omitempty"`
	Story              *Story              `json:"story,omitempty"`
	Video              *Video              `json:"video,omitempty"`
	VideoNote          *VideoNote          `json:"video_note,omitempty"`
	Voice              *Voice              `json:"voice,omitempty"`
	HasMediaSpoiler    bool                `json:"has_media_spoiler,omitempty"`
	Checklist          *Checklist          `json:"checklist,omitempty"`
	Contact            *Contact            `json:"contact,omitempty"`
	Dice               *Dice               `json:"dice,omitempty"`
	Game               *Game               `json:"game,omitempty"`
	Giveaway           *Giveaway           `json:"giveaway,omitempty"`
	GiveawayWinners    *GiveawayWinners    `json:"giveaway_winners,omitempty"`
	Invoice            *Invoice            `json:"invoice,omitempty"`
	Location           *Location           `json:"location,omitempty"`
	Poll               *Poll               `json:"poll,omitempty"`
	Venue              *Venue              `json:"venue,omitempty"`
}

// Giveaway is a message about a scheduled giveaway (Bot API 7.0).
type Giveaway struct {
	Chats                         []Chat   `json:"chats"`                  // the chats a user must join to take part
	WinnersSelectionDate          int64    `json:"winners_selection_date"` // Unix time
	WinnerCount                   int      `json:"winner_count"`
	OnlyNewMembers                bool     `json:"only_new_members,omitempty"`
	HasPublicWinners              bool     `json:"has_public_winners,omitempty"`
	PrizeDescription              string   `json:"prize_description,omitempty"`
	CountryCodes                  []string `json:"country_codes,omitempty"`    // ISO 3166-1 alpha-2; empty: all countries
	PrizeStarCount                int      `json:"prize_star_count,omitempty"` // Telegram Star giveaways only
	PremiumSubscriptionMonthCount int      `json:"premium_subscription_month_count,omitempty"`
}

// GiveawayWinners is a message about a completed giveaway with public winners (Bot API 7.0).
type GiveawayWinners struct {
	Chat                          Chat   `json:"chat"` // the chat that created the giveaway
	GiveawayMessageID             int    `json:"giveaway_message_id"`
	WinnersSelectionDate          int64  `json:"winners_selection_date"` // Unix time
	WinnerCount                   int    `json:"winner_count"`
	Winners                       []User `json:"winners"` // up to 100
	AdditionalChatCount           int    `json:"additional_chat_count,omitempty"`
	PrizeStarCount                int    `json:"prize_star_count,omitempty"`
	PremiumSubscriptionMonthCount int    `json:"premium_subscription_month_count,omitempty"`
	UnclaimedPrizeCount           int    `json:"unclaimed_prize_count,omitempty"`
	OnlyNewMembers                bool   `json:"only_new_members,omitempty"`
	WasRefunded                   bool   `json:"was_refunded,omitempty"`
	PrizeDescription              string `json:"prize_description,omitempty"`
}
