package sender

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// ================== Business Request Types ==================

// GetBusinessConnectionRequest represents a getBusinessConnection request.
type GetBusinessConnectionRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
}

// SetBusinessAccountNameRequest represents a setBusinessAccountName request.
type SetBusinessAccountNameRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	FirstName            string `json:"first_name"`
	LastName             string `json:"last_name,omitempty"`
}

// SetBusinessAccountBioRequest represents a setBusinessAccountBio request.
type SetBusinessAccountBioRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	Bio                  string `json:"bio,omitempty"`
}

// SetBusinessAccountUsernameRequest represents a setBusinessAccountUsername request.
type SetBusinessAccountUsernameRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	Username             string `json:"username,omitempty"`
}

// SetBusinessAccountGiftSettingsRequest represents a setBusinessAccountGiftSettings request.
type SetBusinessAccountGiftSettingsRequest struct {
	BusinessConnectionID string               `json:"business_connection_id"`
	ShowGiftButton       bool                 `json:"show_gift_button"`
	AcceptedGiftTypes    tg.AcceptedGiftTypes `json:"accepted_gift_types"`
}

// PostStoryRequest represents a postStory request.
type PostStoryRequest struct {
	BusinessConnectionID string             `json:"business_connection_id"`
	Content              InputStoryContent  `json:"-"` // Handled by multipart
	ActivePeriod         int                `json:"active_period,omitempty"`
	Caption              string             `json:"caption,omitempty"`
	ParseMode            string             `json:"parse_mode,omitempty"`
	CaptionEntities      []tg.MessageEntity `json:"caption_entities,omitempty"`
	ProtectContent       bool               `json:"protect_content,omitempty"`
}

// EditStoryRequest represents an editStory request.
type EditStoryRequest struct {
	BusinessConnectionID string             `json:"business_connection_id"`
	StoryID              int                `json:"story_id"`
	Content              InputStoryContent  `json:"-"` // Optional, handled by multipart
	Caption              string             `json:"caption,omitempty"`
	ParseMode            string             `json:"parse_mode,omitempty"`
	CaptionEntities      []tg.MessageEntity `json:"caption_entities,omitempty"`
}

// DeleteStoryRequest represents a deleteStory request.
type DeleteStoryRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	StoryID              int    `json:"story_id"`
}

// TransferBusinessAccountStarsRequest represents a transferBusinessAccountStars request.
type TransferBusinessAccountStarsRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	StarCount            int    `json:"star_count"`
}

// GetBusinessAccountStarBalanceRequest represents a getBusinessAccountStarBalance request.
type GetBusinessAccountStarBalanceRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
}

// SetBusinessAccountProfilePhotoRequest represents a setBusinessAccountProfilePhoto request.
type SetBusinessAccountProfilePhotoRequest struct {
	BusinessConnectionID string            `json:"business_connection_id"`
	Photo                InputProfilePhoto `json:"-"` // Handled by multipart
	IsPublic             bool              `json:"is_public,omitempty"`
}

// RemoveBusinessAccountProfilePhotoRequest represents a removeBusinessAccountProfilePhoto request.
type RemoveBusinessAccountProfilePhotoRequest struct {
	BusinessConnectionID string `json:"business_connection_id"`
	IsPublic             bool   `json:"is_public,omitempty"`
}

// ================== Business Methods ==================

// GetBusinessConnection returns information about a business connection.
func (c *Client) GetBusinessConnection(ctx context.Context, businessConnectionID string) (*tg.BusinessConnection, error) {
	if businessConnectionID == "" {
		return nil, tg.NewValidationError("business_connection_id", "required")
	}

	var result tg.BusinessConnection
	if err := c.callJSON(ctx, "getBusinessConnection", GetBusinessConnectionRequest{BusinessConnectionID: businessConnectionID}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetBusinessAccountName sets the name of a business account.
func (c *Client) SetBusinessAccountName(ctx context.Context, req SetBusinessAccountNameRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.FirstName == "" {
		return tg.NewValidationError("first_name", "required")
	}
	if len(req.FirstName) > 64 {
		return tg.NewValidationError("first_name", "must be 1-64 characters")
	}
	if len(req.LastName) > 64 {
		return tg.NewValidationError("last_name", "must be at most 64 characters")
	}

	return c.callJSON(ctx, "setBusinessAccountName", req, nil)
}

// SetBusinessAccountBio sets the bio of a business account.
func (c *Client) SetBusinessAccountBio(ctx context.Context, req SetBusinessAccountBioRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if len(req.Bio) > 140 {
		return tg.NewValidationError("bio", "must be at most 140 characters")
	}

	return c.callJSON(ctx, "setBusinessAccountBio", req, nil)
}

// SetBusinessAccountUsername sets the username of a business account.
func (c *Client) SetBusinessAccountUsername(ctx context.Context, req SetBusinessAccountUsernameRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}

	return c.callJSON(ctx, "setBusinessAccountUsername", req, nil)
}

// SetBusinessAccountGiftSettings sets the gift settings of a business account.
func (c *Client) SetBusinessAccountGiftSettings(ctx context.Context, req SetBusinessAccountGiftSettingsRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}

	return c.callJSON(ctx, "setBusinessAccountGiftSettings", req, nil)
}

// PostStory posts a story on behalf of a business account.
func (c *Client) PostStory(ctx context.Context, req PostStoryRequest) (*tg.Story, error) {
	if req.BusinessConnectionID == "" {
		return nil, tg.NewValidationError("business_connection_id", "required")
	}
	if req.Content == nil {
		return nil, tg.NewValidationError("content", "required")
	}

	payload, err := buildStoryPayload(req)
	if err != nil {
		return nil, err
	}

	var result tg.Story
	if err := c.callJSON(ctx, "postStory", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EditStory edits a story posted by a business account.
func (c *Client) EditStory(ctx context.Context, req EditStoryRequest) (*tg.Story, error) {
	if req.BusinessConnectionID == "" {
		return nil, tg.NewValidationError("business_connection_id", "required")
	}
	if req.StoryID <= 0 {
		return nil, tg.NewValidationError("story_id", "must be positive")
	}

	payload, err := buildEditStoryPayload(req)
	if err != nil {
		return nil, err
	}

	var result tg.Story
	if err := c.callJSON(ctx, "editStory", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteStory deletes a story posted by a business account.
func (c *Client) DeleteStory(ctx context.Context, req DeleteStoryRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.StoryID <= 0 {
		return tg.NewValidationError("story_id", "must be positive")
	}

	return c.callJSON(ctx, "deleteStory", req, nil)
}

// TransferBusinessAccountStars transfers Stars from the bot to a business account.
// NO RETRY — value operation to prevent double-transfer.
func (c *Client) TransferBusinessAccountStars(ctx context.Context, req TransferBusinessAccountStarsRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.StarCount <= 0 {
		return tg.NewValidationError("star_count", "must be positive")
	}

	// NO RETRY — value operation
	return c.callJSON(ctx, "transferBusinessAccountStars", req, nil)
}

// GetBusinessAccountStarBalance returns the Star balance of a business account.
func (c *Client) GetBusinessAccountStarBalance(ctx context.Context, businessConnectionID string) (*tg.StarAmount, error) {
	if businessConnectionID == "" {
		return nil, tg.NewValidationError("business_connection_id", "required")
	}

	var result tg.StarAmount
	if err := c.callJSON(ctx, "getBusinessAccountStarBalance", GetBusinessAccountStarBalanceRequest{BusinessConnectionID: businessConnectionID}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetBusinessAccountProfilePhoto sets the profile photo of a business account.
func (c *Client) SetBusinessAccountProfilePhoto(ctx context.Context, req SetBusinessAccountProfilePhotoRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}
	if req.Photo == nil {
		return tg.NewValidationError("photo", "required")
	}

	payload, err := buildProfilePhotoPayload(req)
	if err != nil {
		return err
	}

	return c.callJSON(ctx, "setBusinessAccountProfilePhoto", payload, nil)
}

// RemoveBusinessAccountProfilePhoto removes the profile photo of a business account.
func (c *Client) RemoveBusinessAccountProfilePhoto(ctx context.Context, req RemoveBusinessAccountProfilePhotoRequest) error {
	if req.BusinessConnectionID == "" {
		return tg.NewValidationError("business_connection_id", "required")
	}

	return c.callJSON(ctx, "removeBusinessAccountProfilePhoto", req, nil)
}

// ================== Business Internal Helpers ==================

// resolveInputFile converts an InputFile to a string reference (FileID, URL, or attach://name)
// and returns an optional FilePart for uploads.
func resolveInputFile(file InputFile, attachName string) (string, *FilePart, error) {
	switch {
	case file.FileID != "":
		return file.FileID, nil, nil
	case file.URL != "":
		return file.URL, nil, nil
	case file.Reader != nil || file.Source != nil:
		fp := &FilePart{
			FieldName: attachName,
			FileName:  file.FileName,
			Reader:    file.OpenReader(),
		}
		return "attach://" + attachName, fp, nil
	default:
		return "", nil, fmt.Errorf("InputFile must have FileID, URL, or Reader set")
	}
}

// storyPayload is the internal multipart-ready payload for postStory.
type storyPayload struct {
	BusinessConnectionID string             `json:"business_connection_id"`
	Content              string             `json:"content"`
	ActivePeriod         int                `json:"active_period,omitempty"`
	Caption              string             `json:"caption,omitempty"`
	ParseMode            string             `json:"parse_mode,omitempty"`
	CaptionEntities      []tg.MessageEntity `json:"caption_entities,omitempty"`
	ProtectContent       bool               `json:"protect_content,omitempty"`
	AttachedFiles        []FilePart         `json:"_file_parts"`
}

// buildStoryPayload resolves InputStoryContent to a multipart-ready payload.
func buildStoryPayload(req PostStoryRequest) (*storyPayload, error) {
	payload := &storyPayload{
		BusinessConnectionID: req.BusinessConnectionID,
		ActivePeriod:         req.ActivePeriod,
		Caption:              req.Caption,
		ParseMode:            req.ParseMode,
		CaptionEntities:      req.CaptionEntities,
		ProtectContent:       req.ProtectContent,
	}

	contentJSON, err := resolveStoryContent(req.Content, &payload.AttachedFiles)
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	payload.Content = contentJSON

	return payload, nil
}

// editStoryPayload is the internal multipart-ready payload for editStory.
type editStoryPayload struct {
	BusinessConnectionID string             `json:"business_connection_id"`
	StoryID              int                `json:"story_id"`
	Content              string             `json:"content,omitempty"`
	Caption              string             `json:"caption,omitempty"`
	ParseMode            string             `json:"parse_mode,omitempty"`
	CaptionEntities      []tg.MessageEntity `json:"caption_entities,omitempty"`
	AttachedFiles        []FilePart         `json:"_file_parts"`
}

// buildEditStoryPayload resolves InputStoryContent to a multipart-ready payload.
func buildEditStoryPayload(req EditStoryRequest) (*editStoryPayload, error) {
	payload := &editStoryPayload{
		BusinessConnectionID: req.BusinessConnectionID,
		StoryID:              req.StoryID,
		Caption:              req.Caption,
		ParseMode:            req.ParseMode,
		CaptionEntities:      req.CaptionEntities,
	}

	if req.Content != nil {
		contentJSON, err := resolveStoryContent(req.Content, &payload.AttachedFiles)
		if err != nil {
			return nil, fmt.Errorf("content: %w", err)
		}
		payload.Content = contentJSON
	}

	return payload, nil
}

// resolveStoryContent converts InputStoryContent to a JSON string and collects file parts.
func resolveStoryContent(content InputStoryContent, files *[]FilePart) (string, error) {
	switch c := content.(type) {
	case *InputStoryContentPhoto:
		ref, fp, err := resolveInputFile(c.Photo, "story_photo")
		if err != nil {
			return "", fmt.Errorf("photo: %w", err)
		}
		if fp != nil {
			*files = append(*files, *fp)
		}
		data, err := json.Marshal(map[string]any{"type": "photo", "photo": ref})
		return string(data), err

	case *InputStoryContentVideo:
		ref, fp, err := resolveInputFile(c.Video, "story_video")
		if err != nil {
			return "", fmt.Errorf("video: %w", err)
		}
		if fp != nil {
			*files = append(*files, *fp)
		}
		m := map[string]any{"type": "video", "video": ref}
		if c.Duration > 0 {
			m["duration"] = c.Duration
		}
		if c.CoverFrameTime > 0 {
			m["cover_frame_time"] = c.CoverFrameTime
		}
		if c.IsAnimation {
			m["is_animation"] = true
		}
		data, err := json.Marshal(m)
		return string(data), err

	default:
		return "", fmt.Errorf("unsupported InputStoryContent type: %T", content)
	}
}

// profilePhotoPayload is the internal multipart-ready payload for setBusinessAccountProfilePhoto.
type profilePhotoPayload struct {
	BusinessConnectionID string     `json:"business_connection_id"`
	Photo                string     `json:"photo"`
	IsPublic             bool       `json:"is_public,omitempty"`
	AttachedFiles        []FilePart `json:"_file_parts"`
}

// buildProfilePhotoPayload resolves InputProfilePhoto to a multipart-ready payload.
func buildProfilePhotoPayload(req SetBusinessAccountProfilePhotoRequest) (*profilePhotoPayload, error) {
	payload := &profilePhotoPayload{
		BusinessConnectionID: req.BusinessConnectionID,
		IsPublic:             req.IsPublic,
	}

	switch p := req.Photo.(type) {
	case *InputProfilePhotoStatic:
		ref, fp, err := resolveInputFile(p.Photo, "profile_photo")
		if err != nil {
			return nil, fmt.Errorf("photo: %w", err)
		}
		if fp != nil {
			payload.AttachedFiles = append(payload.AttachedFiles, *fp)
		}
		data, err := json.Marshal(map[string]any{"type": "static", "photo": ref})
		if err != nil {
			return nil, err
		}
		payload.Photo = string(data)

	case *InputProfilePhotoAnimated:
		ref, fp, err := resolveInputFile(p.Animation, "profile_animation")
		if err != nil {
			return nil, fmt.Errorf("animation: %w", err)
		}
		if fp != nil {
			payload.AttachedFiles = append(payload.AttachedFiles, *fp)
		}
		m := map[string]any{"type": "animated", "animation": ref}
		if p.MainFrameTime > 0 {
			m["main_frame_time"] = p.MainFrameTime
		}
		data, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		payload.Photo = string(data)

	default:
		return nil, fmt.Errorf("unsupported InputProfilePhoto type: %T", req.Photo)
	}

	return payload, nil
}
