package tg

// Bot API 10.4 widgets, read side: Message.Widget, ExternalReplyInfo.Widget, Update.WidgetQuery and
// Update.SharedWidgetQuery.

// Widget describes an HTML widget in a message (Bot API 10.4).
type Widget struct {
	ID         string `json:"id,omitempty"`          // the shared widget's id, for sendSharedWidget; empty if not shared
	Width      int    `json:"width,omitempty"`       // in logical pixels; 0: the whole width of the message bubble
	Height     int    `json:"height,omitempty"`      // in logical pixels; 0: the height is not fixed
	HTML       string `json:"html"`                  // the widget's HTML code
	PublicData string `json:"public_data,omitempty"` // available wherever the widget is shown; the same for every message with a shared widget
	ChatData   string `json:"chat_data,omitempty"`   // specific to the message
}

// Widget query types: the values of WidgetQueryType.Type.
const (
	WidgetQueryTypeData            = "data"             // WidgetQueryTypeData: data for the bot
	WidgetQueryTypeSignData        = "sign_data"        // WidgetQueryTypeSignData: data signed by a TON wallet
	WidgetQueryTypeSentTransaction = "sent_transaction" // WidgetQueryTypeSentTransaction: a transaction sent from a TON wallet
)

// WidgetQueryType is the type of a query from a widget (Bot API 10.4). It is one of the spec's WidgetQueryTypeData
// ("data"), WidgetQueryTypeSignData ("sign_data") and WidgetQueryTypeSentTransaction ("sent_transaction"); Type says
// which, and the server sends only that type's fields. encoding/json fills the fields by JSON key (matched without
// regard to case) and ignores other keys: a type added in a later Bot API version keeps its Type and any of these keys
// it sends, and its other keys are dropped. A non-null value that does not fit its field, at any depth (a string for
// wallet, a number for wallet.address), makes the decode fail; null does not. SignData and MsgHash are omitted when
// empty, also in the type that requires them.
type WidgetQueryType struct {
	Type       string             `json:"type"`                  // "data", "sign_data" or "sent_transaction"
	WalletCard *TonWalletCard     `json:"wallet_card,omitempty"` // "data": the sender's TON wallet card, if it was sent with the query
	Wallet     *WidgetQueryWallet `json:"wallet,omitempty"`      // "sign_data": the wallet that signed; "sent_transaction": the wallet that sent
	SignData   string             `json:"sign_data,omitempty"`   // "sign_data": the JSON-serialized signed data; signed for the domain bot<bot_id>
	MsgHash    string             `json:"msg_hash,omitempty"`    // "sent_transaction": the msg_hash of the transaction
}

// WidgetQueryWallet is a user's TON wallet used in a query from a widget (Bot API 10.4).
type WidgetQueryWallet struct {
	Address   string `json:"address"`
	Amount    string `json:"amount"`               // balance in the smallest units, as a decimal number
	PublicKey string `json:"public_key,omitempty"` // hex; empty if unknown
	StateInit string `json:"state_init,omitempty"` // base64 StateInit of the wallet contract; empty if and only if PublicKey is empty
}

// WidgetQuery is a query from a widget in a message sent by the bot or via the bot (Bot API 10.4). Answer it with
// answerWidgetQuery (sender.Client.AnswerWidgetQuery). A user can send at most 5 queries per 5 seconds to the bot
// from all its widgets.
type WidgetQuery struct {
	ID              string          `json:"id"`
	From            *User           `json:"from"`
	Chat            *Chat           `json:"chat,omitempty"`              // nil if the widget is in an inline message
	MessageID       int             `json:"message_id,omitempty"`        // 0 if the widget is in an inline message
	InlineMessageID string          `json:"inline_message_id,omitempty"` // for a widget in an inline message
	ChatInstance    string          `json:"chat_instance"`
	ChatType        string          `json:"chat_type,omitempty"` // inline messages: "sender", "private", "group", "supergroup" or "channel"
	Type            WidgetQueryType `json:"type"`
	Data            string          `json:"data"`                // 1-64 bytes
	WidgetID        string          `json:"widget_id,omitempty"` // the shared widget's id; empty if the widget is not shared
}

// SharedWidgetQuery is a query from a forwarded message with a shared widget created by the bot (Bot API 10.4).
// Answer it with answerWidgetQuery (sender.Client.AnswerWidgetQuery). A user can send at most 5 queries per 5 seconds
// to the bot from all its widgets.
type SharedWidgetQuery struct {
	ID       string          `json:"id"`
	From     *User           `json:"from"`
	Type     WidgetQueryType `json:"type"`
	Data     string          `json:"data"` // 1-64 bytes
	WidgetID string          `json:"widget_id"`
}
