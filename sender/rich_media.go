package sender

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 rich message media: InputMedia as tg.InputRichMedia, and uploads in tg.InputRichMessage.Media.

// errRichMediaUpload is returned when an InputMedia with a file upload is JSON-encoded: only the multipart builder
// can send an upload, and only for tg.InputRichMessage.Media.
var errRichMediaUpload = errors.New("sender: an InputMedia upload in a rich message is supported only in " +
	"InputRichMessage.Media of a request sent by sender.Client; use a file_id or an HTTP URL")

// NewInputMediaVoiceNote creates an InputMedia of type "voice_note" (Bot API 10.2, rich messages).
func NewInputMediaVoiceNote(media InputFile) InputMedia {
	return InputMedia{Type: "voice_note", Media: media}
}

// InputRichMediaType returns the media type; it makes InputMedia a tg.InputRichMedia (Bot API 10.2).
func (m InputMedia) InputRichMediaType() string { return m.Type }

// MarshalJSON encodes the media as the Bot API InputMedia* item that the multipart builder writes for it (Bot API
// 10.2, rich messages). A file upload cannot be JSON-encoded: it returns an error, and its reader is not opened.
func (m InputMedia) MarshalJSON() ([]byte, error) {
	switch m.Type {
	case "location", "venue", "link":
	default:
		if fileNeedsUpload(m.Media) || (m.Media.Photo != nil && fileNeedsUpload(*m.Media.Photo)) {
			return nil, errRichMediaUpload
		}
	}
	var scratch MultipartRequest
	idx := 0
	item, err := inputMediaItem(&scratch, m, &idx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(item)
}

// fileNeedsUpload reports whether the builder sends f as a file part: no file_id and no URL (they come first), and a
// Reader or Source.
func fileNeedsUpload(f InputFile) bool {
	return f.FileID == "" && f.URL == "" && f.IsUpload()
}

// richMediaItem is a resolved InputMedia upload in a rich message: its item with an attach:// reference.
type richMediaItem map[string]any

func (r richMediaItem) InputRichMediaType() string {
	s, _ := r["type"].(string)
	return s
}

// handleRichMessage writes a rich_message field (Bot API 10.2). Each InputMedia in its Media list is replaced, in a
// copy, by the item inputMediaItem builds: an upload becomes an attach:// item with its own file part, in list order;
// a file_id or URL gives the same item as InputMedia.MarshalJSON. The caller's value is not changed. Without an upload
// the form value is the same JSON as json.Marshal of the value.
func handleRichMessage(req *MultipartRequest, fieldName string, rm tg.InputRichMessage, attachIdx *int) error {
	if len(rm.Media) > 0 {
		media := make([]tg.InputRichMessageMedia, len(rm.Media))
		copy(media, rm.Media)
		for i, item := range media {
			var m InputMedia
			switch v := item.Media.(type) {
			case InputMedia:
				m = v
			case *InputMedia:
				if v == nil {
					continue
				}
				m = *v
			default:
				continue
			}
			resolved, err := inputMediaItem(req, m, attachIdx)
			if err != nil {
				return fmt.Errorf("media %d: %w", i, err)
			}
			media[i].Media = richMediaItem(resolved)
		}
		rm.Media = media
	}
	data, err := json.Marshal(rm)
	if err != nil {
		return fmt.Errorf("JSON marshal rich message: %w", err)
	}
	req.Params[fieldName] = string(data)
	return nil
}
