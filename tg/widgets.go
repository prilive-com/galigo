// RED STUB (B-65 65e step 8): every new field is tagged json:"-" and the constants are "x"; replaced by the real change.
package tg

// Bot API 10.4 widgets, read side: Message.Widget, ExternalReplyInfo.Widget, Update.WidgetQuery and
// Update.SharedWidgetQuery.

// Widget describes an HTML widget in a message (Bot API 10.4).
type Widget struct {
	ID         string `json:"-"` // the shared widget's id, for sendSharedWidget; empty if not shared
	Width      int    `json:"-"` // in logical pixels; 0: the whole width of the message bubble
	Height     int    `json:"-"` // in logical pixels; 0: the height is not fixed
	HTML       string `json:"-"` // the widget's HTML code
	PublicData string `json:"-"` // shown wherever the widget is shown; the same for every message with a shared widget
	ChatData   string `json:"-"` // specific to the message
}

// Widget query types: the values of WidgetQueryType.Type.
const (
	WidgetQueryTypeData            = "x" // WidgetQueryTypeData: data for the bot
	WidgetQueryTypeSignData        = "x" // WidgetQueryTypeSignData: data signed by a TON wallet
	WidgetQueryTypeSentTransaction = "x" // WidgetQueryTypeSentTransaction: a transaction sent from a TON wallet
)

// WidgetQueryType is the type of a query from a widget (Bot API 10.4). It is one of the spec's WidgetQueryTypeData
// ("data"), WidgetQueryTypeSignData ("sign_data") and WidgetQueryTypeSentTransaction ("sent_transaction"); Type says
// which, and the server sends only that type's fields. encoding/json fills the fields by JSON key (matched without
// regard to case) and ignores other keys: a type added in a later Bot API version keeps its Type and any of these keys
// it sends, and its other keys are dropped. A non-null value that does not fit its field, at any depth (a string for
// wallet, a number for wallet.address), makes the decode fail; null does not. SignData and MsgHash are omitted when
// empty, also in the type that requires them.
type WidgetQueryType struct {
	Type       string             `json:"-"` // "data", "sign_data" or "sent_transaction"
	WalletCard *TonWalletCard     `json:"-"` // "data": the sender's TON wallet card, if it was sent with the query
	Wallet     *WidgetQueryWallet `json:"-"` // "sign_data": the wallet that signed; "sent_transaction": the wallet that sent
	SignData   string             `json:"-"` // "sign_data": the JSON-serialized signed data; signed for the domain bot<bot_id>
	MsgHash    string             `json:"-"` // "sent_transaction": the msg_hash of the transaction
}

// WidgetQueryWallet is a user's TON wallet used in a query from a widget (Bot API 10.4).
type WidgetQueryWallet struct {
	Address   string `json:"-"`
	Amount    string `json:"-"` // balance in the smallest units, as a decimal number
	PublicKey string `json:"-"` // hex; empty if unknown
	StateInit string `json:"-"` // base64 StateInit of the wallet contract; empty if and only if PublicKey is empty
}

// WidgetQuery is a query from a widget in a message sent by the bot or via the bot (Bot API 10.4). Answer it with
// answerWidgetQuery. A user can send at most 5 queries per 5 seconds to the bot from all its widgets.
type WidgetQuery struct {
	ID              string          `json:"-"`
	From            *User           `json:"-"`
	Chat            *Chat           `json:"-"` // nil if the widget is in an inline message
	MessageID       int             `json:"-"` // 0 if the widget is in an inline message
	InlineMessageID string          `json:"-"` // for a widget in an inline message
	ChatInstance    string          `json:"-"`
	ChatType        string          `json:"-"` // inline messages: "sender", "private", "group", "supergroup" or "channel"
	Type            WidgetQueryType `json:"-"`
	Data            string          `json:"-"` // 1-64 bytes
	WidgetID        string          `json:"-"` // the shared widget's id; empty if the widget is not shared
}

// SharedWidgetQuery is a query from a forwarded message with a shared widget created by the bot (Bot API 10.4).
// Answer it with answerWidgetQuery. A user can send at most 5 queries per 5 seconds to the bot from all its widgets.
type SharedWidgetQuery struct {
	ID       string          `json:"-"`
	From     *User           `json:"-"`
	Type     WidgetQueryType `json:"-"`
	Data     string          `json:"-"` // 1-64 bytes
	WidgetID string          `json:"-"`
}
