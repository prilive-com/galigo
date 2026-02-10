package tg

import (
	"encoding/json"
	"iter"
	"strconv"
)

// InlineKeyboardMarkup represents an inline keyboard attached to a message.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// InlineKeyboardButton represents a button in an inline keyboard.
type InlineKeyboardButton struct {
	Text                         string                       `json:"text"`
	IconCustomEmojiID            string                       `json:"icon_custom_emoji_id,omitempty"` // 9.4
	Style                        string                       `json:"style,omitempty"`                // 9.4: "danger"|"success"|"primary"
	URL                          string                       `json:"url,omitempty"`
	CallbackData                 string                       `json:"callback_data,omitempty"`
	WebApp                       *WebAppInfo                  `json:"web_app,omitempty"`
	LoginURL                     *LoginURL                    `json:"login_url,omitempty"`
	SwitchInlineQuery            string                       `json:"switch_inline_query,omitempty"`
	SwitchInlineQueryCurrentChat string                       `json:"switch_inline_query_current_chat,omitempty"`
	SwitchInlineQueryChosenChat  *SwitchInlineQueryChosenChat `json:"switch_inline_query_chosen_chat,omitempty"`
	Pay                          bool                         `json:"pay,omitempty"`
}

// Button style constants for InlineKeyboardButton and KeyboardButton.
const (
	ButtonStyleDanger  = "danger"  // Red
	ButtonStyleSuccess = "success" // Green
	ButtonStylePrimary = "primary" // Blue
)

// WithStyle returns a copy of the button with the given style.
// Valid values: ButtonStyleDanger ("danger"), ButtonStyleSuccess ("success"),
// ButtonStylePrimary ("primary").
func (b InlineKeyboardButton) WithStyle(style string) InlineKeyboardButton {
	b.Style = style
	return b
}

// WithIcon returns a copy of the button with a custom emoji icon.
// The bot must be eligible to use custom emoji in the message.
func (b InlineKeyboardButton) WithIcon(customEmojiID string) InlineKeyboardButton {
	b.IconCustomEmojiID = customEmojiID
	return b
}

// WebAppInfo contains information about a Web App.
type WebAppInfo struct {
	URL string `json:"url"`
}

// LoginURL represents HTTP URL login button parameters.
type LoginURL struct {
	URL                string `json:"url"`
	ForwardText        string `json:"forward_text,omitempty"`
	BotUsername        string `json:"bot_username,omitempty"`
	RequestWriteAccess bool   `json:"request_write_access,omitempty"`
}

// SwitchInlineQueryChosenChat represents inline query switch to chosen chat.
type SwitchInlineQueryChosenChat struct {
	Query             string `json:"query,omitempty"`
	AllowUserChats    bool   `json:"allow_user_chats,omitempty"`
	AllowBotChats     bool   `json:"allow_bot_chats,omitempty"`
	AllowGroupChats   bool   `json:"allow_group_chats,omitempty"`
	AllowChannelChats bool   `json:"allow_channel_chats,omitempty"`
}

// Button constructors

// Btn creates a callback button (most common type).
func Btn(text, callbackData string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, CallbackData: callbackData}
}

// BtnURL creates a URL button.
func BtnURL(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, URL: url}
}

// BtnWebApp creates a Web App button.
func BtnWebApp(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, WebApp: &WebAppInfo{URL: url}}
}

// BtnSwitch creates an inline query switch button.
func BtnSwitch(text, query string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, SwitchInlineQuery: query}
}

// BtnSwitchCurrent creates an inline query switch button for current chat.
func BtnSwitchCurrent(text, query string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, SwitchInlineQueryCurrentChat: query}
}

// BtnLogin creates a login URL button.
func BtnLogin(text string, loginURL LoginURL) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, LoginURL: &loginURL}
}

// BtnPay creates a Pay button (must be first in first row).
func BtnPay(text string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, Pay: true}
}

// Keyboard builds inline keyboards fluently.
type Keyboard struct {
	rows [][]InlineKeyboardButton
}

// NewKeyboard creates a new keyboard builder.
func NewKeyboard() *Keyboard {
	return &Keyboard{rows: make([][]InlineKeyboardButton, 0, 4)}
}

// Row adds a row of buttons.
func (k *Keyboard) Row(buttons ...InlineKeyboardButton) *Keyboard {
	if len(buttons) > 0 {
		k.rows = append(k.rows, buttons)
	}
	return k
}

// Add appends buttons to the last row, or creates a new row if empty.
func (k *Keyboard) Add(buttons ...InlineKeyboardButton) *Keyboard {
	if len(k.rows) == 0 {
		k.rows = append(k.rows, buttons)
	} else {
		lastIdx := len(k.rows) - 1
		k.rows[lastIdx] = append(k.rows[lastIdx], buttons...)
	}
	return k
}

// Build returns the completed InlineKeyboardMarkup.
func (k *Keyboard) Build() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: k.rows}
}

// Inline returns the completed InlineKeyboardMarkup (alias for Build).
func (k *Keyboard) Inline() *InlineKeyboardMarkup {
	return k.Build()
}

// Empty returns true if keyboard has no buttons.
func (k *Keyboard) Empty() bool {
	return len(k.rows) == 0
}

// RowCount returns the number of rows.
func (k *Keyboard) RowCount() int {
	return len(k.rows)
}

// Rows returns an iterator over keyboard rows.
func (k *Keyboard) Rows() iter.Seq[[]InlineKeyboardButton] {
	return func(yield func([]InlineKeyboardButton) bool) {
		for _, row := range k.rows {
			if !yield(row) {
				return
			}
		}
	}
}

// AllButtons returns an iterator over all buttons.
func (k *Keyboard) AllButtons() iter.Seq[InlineKeyboardButton] {
	return func(yield func(InlineKeyboardButton) bool) {
		for _, row := range k.rows {
			for _, btn := range row {
				if !yield(btn) {
					return
				}
			}
		}
	}
}

// MarshalJSON implements json.Marshaler.
func (k *Keyboard) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.Build())
}

// Quick keyboard builders

// InlineKeyboard creates a keyboard from rows of buttons.
func InlineKeyboard(rows ...[]InlineKeyboardButton) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// Row creates a row of buttons (for use with InlineKeyboard).
func Row(buttons ...InlineKeyboardButton) []InlineKeyboardButton {
	return buttons
}

// Pagination creates a pagination keyboard.
func Pagination(current, total int, prefix string) *InlineKeyboardMarkup {
	k := NewKeyboard()
	var buttons []InlineKeyboardButton

	if current > 1 {
		buttons = append(buttons, Btn("« Prev", prefix+":"+strconv.Itoa(current-1)))
	}

	buttons = append(buttons, Btn(strconv.Itoa(current)+"/"+strconv.Itoa(total), prefix+":current"))

	if current < total {
		buttons = append(buttons, Btn("Next »", prefix+":"+strconv.Itoa(current+1)))
	}

	return k.Row(buttons...).Build()
}

// Confirm creates a Yes/No confirmation keyboard.
func Confirm(yesData, noData string) *InlineKeyboardMarkup {
	return NewKeyboard().
		Row(Btn("Yes", yesData), Btn("No", noData)).
		Build()
}

// ConfirmCustom creates a confirmation keyboard with custom labels.
func ConfirmCustom(yesText, yesData, noText, noData string) *InlineKeyboardMarkup {
	return NewKeyboard().
		Row(Btn(yesText, yesData), Btn(noText, noData)).
		Build()
}

// Grid creates a keyboard with buttons arranged in a grid.
func Grid[T any](items []T, columns int, btnFunc func(T) InlineKeyboardButton) *InlineKeyboardMarkup {
	k := NewKeyboard()
	var row []InlineKeyboardButton

	for i, item := range items {
		row = append(row, btnFunc(item))
		if (i+1)%columns == 0 {
			k.Row(row...)
			row = nil
		}
	}

	if len(row) > 0 {
		k.Row(row...)
	}

	return k.Build()
}

// Reply keyboard types (for custom keyboards shown below the message input)

// KeyboardButton represents one button of a reply keyboard.
type KeyboardButton struct {
	Text              string                      `json:"text"`
	IconCustomEmojiID string                      `json:"icon_custom_emoji_id,omitempty"` // 9.4
	Style             string                      `json:"style,omitempty"`                // 9.4
	RequestUsers      *KeyboardButtonRequestUsers `json:"request_users,omitempty"`
	RequestChat       *KeyboardButtonRequestChat  `json:"request_chat,omitempty"`
	RequestContact    bool                        `json:"request_contact,omitempty"`
	RequestLocation   bool                        `json:"request_location,omitempty"`
	RequestPoll       *KeyboardButtonPollType     `json:"request_poll,omitempty"`
	WebApp            *WebAppInfo                 `json:"web_app,omitempty"`
}

// KeyboardButtonRequestUsers defines criteria for requesting users.
type KeyboardButtonRequestUsers struct {
	RequestID     int   `json:"request_id"`
	UserIsBot     *bool `json:"user_is_bot,omitempty"`
	UserIsPremium *bool `json:"user_is_premium,omitempty"`
	MaxQuantity   int   `json:"max_quantity,omitempty"`
}

// KeyboardButtonRequestChat defines criteria for requesting a chat.
type KeyboardButtonRequestChat struct {
	RequestID               int                      `json:"request_id"`
	ChatIsChannel           bool                     `json:"chat_is_channel"`
	ChatIsForum             *bool                    `json:"chat_is_forum,omitempty"`
	ChatHasUsername         *bool                    `json:"chat_has_username,omitempty"`
	ChatIsCreated           bool                     `json:"chat_is_created,omitempty"`
	UserAdministratorRights *ChatAdministratorRights `json:"user_administrator_rights,omitempty"`
	BotAdministratorRights  *ChatAdministratorRights `json:"bot_administrator_rights,omitempty"`
	BotIsMember             bool                     `json:"bot_is_member,omitempty"`
}

// KeyboardButtonPollType limits polls to a specific type.
type KeyboardButtonPollType struct {
	Type string `json:"type,omitempty"` // "quiz" or "regular"
}

// ReplyKeyboardMarkup represents a custom keyboard with reply options.
type ReplyKeyboardMarkup struct {
	Keyboard              [][]KeyboardButton `json:"keyboard"`
	IsPersistent          bool               `json:"is_persistent,omitempty"`
	ResizeKeyboard        bool               `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard       bool               `json:"one_time_keyboard,omitempty"`
	InputFieldPlaceholder string             `json:"input_field_placeholder,omitempty"`
	Selective             bool               `json:"selective,omitempty"`
}

// ReplyKeyboardRemove requests removal of the custom keyboard.
type ReplyKeyboardRemove struct {
	RemoveKeyboard bool `json:"remove_keyboard"` // Always true
	Selective      bool `json:"selective,omitempty"`
}
