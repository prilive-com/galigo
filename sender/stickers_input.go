package sender

import "github.com/prilive-com/galigo/tg"

// InputSticker represents a sticker to be uploaded to a sticker set.
// Lives in sender/ because it contains InputFile (avoids tg/ → sender/ import cycle).
type InputSticker struct {
	Sticker      InputFile        `json:"-"`      // Handled by multipart encoder
	Format       string           `json:"format"` // "static", "animated", "video"
	EmojiList    []string         `json:"emoji_list"`
	MaskPosition *tg.MaskPosition `json:"mask_position,omitempty"`
	Keywords     []string         `json:"keywords,omitempty"`
}
