package tg

// Community is a community (a group of chats). Added in Bot API 10.2.
type Community struct {
	// ID may have more than 32 significant bits but has at most 52, so it fits int64.
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CommunityChatAdded is a service message: a chat or a bot was added to a community. Added in Bot API 10.2.
type CommunityChatAdded struct {
	Community Community `json:"community"` // the new community of the chat or the bot
}

// CommunityChatRemoved is a service message: a chat or a bot was removed from a community. It holds no
// information. Added in Bot API 10.2.
type CommunityChatRemoved struct{}
