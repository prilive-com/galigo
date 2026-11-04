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

// BotAccessSettings describes the access settings of a managed bot. Added in Bot API 10.0.
type BotAccessSettings struct {
	IsAccessRestricted bool   `json:"is_access_restricted"`  // only selected users can access the bot; its owner always can
	AddedUsers         []User `json:"added_users,omitempty"` // the users who can access the bot when access is restricted
}
