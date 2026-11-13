package tg

// SuggestedPostPrice is the price of a suggested post (Bot API 9.2).
type SuggestedPostPrice struct {
	// Currency is "XTR" for Telegram Stars or "TON" for TON grams.
	Currency string `json:"currency"`
	// Amount is in the smallest units of the currency: Stars or nanograms.
	// The server checks the range (Stars 5-100000, nanograms 10^7-10^13);
	// int64 because 10^13 does not fit a 32-bit int.
	Amount int64 `json:"amount"`
}

// SuggestedPostParameters are the parameters of a post the bot suggests
// (Bot API 9.2). A send method takes them in suggested_post_parameters, in a
// channel direct messages chat only; the server refuses them elsewhere. A
// non-nil empty value is sent as {}: an unpaid post that can be published at
// any time within 30 days.
type SuggestedPostParameters struct {
	// Price is the proposed price; nil means an unpaid post.
	Price *SuggestedPostPrice `json:"price,omitempty"`
	// SendDate is the proposed publication time (Unix time); 0 means any time
	// within 30 days. The server checks the window: 300 to 2678400 seconds
	// ahead (the spec calls it 30 days).
	SendDate int64 `json:"send_date,omitempty"`
}

// SuggestedPostInfo is the state of a suggested post in a channel direct
// messages chat (Bot API 9.2); see Message.SuggestedPostInfo. A suggested
// post that was approved or declined can't be edited.
type SuggestedPostInfo struct {
	// State is "pending", "approved" or "declined".
	State string `json:"state"`
	// Price is the proposed price; nil means an unpaid post.
	Price *SuggestedPostPrice `json:"price,omitempty"`
	// SendDate is the proposed publication time (Unix time); 0 means any time
	// within 30 days.
	SendDate int64 `json:"send_date,omitempty"`
}

// SuggestedPostApproved is a service message: a suggested post was approved
// (Bot API 9.2).
type SuggestedPostApproved struct {
	// SuggestedPostMessage is the suggested post; the server leaves out its
	// ReplyToMessage.
	SuggestedPostMessage *Message `json:"suggested_post_message,omitempty"`
	// Price is the amount paid for the post; nil for an unpaid post.
	Price *SuggestedPostPrice `json:"price,omitempty"`
	// SendDate is when the post will be published (Unix time).
	SendDate int64 `json:"send_date"`
}

// SuggestedPostApprovalFailed is a service message: the approval of a
// suggested post failed (Bot API 9.2), today only because the user had not
// enough funds.
type SuggestedPostApprovalFailed struct {
	// SuggestedPostMessage is the suggested post; the server leaves out its
	// ReplyToMessage.
	SuggestedPostMessage *Message `json:"suggested_post_message,omitempty"`
	// Price is the expected price of the post.
	Price SuggestedPostPrice `json:"price"`
}

// SuggestedPostDeclined is a service message: a suggested post was declined
// (Bot API 9.2).
type SuggestedPostDeclined struct {
	// SuggestedPostMessage is the suggested post; the server leaves out its
	// ReplyToMessage.
	SuggestedPostMessage *Message `json:"suggested_post_message,omitempty"`
	// Comment is the comment given with the decline, if any.
	Comment string `json:"comment,omitempty"`
}

// SuggestedPostPaid is a service message: the payment for a suggested post was
// received (Bot API 9.2).
type SuggestedPostPaid struct {
	// SuggestedPostMessage is the suggested post; the server leaves out its
	// ReplyToMessage.
	SuggestedPostMessage *Message `json:"suggested_post_message,omitempty"`
	// Currency is "XTR" for Telegram Stars or "TON" for TON grams.
	Currency string `json:"currency"`
	// Amount is the nanograms the channel received; for TON payments only.
	Amount int64 `json:"amount,omitempty"`
	// StarAmount is the Stars the channel received; for Star payments only.
	StarAmount *StarAmount `json:"star_amount,omitempty"`
}

// SuggestedPostRefunded is a service message: the payment for a suggested post
// was refunded (Bot API 9.2).
type SuggestedPostRefunded struct {
	// SuggestedPostMessage is the suggested post; the server leaves out its
	// ReplyToMessage.
	SuggestedPostMessage *Message `json:"suggested_post_message,omitempty"`
	// Reason is "post_deleted" (the post was deleted within 24 hours of being
	// posted, or removed from the scheduled messages before it was posted) or
	// "payment_refunded" (the payer refunded the payment).
	Reason string `json:"reason"`
}
