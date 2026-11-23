package tg

import "encoding/json"

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
	PublisherChat    *Chat   `json:"publisher_chat,omitempty"` // 9.2: the chat that published the gift
}

// OwnedGifts is a list of gifts owned by a user or a chat (the result of getUserGifts, getChatGifts and
// getBusinessAccountGifts).
type OwnedGifts struct {
	TotalCount int         `json:"total_count"`
	Gifts      []OwnedGift `json:"gifts"`
	NextOffset string      `json:"next_offset,omitempty"`
}

// OwnedGift is a gift owned by a user or a chat. It is one of the spec's OwnedGiftRegular ("regular") and
// OwnedGiftUnique ("unique"); Type says which. The spec's "gift" key is decoded into Gift for a regular gift and into
// UniqueGift for a unique gift; for any other Type its body is dropped and both stay nil. Encode writes "gift" from
// UniqueGift when Type is "unique", else from Gift.
type OwnedGift struct {
	Type       string      `json:"type"` // "regular" or "unique"
	Gift       *Gift       `json:"gift,omitempty"`
	UniqueGift *UniqueGift `json:"-"` // unique: the gift (the spec's "gift" key, filled by the codec)

	OwnedGiftID string `json:"owned_gift_id,omitempty"` // both

	// Sender info (both)
	SenderUser *User `json:"sender_user,omitempty"`
	SendDate   int64 `json:"send_date"`

	// Message (regular)
	Text         string          `json:"text,omitempty"`
	TextEntities []MessageEntity `json:"entities,omitempty"` // the spec's "entities"

	// State flags
	IsPrivate     bool `json:"is_private,omitempty"`      // regular
	IsSaved       bool `json:"is_saved,omitempty"`        // both
	CanBeUpgraded bool `json:"can_be_upgraded,omitempty"` // regular
	WasRefunded   bool `json:"was_refunded,omitempty"`    // regular

	// Star values
	ConvertStarCount        int   `json:"convert_star_count,omitempty"`         // regular
	PrepaidUpgradeStarCount int   `json:"prepaid_upgrade_star_count,omitempty"` // regular
	IsUpgradeSeparate       bool  `json:"is_upgrade_separate,omitempty"`        // regular
	UniqueGiftNumber        int   `json:"unique_gift_number,omitempty"`         // regular
	CanBeTransferred        bool  `json:"can_be_transferred,omitempty"`         // unique
	TransferStarCount       int   `json:"transfer_star_count,omitempty"`        // unique
	NextTransferDate        int64 `json:"next_transfer_date,omitempty"`         // unique: Unix time
}

// UnmarshalJSON decodes an OwnedGift. JSON null changes nothing. Otherwise Gift and UniqueGift are cleared, the other
// keys merge into g, and "gift" is routed by the Type after this decode.
func (g *OwnedGift) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	type plain OwnedGift
	aux := struct {
		*plain
		Gift json.RawMessage `json:"gift"`
	}{plain: (*plain)(g)}
	g.Gift, g.UniqueGift = nil, nil
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Gift) == 0 || string(aux.Gift) == "null" {
		return nil
	}
	switch g.Type {
	case "regular":
		g.Gift = new(Gift)
		return json.Unmarshal(aux.Gift, g.Gift)
	case "unique":
		g.UniqueGift = new(UniqueGift)
		return json.Unmarshal(aux.Gift, g.UniqueGift)
	}
	return nil
}

// MarshalJSON encodes an OwnedGift with "gift" taken from UniqueGift when Type is "unique", else from Gift.
func (g OwnedGift) MarshalJSON() ([]byte, error) {
	type plain OwnedGift
	aux := struct {
		Type string `json:"type"`
		Gift any    `json:"gift,omitempty"`
		plain
	}{Type: g.Type, plain: plain(g)}
	if g.Type == "unique" {
		if g.UniqueGift != nil {
			aux.Gift = g.UniqueGift
		}
	} else if g.Gift != nil {
		aux.Gift = g.Gift
	}
	return json.Marshal(aux)
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
// Added in Bot API 9.0, updated in 9.2, 9.3 and 9.4. The field order is not the spec's (Colors stays before
// IsBurned); JSON key order is not a contract.
type UniqueGift struct {
	// GiftID is the identifier of the regular gift this unique gift was upgraded from (9.3, required; not an
	// owned_gift_id). Encode always writes it.
	GiftID           string             `json:"gift_id"`
	BaseName         string             `json:"base_name"`
	Name             string             `json:"name"`
	Number           int                `json:"number"`
	Model            UniqueGiftModel    `json:"model"`
	Symbol           UniqueGiftSymbol   `json:"symbol"`
	Backdrop         UniqueGiftBackdrop `json:"backdrop"`
	IsPremium        bool               `json:"is_premium,omitempty"`         // 9.3: only Telegram Premium users can buy it
	Colors           *UniqueGiftColors  `json:"colors,omitempty"`             // 9.3 — optional
	IsBurned         bool               `json:"is_burned,omitempty"`          // 9.4
	IsFromBlockchain bool               `json:"is_from_blockchain,omitempty"` // 9.3: assigned from the TON blockchain
	PublisherChat    *Chat              `json:"publisher_chat,omitempty"`     // 9.2: the chat that published the gift
}

// UniqueGiftModel rarity constants (added in Bot API 9.4).
const (
	GiftRarityUncommon  = "uncommon"
	GiftRarityRare      = "rare"
	GiftRarityEpic      = "epic"
	GiftRarityLegendary = "legendary"
)
