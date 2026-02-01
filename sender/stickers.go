package sender

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// ================== Sticker Request Types ==================

// GetStickerSetRequest represents a getStickerSet request.
type GetStickerSetRequest struct {
	Name string `json:"name"`
}

// GetCustomEmojiStickersRequest represents a getCustomEmojiStickers request.
type GetCustomEmojiStickersRequest struct {
	CustomEmojiIDs []string `json:"custom_emoji_ids"`
}

// UploadStickerFileRequest represents an uploadStickerFile request.
// The sticker file is handled via multipart.
type UploadStickerFileRequest struct {
	UserID        int64     `json:"user_id"`
	Sticker       InputFile `json:"sticker"`
	StickerFormat string    `json:"sticker_format"` // "static", "animated", "video"
}

// CreateNewStickerSetRequest represents a createNewStickerSet request.
type CreateNewStickerSetRequest struct {
	UserID          int64    `json:"user_id"`
	Name            string   `json:"name"`
	Title           string   `json:"title"`
	Stickers        []InputSticker `json:"-"` // Handled specially
	StickerType     string   `json:"sticker_type,omitempty"` // "regular", "mask", "custom_emoji"
	NeedsRepainting bool     `json:"needs_repainting,omitempty"`
}

// AddStickerToSetRequest represents an addStickerToSet request.
type AddStickerToSetRequest struct {
	UserID  int64        `json:"user_id"`
	Name    string       `json:"name"`
	Sticker InputSticker `json:"-"` // Handled specially
}

// SetStickerPositionInSetRequest represents a setStickerPositionInSet request.
type SetStickerPositionInSetRequest struct {
	Sticker  string `json:"sticker"` // file_id
	Position int    `json:"position"`
}

// DeleteStickerFromSetRequest represents a deleteStickerFromSet request.
type DeleteStickerFromSetRequest struct {
	Sticker string `json:"sticker"` // file_id
}

// SetStickerSetTitleRequest represents a setStickerSetTitle request.
type SetStickerSetTitleRequest struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// DeleteStickerSetRequest represents a deleteStickerSet request.
type DeleteStickerSetRequest struct {
	Name string `json:"name"`
}

// SetStickerSetThumbnailRequest represents a setStickerSetThumbnail request.
type SetStickerSetThumbnailRequest struct {
	Name      string    `json:"name"`
	UserID    int64     `json:"user_id"`
	Thumbnail InputFile `json:"thumbnail"`
	Format    string    `json:"format"` // "static", "animated", "video"
}

// SetStickerEmojiListRequest represents a setStickerEmojiList request.
type SetStickerEmojiListRequest struct {
	Sticker   string   `json:"sticker"` // file_id
	EmojiList []string `json:"emoji_list"`
}

// SetStickerKeywordsRequest represents a setStickerKeywords request.
type SetStickerKeywordsRequest struct {
	Sticker  string   `json:"sticker"` // file_id
	Keywords []string `json:"keywords,omitempty"`
}

// SetStickerMaskPositionRequest represents a setStickerMaskPosition request.
type SetStickerMaskPositionRequest struct {
	Sticker      string           `json:"sticker"` // file_id
	MaskPosition *tg.MaskPosition `json:"mask_position,omitempty"`
}

// ReplaceStickerInSetRequest represents a replaceStickerInSet request.
type ReplaceStickerInSetRequest struct {
	UserID     int64        `json:"user_id"`
	Name       string       `json:"name"`
	OldSticker string       `json:"old_sticker"` // file_id of sticker to replace
	Sticker    InputSticker `json:"-"`            // Handled specially
}

// ================== Sticker Methods ==================

// GetStickerSet returns a sticker set by name.
func (c *Client) GetStickerSet(ctx context.Context, name string) (*tg.StickerSet, error) {
	if name == "" {
		return nil, tg.NewValidationError("name", "required")
	}

	var result tg.StickerSet
	if err := c.callJSON(ctx, "getStickerSet", GetStickerSetRequest{Name: name}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetCustomEmojiStickers returns information about custom emoji stickers by their identifiers.
func (c *Client) GetCustomEmojiStickers(ctx context.Context, customEmojiIDs []string) ([]tg.Sticker, error) {
	if len(customEmojiIDs) == 0 {
		return nil, tg.NewValidationError("custom_emoji_ids", "at least one ID required")
	}
	if len(customEmojiIDs) > 200 {
		return nil, tg.NewValidationError("custom_emoji_ids", "at most 200 IDs allowed")
	}

	var result []tg.Sticker
	if err := c.callJSON(ctx, "getCustomEmojiStickers", GetCustomEmojiStickersRequest{CustomEmojiIDs: customEmojiIDs}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// UploadStickerFile uploads a sticker file for later use in sticker sets.
func (c *Client) UploadStickerFile(ctx context.Context, req UploadStickerFileRequest) (*tg.File, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	if req.Sticker.IsEmpty() {
		return nil, tg.NewValidationError("sticker", "required")
	}
	if req.StickerFormat == "" {
		return nil, tg.NewValidationError("sticker_format", "required")
	}

	var result tg.File
	if err := c.callJSON(ctx, "uploadStickerFile", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateNewStickerSet creates a new sticker set owned by a user.
func (c *Client) CreateNewStickerSet(ctx context.Context, req CreateNewStickerSetRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	if req.Name == "" {
		return tg.NewValidationError("name", "required")
	}
	if req.Title == "" {
		return tg.NewValidationError("title", "required")
	}
	if len(req.Stickers) == 0 {
		return tg.NewValidationError("stickers", "at least one sticker required")
	}
	if len(req.Stickers) > 50 {
		return tg.NewValidationError("stickers", "at most 50 stickers allowed")
	}

	payload, err := buildStickerSetPayload(req.UserID, req.Name, req.Title, req.StickerType, req.NeedsRepainting, req.Stickers)
	if err != nil {
		return err
	}

	return c.callJSON(ctx, "createNewStickerSet", payload, nil)
}

// AddStickerToSet adds a new sticker to an existing sticker set.
func (c *Client) AddStickerToSet(ctx context.Context, req AddStickerToSetRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	if req.Name == "" {
		return tg.NewValidationError("name", "required")
	}

	payload, err := buildSingleStickerPayload(req.UserID, req.Name, "", req.Sticker)
	if err != nil {
		return err
	}

	return c.callJSON(ctx, "addStickerToSet", payload, nil)
}

// SetStickerPositionInSet moves a sticker in a set to a specific position.
func (c *Client) SetStickerPositionInSet(ctx context.Context, req SetStickerPositionInSetRequest) error {
	if req.Sticker == "" {
		return tg.NewValidationError("sticker", "required")
	}

	return c.callJSON(ctx, "setStickerPositionInSet", req, nil)
}

// DeleteStickerFromSet deletes a sticker from a set.
func (c *Client) DeleteStickerFromSet(ctx context.Context, sticker string) error {
	if sticker == "" {
		return tg.NewValidationError("sticker", "required")
	}

	return c.callJSON(ctx, "deleteStickerFromSet", DeleteStickerFromSetRequest{Sticker: sticker}, nil)
}

// SetStickerSetTitle sets the title of a sticker set.
func (c *Client) SetStickerSetTitle(ctx context.Context, name, title string) error {
	if name == "" {
		return tg.NewValidationError("name", "required")
	}
	if title == "" {
		return tg.NewValidationError("title", "required")
	}

	return c.callJSON(ctx, "setStickerSetTitle", SetStickerSetTitleRequest{Name: name, Title: title}, nil)
}

// DeleteStickerSet deletes a sticker set.
func (c *Client) DeleteStickerSet(ctx context.Context, name string) error {
	if name == "" {
		return tg.NewValidationError("name", "required")
	}

	return c.callJSON(ctx, "deleteStickerSet", DeleteStickerSetRequest{Name: name}, nil)
}

// SetStickerSetThumbnail sets the thumbnail of a sticker set.
func (c *Client) SetStickerSetThumbnail(ctx context.Context, req SetStickerSetThumbnailRequest) error {
	if req.Name == "" {
		return tg.NewValidationError("name", "required")
	}
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	if req.Format == "" {
		return tg.NewValidationError("format", "required")
	}

	return c.callJSON(ctx, "setStickerSetThumbnail", req, nil)
}

// SetStickerEmojiList changes the list of emojis assigned to a sticker.
func (c *Client) SetStickerEmojiList(ctx context.Context, req SetStickerEmojiListRequest) error {
	if req.Sticker == "" {
		return tg.NewValidationError("sticker", "required")
	}
	if len(req.EmojiList) == 0 {
		return tg.NewValidationError("emoji_list", "at least one emoji required")
	}

	return c.callJSON(ctx, "setStickerEmojiList", req, nil)
}

// SetStickerKeywords changes the search keywords for a sticker.
func (c *Client) SetStickerKeywords(ctx context.Context, req SetStickerKeywordsRequest) error {
	if req.Sticker == "" {
		return tg.NewValidationError("sticker", "required")
	}

	return c.callJSON(ctx, "setStickerKeywords", req, nil)
}

// SetStickerMaskPosition changes the mask position of a mask sticker.
func (c *Client) SetStickerMaskPosition(ctx context.Context, req SetStickerMaskPositionRequest) error {
	if req.Sticker == "" {
		return tg.NewValidationError("sticker", "required")
	}

	return c.callJSON(ctx, "setStickerMaskPosition", req, nil)
}

// ReplaceStickerInSet replaces an existing sticker in a sticker set with a new one.
func (c *Client) ReplaceStickerInSet(ctx context.Context, req ReplaceStickerInSetRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	if req.Name == "" {
		return tg.NewValidationError("name", "required")
	}
	if req.OldSticker == "" {
		return tg.NewValidationError("old_sticker", "required")
	}

	payload, err := buildSingleStickerPayload(req.UserID, req.Name, req.OldSticker, req.Sticker)
	if err != nil {
		return err
	}

	return c.callJSON(ctx, "replaceStickerInSet", payload, nil)
}

// ================== Internal Helpers ==================

// inputStickerJSON is the JSON representation of InputSticker for the API.
type inputStickerJSON struct {
	Sticker      string           `json:"sticker"`
	Format       string           `json:"format"`
	EmojiList    []string         `json:"emoji_list"`
	MaskPosition *tg.MaskPosition `json:"mask_position,omitempty"`
	Keywords     []string         `json:"keywords,omitempty"`
}

// resolveInputSticker converts an InputSticker to its JSON representation,
// resolving InputFile to FileID, URL, or attach:// reference.
// Returns the JSON struct and an optional FilePart for uploads.
func resolveInputSticker(s InputSticker, attachName string) (inputStickerJSON, *FilePart, error) {
	sj := inputStickerJSON{
		Format:       s.Format,
		EmojiList:    s.EmojiList,
		MaskPosition: s.MaskPosition,
		Keywords:     s.Keywords,
	}

	switch {
	case s.Sticker.FileID != "":
		sj.Sticker = s.Sticker.FileID
		return sj, nil, nil
	case s.Sticker.URL != "":
		sj.Sticker = s.Sticker.URL
		return sj, nil, nil
	case s.Sticker.Reader != nil || s.Sticker.Source != nil:
		sj.Sticker = "attach://" + attachName
		fp := &FilePart{
			FieldName: attachName,
			FileName:  s.Sticker.FileName,
			Reader:    s.Sticker.OpenReader(),
		}
		return sj, fp, nil
	default:
		return sj, nil, fmt.Errorf("InputFile must have FileID, URL, or Reader set")
	}
}

// buildStickerSetPayload builds a multipart-compatible request for createNewStickerSet.
// InputSticker files are encoded as attach:// references.
func buildStickerSetPayload(userID int64, name, title, stickerType string, needsRepainting bool, stickers []InputSticker) (*stickerSetRequest, error) {
	req := &stickerSetRequest{
		UserID:          userID,
		Name:            name,
		Title:           title,
		StickerType:     stickerType,
		NeedsRepainting: needsRepainting,
	}

	stickerJSONs := make([]inputStickerJSON, 0, len(stickers))
	for i, s := range stickers {
		sj, fp, err := resolveInputSticker(s, fmt.Sprintf("sticker_file_%d", i))
		if err != nil {
			return nil, fmt.Errorf("sticker[%d]: %w", i, err)
		}
		if fp != nil {
			req.AttachedFiles = append(req.AttachedFiles, *fp)
		}
		stickerJSONs = append(stickerJSONs, sj)
	}

	data, err := json.Marshal(stickerJSONs)
	if err != nil {
		return nil, fmt.Errorf("marshal stickers: %w", err)
	}
	req.StickersJSON = string(data)

	return req, nil
}

// buildSingleStickerPayload builds a payload for addStickerToSet / replaceStickerInSet.
func buildSingleStickerPayload(userID int64, name, oldSticker string, sticker InputSticker) (*singleStickerRequest, error) {
	req := &singleStickerRequest{
		UserID:     userID,
		Name:       name,
		OldSticker: oldSticker,
	}

	sj, fp, err := resolveInputSticker(sticker, "sticker_file_0")
	if err != nil {
		return nil, fmt.Errorf("sticker: %w", err)
	}
	if fp != nil {
		req.AttachedFiles = append(req.AttachedFiles, *fp)
	}

	data, err := json.Marshal(sj)
	if err != nil {
		return nil, fmt.Errorf("marshal sticker: %w", err)
	}
	req.StickerJSON = string(data)

	return req, nil
}

// stickerSetRequest is the internal payload for createNewStickerSet.
// The stickers array is pre-serialized as JSON string, and files are attached separately.
type stickerSetRequest struct {
	UserID          int64      `json:"user_id"`
	Name            string     `json:"name"`
	Title           string     `json:"title"`
	StickerType     string     `json:"sticker_type,omitempty"`
	NeedsRepainting bool       `json:"needs_repainting,omitempty"`
	StickersJSON    string     `json:"stickers"`    // Pre-serialized JSON array
	AttachedFiles   []FilePart `json:"_file_parts"` // Picked up by multipart builder
}

// singleStickerRequest is the internal payload for addStickerToSet / replaceStickerInSet.
type singleStickerRequest struct {
	UserID        int64      `json:"user_id"`
	Name          string     `json:"name"`
	OldSticker    string     `json:"old_sticker,omitempty"`
	StickerJSON   string     `json:"sticker"`     // Pre-serialized JSON object
	AttachedFiles []FilePart `json:"_file_parts"` // Picked up by multipart builder
}
