package tg

// EphemeralMessageParameters describes an ephemeral message to send (Bot API 10.3; ReceiverUserID is optional since
// 10.4): a message that only one user and the bot can see.
//
// Set ReceiverUserID to send it to that member of the request's chat. Set CallbackQueryID without ReceiverUserID to
// send it as an ephemeral reply to that callback query, shown only to the user who pressed the button; leave the
// request's ChatID nil then (the server ignores it; a zero int or an empty string is refused locally by the methods
// that check the chat). The bot must still answer the callback query. The server checks the rest (who may receive
// the message, the 15-second window, chat kinds).
type EphemeralMessageParameters struct {
	// ReceiverUserID is required unless the message is an ephemeral reply to a callback query.
	ReceiverUserID int64 `json:"receiver_user_id,omitempty"`
	// CallbackQueryID is the callback query that triggered the message.
	CallbackQueryID string `json:"callback_query_id,omitempty"`
	// ReplaceCallbackQueryMessage shows the message in place of the original message.
	ReplaceCallbackQueryMessage bool `json:"replace_callback_query_message,omitempty"`
}

// Validate refuses parameters with neither ReceiverUserID nor CallbackQueryID: they do not describe an ephemeral
// message (sendMessage was seen to send a normal message, visible to every chat member, for them). A nil value is
// valid (not an ephemeral message).
func (p *EphemeralMessageParameters) Validate() error {
	if p != nil && p.ReceiverUserID == 0 && p.CallbackQueryID == "" {
		return NewValidationError("ephemeral_message_parameters", "receiver_user_id or callback_query_id required")
	}
	return nil
}
