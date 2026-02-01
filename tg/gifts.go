package tg

// Gifts represents a list of available gifts.
type Gifts struct {
	Gifts []Gift `json:"gifts"`
}

// Gift represents a gift that can be sent.
type Gift struct {
	ID               string  `json:"id"`
	Sticker          Sticker `json:"sticker"`
	StarCount        int     `json:"star_count"`
	UpgradeStarCount int     `json:"upgrade_star_count,omitempty"`
	TotalCount       int     `json:"total_count,omitempty"`
	RemainingCount   int     `json:"remaining_count,omitempty"`
}

// OwnedGifts represents a list of gifts owned by a user.
type OwnedGifts struct {
	TotalCount int         `json:"total_count"`
	Gifts      []OwnedGift `json:"gifts"`
	NextOffset string      `json:"next_offset,omitempty"`
}

// OwnedGift represents a gift owned by a user.
type OwnedGift struct {
	Type        string `json:"type"` // "regular" or "unique"
	Gift        *Gift  `json:"gift,omitempty"`
	OwnedGiftID string `json:"owned_gift_id,omitempty"`

	// Sender info
	SenderUser *User `json:"sender_user,omitempty"`
	SendDate   int64 `json:"send_date"`

	// Message
	Text         string          `json:"text,omitempty"`
	TextEntities []MessageEntity `json:"text_entities,omitempty"`

	// State flags
	IsSaved       bool `json:"is_saved,omitempty"`
	CanBeUpgraded bool `json:"can_be_upgraded,omitempty"`
	WasRefunded   bool `json:"was_refunded,omitempty"`

	// Star values
	ConvertStarCount        int `json:"convert_star_count,omitempty"`
	PrepaidUpgradeStarCount int `json:"prepaid_upgrade_star_count,omitempty"`
	TransferStarCount       int `json:"transfer_star_count,omitempty"`
}

// AcceptedGiftTypes describes which gift types are accepted.
type AcceptedGiftTypes struct {
	UnlimitedGifts      bool `json:"unlimited_gifts,omitempty"`
	LimitedGifts        bool `json:"limited_gifts,omitempty"`
	UniqueGifts         bool `json:"unique_gifts,omitempty"`
	PremiumSubscription bool `json:"premium_subscription,omitempty"`
}
