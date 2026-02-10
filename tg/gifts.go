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

// UniqueGiftModel describes the model of a unique gift.
// Added in Bot API 9.0, updated in 9.4.
type UniqueGiftModel struct {
	Name           string  `json:"name"`
	Sticker        Sticker `json:"sticker"`
	RarityPerMille int     `json:"rarity_per_mille"` // 9.0 — required, 0 for crafted
	Rarity         string  `json:"rarity,omitempty"` // 9.4: "uncommon"|"rare"|"epic"|"legendary"
}

// UniqueGiftSymbol describes the symbol of a unique gift.
// Added in Bot API 9.0.
type UniqueGiftSymbol struct {
	Name           string  `json:"name"`
	Sticker        Sticker `json:"sticker"`
	RarityPerMille int     `json:"rarity_per_mille"` // required, 0 for crafted
}

// UniqueGiftBackdropColors describes the colors of the backdrop of a unique gift.
// All fields are RGB24 integers (0..16777215 / 0x000000..0xFFFFFF).
// Added in Bot API 9.0.
type UniqueGiftBackdropColors struct {
	CenterColor int `json:"center_color"`
	EdgeColor   int `json:"edge_color"`
	SymbolColor int `json:"symbol_color"`
	TextColor   int `json:"text_color"`
}

// UniqueGiftBackdrop describes the backdrop of a unique gift.
// Added in Bot API 9.0.
type UniqueGiftBackdrop struct {
	Name           string                   `json:"name"`
	Colors         UniqueGiftBackdropColors `json:"colors"`
	RarityPerMille int                      `json:"rarity_per_mille"` // required, 0 for crafted
}

// UniqueGiftColors describes the color scheme for a user's name,
// message replies and link previews based on a unique gift.
// Added in Bot API 9.3.
type UniqueGiftColors struct {
	ModelCustomEmojiID    string `json:"model_custom_emoji_id"`
	SymbolCustomEmojiID   string `json:"symbol_custom_emoji_id"`
	LightThemeMainColor   int    `json:"light_theme_main_color"`   // RGB24
	LightThemeOtherColors []int  `json:"light_theme_other_colors"` // 1-3 RGB24 colors
	DarkThemeMainColor    int    `json:"dark_theme_main_color"`    // RGB24
	DarkThemeOtherColors  []int  `json:"dark_theme_other_colors"`  // 1-3 RGB24 colors
}

// UniqueGift represents a gift upgraded to a unique one.
// Added in Bot API 9.0, updated in 9.3 and 9.4.
type UniqueGift struct {
	BaseName string             `json:"base_name"`
	Name     string             `json:"name"`
	Number   int                `json:"number"`
	Model    UniqueGiftModel    `json:"model"`
	Symbol   UniqueGiftSymbol   `json:"symbol"`
	Backdrop UniqueGiftBackdrop `json:"backdrop"`
	Colors   *UniqueGiftColors  `json:"colors,omitempty"`    // 9.3 — optional
	IsBurned bool               `json:"is_burned,omitempty"` // 9.4
}

// UniqueGiftModel rarity constants (added in Bot API 9.4).
const (
	GiftRarityUncommon  = "uncommon"
	GiftRarityRare      = "rare"
	GiftRarityEpic      = "epic"
	GiftRarityLegendary = "legendary"
)
