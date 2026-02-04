package tg

// BotCommand represents a bot command shown in the menu.
type BotCommand struct {
	Command     string `json:"command"`     // 1-32 chars, lowercase a-z, 0-9, _
	Description string `json:"description"` // 1-256 chars
}

// BotCommandScope defines which users see specific commands.
// This is a polymorphic type with several variants.
type BotCommandScope struct {
	Type   string `json:"type"`              // Required: scope type
	ChatID ChatID `json:"chat_id,omitempty"` // For chat/chat_administrators/chat_member
	UserID int64  `json:"user_id,omitempty"` // For chat_member only
}

// BotCommandScope constructors

// BotCommandScopeDefault returns the default scope for all users.
func BotCommandScopeDefault() BotCommandScope {
	return BotCommandScope{Type: "default"}
}

// BotCommandScopeAllPrivateChats returns scope for all private chats.
func BotCommandScopeAllPrivateChats() BotCommandScope {
	return BotCommandScope{Type: "all_private_chats"}
}

// BotCommandScopeAllGroupChats returns scope for all group chats.
func BotCommandScopeAllGroupChats() BotCommandScope {
	return BotCommandScope{Type: "all_group_chats"}
}

// BotCommandScopeAllChatAdministrators returns scope for all chat administrators.
func BotCommandScopeAllChatAdministrators() BotCommandScope {
	return BotCommandScope{Type: "all_chat_administrators"}
}

// BotCommandScopeChat returns scope for a specific chat.
func BotCommandScopeChat(chatID ChatID) BotCommandScope {
	return BotCommandScope{Type: "chat", ChatID: chatID}
}

// BotCommandScopeChatAdministrators returns scope for administrators of a specific chat.
func BotCommandScopeChatAdministrators(chatID ChatID) BotCommandScope {
	return BotCommandScope{Type: "chat_administrators", ChatID: chatID}
}

// BotCommandScopeChatMember returns scope for a specific member of a chat.
func BotCommandScopeChatMember(chatID ChatID, userID int64) BotCommandScope {
	return BotCommandScope{Type: "chat_member", ChatID: chatID, UserID: userID}
}

// BotName represents the bot's display name.
type BotName struct {
	Name string `json:"name"` // 0-64 chars
}

// BotDescription represents the bot's long description (shown in empty chat).
type BotDescription struct {
	Description string `json:"description"` // 0-512 chars
}

// BotShortDescription represents the bot's short description (shown in profile/search).
type BotShortDescription struct {
	ShortDescription string `json:"short_description"` // 0-120 chars
}
