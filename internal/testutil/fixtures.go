package testutil

import "github.com/prilive-com/galigo/tg"

// Test constants for consistent test data.
const (
	// TestToken is a valid-format bot token for testing.
	TestToken = "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"

	// TestChatID is a test chat ID.
	TestChatID = int64(123456789)

	// TestUserID is a test user ID.
	TestUserID = int64(987654321)

	// TestBotID is a test bot ID.
	TestBotID = int64(123456789)

	// TestUsername is a test username.
	TestUsername = "testuser"

	// TestBotUsername is a test bot username.
	TestBotUsername = "testbot"
)

// TestUser returns a test user fixture.
func TestUser() *tg.User {
	return &tg.User{
		ID:        TestUserID,
		IsBot:     false,
		FirstName: "Test",
		LastName:  "User",
		Username:  TestUsername,
	}
}

// TestBot returns a test bot user fixture.
func TestBot() *tg.User {
	return &tg.User{
		ID:        TestBotID,
		IsBot:     true,
		FirstName: "Test Bot",
		Username:  TestBotUsername,
	}
}

// TestChat returns a test private chat fixture.
func TestChat() *tg.Chat {
	return &tg.Chat{
		ID:        TestChatID,
		Type:      "private",
		FirstName: "Test",
		LastName:  "User",
		Username:  TestUsername,
	}
}

// TestGroupChat returns a test group chat fixture.
func TestGroupChat(id int64, title string) *tg.Chat {
	return &tg.Chat{
		ID:    id,
		Type:  "group",
		Title: title,
	}
}

// TestSuperGroupChat returns a test supergroup chat fixture.
func TestSuperGroupChat(id int64, title, username string) *tg.Chat {
	return &tg.Chat{
		ID:       id,
		Type:     "supergroup",
		Title:    title,
		Username: username,
	}
}

// TestChannelChat returns a test channel chat fixture.
func TestChannelChat(id int64, title, username string) *tg.Chat {
	return &tg.Chat{
		ID:       id,
		Type:     "channel",
		Title:    title,
		Username: username,
	}
}

// TestMessage returns a test message fixture.
func TestMessage(messageID int, text string) *tg.Message {
	return &tg.Message{
		MessageID: messageID,
		Date:      1234567890,
		Chat:      TestChat(),
		From:      TestUser(),
		Text:      text,
	}
}

// TestMessageInChat returns a test message fixture for a specific chat.
func TestMessageInChat(messageID int, chatID int64, text string) *tg.Message {
	return &tg.Message{
		MessageID: messageID,
		Date:      1234567890,
		Chat: &tg.Chat{
			ID:   chatID,
			Type: "private",
		},
		From: TestUser(),
		Text: text,
	}
}

// TestUpdate returns a test update fixture with a message.
func TestUpdate(updateID int, text string) tg.Update {
	return tg.Update{
		UpdateID: updateID,
		Message:  TestMessage(1, text),
	}
}

// TestUpdateWithMessage returns a test update fixture with a custom message.
func TestUpdateWithMessage(updateID int, msg *tg.Message) tg.Update {
	return tg.Update{
		UpdateID: updateID,
		Message:  msg,
	}
}

// TestCallbackQuery returns a test callback query fixture.
func TestCallbackQuery(id, data string) *tg.CallbackQuery {
	return &tg.CallbackQuery{
		ID:           id,
		From:         TestUser(),
		Message:      TestMessage(1, "Original message"),
		ChatInstance: "instance_123",
		Data:         data,
	}
}

// TestCallbackQueryWithMessage returns a test callback query with a custom message.
func TestCallbackQueryWithMessage(id, data string, msg *tg.Message) *tg.CallbackQuery {
	return &tg.CallbackQuery{
		ID:           id,
		From:         TestUser(),
		Message:      msg,
		ChatInstance: "instance_123",
		Data:         data,
	}
}

// TestUpdateWithCallback returns a test update fixture with a callback query.
func TestUpdateWithCallback(updateID int, cbID, cbData string) tg.Update {
	return tg.Update{
		UpdateID:      updateID,
		CallbackQuery: TestCallbackQuery(cbID, cbData),
	}
}

// TestInlineKeyboard returns a test inline keyboard fixture.
func TestInlineKeyboard(buttons ...[]tg.InlineKeyboardButton) *tg.InlineKeyboardMarkup {
	return &tg.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}
}

// TestInlineButton returns a test inline keyboard button with callback data.
func TestInlineButton(text, callbackData string) tg.InlineKeyboardButton {
	return tg.InlineKeyboardButton{
		Text:         text,
		CallbackData: callbackData,
	}
}

// TestURLButton returns a test inline keyboard button with URL.
func TestURLButton(text, url string) tg.InlineKeyboardButton {
	return tg.InlineKeyboardButton{
		Text: text,
		URL:  url,
	}
}
