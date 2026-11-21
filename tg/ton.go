// RED STUB (B-65 65e step 8): every new field is tagged json:"-" and the constants are "x"; replaced by the real change.
package tg

// Bot API 10.4 TON types used by widget queries (WidgetQueryType.WalletCard).

// TonWalletCard is a TON wallet card, shown on a user's profile page to everyone (Bot API 10.4).
type TonWalletCard struct {
	Address              string                 `json:"-"`
	Amount               string                 `json:"-"` // balance in the smallest units, as a decimal number
	Stickers             []TonWalletCardSticker `json:"-"`
	BackgroundNFTAddress string                 `json:"-"` // a sticker NFT whose colors the background uses; empty: the default background
}

// TonWalletCardSticker is a sticker on a TON wallet card (Bot API 10.4).
type TonWalletCardSticker struct {
	Position AreaPosition `json:"-"`
	NFT      TonNFT       `json:"-"`
}

// TonNFT is an NFT in the TON blockchain (Bot API 10.4).
type TonNFT struct {
	Address           string            `json:"-"`
	CollectionAddress string            `json:"-"` // empty if unknown
	Index             string            `json:"-"` // index in the collection, as a decimal number
	Name              string            `json:"-"`
	Description       string            `json:"-"`
	Attributes        []TonNFTAttribute `json:"-"`
	// Gift is the unique gift that corresponds to the NFT. tg.UniqueGift does not have every spec field yet
	// (gift_id, is_premium, is_from_blockchain, publisher_chat).
	Gift *UniqueGift `json:"-"`
}

// TonNFTAttribute is an attribute of a TON NFT (Bot API 10.4).
type TonNFTAttribute struct {
	TraitType string `json:"-"`
	Value     string `json:"-"`
}

// AreaPosition is the position of a rectangle area on a story media or a TON wallet card (Bot API 10.4; the spec's
// earlier StoryAreaPosition). Percentages are of the media or card width (X, width, corner radius) or height (Y,
// height).
type AreaPosition struct {
	XPercentage            float64 `json:"-"` // the center
	YPercentage            float64 `json:"-"` // the center
	WidthPercentage        float64 `json:"-"`
	HeightPercentage       float64 `json:"-"`
	RotationAngle          float64 `json:"-"` // clockwise, in degrees, 0-360
	CornerRadiusPercentage float64 `json:"-"`
}
