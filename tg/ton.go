package tg

// Bot API 10.4 TON types: the wallet card (WidgetQueryType.WalletCard, ChatFullInfo.TonWalletCard) and the TON
// transfer service message (Message.TonWalletTransfer).

// TonWalletCard is a TON wallet card, shown on a user's profile page to everyone (Bot API 10.4).
type TonWalletCard struct {
	Address              string                 `json:"address"`
	Amount               string                 `json:"amount"` // balance in the smallest units, as a decimal number
	Stickers             []TonWalletCardSticker `json:"stickers"`
	BackgroundNFTAddress string                 `json:"background_nft_address,omitempty"` // a sticker NFT whose colors the background uses; empty: the default background
}

// TonWalletCardSticker is a sticker on a TON wallet card (Bot API 10.4).
type TonWalletCardSticker struct {
	Position AreaPosition `json:"position"`
	NFT      TonNFT       `json:"nft"`
}

// TonNFT is an NFT in the TON blockchain (Bot API 10.4).
type TonNFT struct {
	Address           string            `json:"address"`
	CollectionAddress string            `json:"collection_address,omitempty"` // empty if unknown
	Index             string            `json:"index"`                        // index in the collection, as a decimal number
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Attributes        []TonNFTAttribute `json:"attributes,omitempty"`
	// Gift is the unique gift that corresponds to the NFT.
	Gift *UniqueGift `json:"gift,omitempty"`
}

// TonNFTAttribute is an attribute of a TON NFT (Bot API 10.4).
type TonNFTAttribute struct {
	TraitType string `json:"trait_type"`
	Value     string `json:"value"`
}

// AreaPosition is the position of a rectangle area on a story media or a TON wallet card (Bot API 10.4; the spec's
// earlier StoryAreaPosition). Percentages are of the media or card width (X, width, corner radius) or height (Y,
// height).
type AreaPosition struct {
	XPercentage            float64 `json:"x_percentage"` // the center
	YPercentage            float64 `json:"y_percentage"` // the center
	WidthPercentage        float64 `json:"width_percentage"`
	HeightPercentage       float64 `json:"height_percentage"`
	RotationAngle          float64 `json:"rotation_angle"` // clockwise, in degrees, 0-360
	CornerRadiusPercentage float64 `json:"corner_radius_percentage"`
}

// TonWalletTransfer is a service message about a TON transfer (Bot API 10.4). Telegram omits comment when the comment is
// encrypted, so an empty Comment means no comment or an encrypted one.
type TonWalletTransfer struct {
	TransactionID string `json:"transaction_id"` // empty if the transaction was not sent yet
	PeerAddress   string `json:"peer_address"`
	Amount        string `json:"amount"` // in the smallest units, as a decimal number; negative for an outgoing transfer
	Comment       string `json:"comment,omitempty"`
}
