package sender

import (
	"context"
	"encoding/json"

	"github.com/prilive-com/galigo/tg"
)

// ================== Bot Identity Methods ==================

// GetMe returns basic information about the bot.
func (c *Client) GetMe(ctx context.Context) (*tg.User, error) {
	resp, err := c.executeRequest(ctx, "getMe", struct{}{})
	if err != nil {
		return nil, err
	}
	var user tg.User
	if err := json.Unmarshal(resp.Result, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// LogOut logs out from the cloud Bot API server.
// After a successful call, you can use the local Bot API server.
func (c *Client) LogOut(ctx context.Context) error {
	_, err := c.executeRequest(ctx, "logOut", struct{}{})
	return err
}

// CloseBot closes the bot instance on Telegram servers.
// Used before moving to a local Bot API server.
// Note: This is different from Client.Close() which releases local resources.
func (c *Client) CloseBot(ctx context.Context) error {
	_, err := c.executeRequest(ctx, "close", struct{}{})
	return err
}

// ================== Media Methods ==================

// SendDocument sends a document.
func (c *Client) SendDocument(ctx context.Context, req SendDocumentRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendDocument", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendVideo sends a video.
func (c *Client) SendVideo(ctx context.Context, req SendVideoRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendVideo", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendAudio sends an audio file.
func (c *Client) SendAudio(ctx context.Context, req SendAudioRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendAudio", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendVoice sends a voice message.
func (c *Client) SendVoice(ctx context.Context, req SendVoiceRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendVoice", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendAnimation sends an animation (GIF or H.264/MPEG-4 AVC video without sound).
func (c *Client) SendAnimation(ctx context.Context, req SendAnimationRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendAnimation", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendVideoNote sends a video note (round video up to 1 minute).
func (c *Client) SendVideoNote(ctx context.Context, req SendVideoNoteRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendVideoNote", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendSticker sends a sticker.
func (c *Client) SendSticker(ctx context.Context, req SendStickerRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendSticker", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendMediaGroup sends a group of photos, videos, documents or audios as an album.
func (c *Client) SendMediaGroup(ctx context.Context, req SendMediaGroupRequest) ([]*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendMediaGroup", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	var messages []*tg.Message
	if err := json.Unmarshal(resp.Result, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// ================== Utility Methods ==================

// GetFile returns basic info about a file and prepares it for downloading.
func (c *Client) GetFile(ctx context.Context, fileID string) (*tg.File, error) {
	resp, err := c.executeRequest(ctx, "getFile", GetFileRequest{FileID: fileID})
	if err != nil {
		return nil, err
	}
	var file tg.File
	if err := json.Unmarshal(resp.Result, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

// SendChatAction sends a chat action (typing, upload_photo, etc.).
func (c *Client) SendChatAction(ctx context.Context, chatID tg.ChatID, action string) error {
	_, err := c.executeRequest(ctx, "sendChatAction", SendChatActionRequest{
		ChatID: chatID,
		Action: action,
	}, extractChatID(chatID))
	return err
}

// GetUserProfilePhotos returns a user's profile pictures.
func (c *Client) GetUserProfilePhotos(ctx context.Context, userID int64, opts ...GetUserProfilePhotosOption) (*tg.UserProfilePhotos, error) {
	req := GetUserProfilePhotosRequest{UserID: userID}
	for _, opt := range opts {
		opt(&req)
	}
	resp, err := c.executeRequest(ctx, "getUserProfilePhotos", req)
	if err != nil {
		return nil, err
	}
	var photos tg.UserProfilePhotos
	if err := json.Unmarshal(resp.Result, &photos); err != nil {
		return nil, err
	}
	return &photos, nil
}

// ================== Location/Contact Methods ==================

// SendLocation sends a location.
func (c *Client) SendLocation(ctx context.Context, req SendLocationRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendLocation", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendVenue sends a venue.
func (c *Client) SendVenue(ctx context.Context, req SendVenueRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendVenue", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendContact sends a phone contact.
func (c *Client) SendContact(ctx context.Context, req SendContactRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendContact", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendPoll sends a native poll.
func (c *Client) SendPoll(ctx context.Context, req SendPollRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendPoll", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// SendDice sends an animated emoji that displays a random value.
func (c *Client) SendDice(ctx context.Context, chatID tg.ChatID, opts ...SendDiceOption) (*tg.Message, error) {
	req := SendDiceRequest{ChatID: chatID}
	for _, opt := range opts {
		opt(&req)
	}
	resp, err := c.executeRequest(ctx, "sendDice", req, extractChatID(chatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// ================== Bulk Operations ==================

// ForwardMessages forwards multiple messages at once.
func (c *Client) ForwardMessages(ctx context.Context, req ForwardMessagesRequest) ([]tg.MessageID, error) {
	resp, err := c.executeRequest(ctx, "forwardMessages", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	var ids []tg.MessageID
	if err := json.Unmarshal(resp.Result, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// CopyMessages copies multiple messages at once.
func (c *Client) CopyMessages(ctx context.Context, req CopyMessagesRequest) ([]tg.MessageID, error) {
	resp, err := c.executeRequest(ctx, "copyMessages", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	var ids []tg.MessageID
	if err := json.Unmarshal(resp.Result, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// DeleteMessages deletes multiple messages at once.
func (c *Client) DeleteMessages(ctx context.Context, chatID tg.ChatID, messageIDs []int) error {
	_, err := c.executeRequest(ctx, "deleteMessages", DeleteMessagesRequest{
		ChatID:     chatID,
		MessageIDs: messageIDs,
	}, extractChatID(chatID))
	return err
}

// SetMessageReaction sets a reaction on a message.
func (c *Client) SetMessageReaction(ctx context.Context, req SetMessageReactionRequest) error {
	_, err := c.executeRequest(ctx, "setMessageReaction", req, extractChatID(req.ChatID))
	return err
}

// ================== Options ==================

// GetUserProfilePhotosOption configures GetUserProfilePhotos.
type GetUserProfilePhotosOption func(*GetUserProfilePhotosRequest)

// WithPhotosOffset sets the offset for user profile photos.
func WithPhotosOffset(offset int) GetUserProfilePhotosOption {
	return func(r *GetUserProfilePhotosRequest) {
		r.Offset = offset
	}
}

// WithPhotosLimit sets the limit for user profile photos.
func WithPhotosLimit(limit int) GetUserProfilePhotosOption {
	return func(r *GetUserProfilePhotosRequest) {
		r.Limit = limit
	}
}

// SendDiceOption configures SendDice.
type SendDiceOption func(*SendDiceRequest)

// WithDiceEmoji sets the emoji for the dice.
func WithDiceEmoji(emoji string) SendDiceOption {
	return func(r *SendDiceRequest) {
		r.Emoji = emoji
	}
}
