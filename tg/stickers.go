package tg

// StickerSet represents a sticker set.
type StickerSet struct {
	Name        string     `json:"name"`
	Title       string     `json:"title"`
	StickerType string     `json:"sticker_type"` // "regular", "mask", "custom_emoji"
	IsAnimated  bool       `json:"is_animated"`
	IsVideo     bool       `json:"is_video"`
	Stickers    []Sticker  `json:"stickers"`
	Thumbnail   *PhotoSize `json:"thumbnail,omitempty"`
}
