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
