package tg

// ManagedBotCreated is a service message: a user created a bot that this bot manages. Added in Bot API 9.6.
type ManagedBotCreated struct {
	Bot *User `json:"bot"`
}

// ManagedBotUpdated is an update: a managed bot was created or its token or owner changed. Added in Bot API 9.6.
type ManagedBotUpdated struct {
	User *User `json:"user"` // the user who owns the bot
	Bot  *User `json:"bot"`  // the managed bot
}
