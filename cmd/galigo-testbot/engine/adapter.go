package engine

import (
	"bytes"
	"context"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// SenderAdapter adapts sender.Client to SenderClient interface.
type SenderAdapter struct {
	client *sender.Client
}

// NewSenderAdapter creates a new adapter wrapping a sender.Client.
func NewSenderAdapter(client *sender.Client) *SenderAdapter {
	return &SenderAdapter{client: client}
}

// GetMe returns basic information about the bot.
func (a *SenderAdapter) GetMe(ctx context.Context) (*tg.User, error) {
	return a.client.GetMe(ctx)
}

// SendMessage sends a text message.
func (a *SenderAdapter) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error) {
	options := &SendOptions{}
	for _, opt := range opts {
		opt(options)
	}

	req := sender.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: tg.ParseMode(options.ParseMode),
	}

	if options.ReplyMarkup != nil {
		req.ReplyMarkup = options.ReplyMarkup
	}

	return a.client.SendMessage(ctx, req)
}

// EditMessageText edits a message's text.
func (a *SenderAdapter) EditMessageText(ctx context.Context, chatID int64, messageID int, text string) (*tg.Message, error) {
	return a.client.EditMessageText(ctx, sender.EditMessageTextRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	})
}

// DeleteMessage deletes a message.
func (a *SenderAdapter) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	return a.client.DeleteMessage(ctx, sender.DeleteMessageRequest{
		ChatID:    chatID,
		MessageID: messageID,
	})
}

// ForwardMessage forwards a message.
func (a *SenderAdapter) ForwardMessage(ctx context.Context, chatID, fromChatID int64, messageID int) (*tg.Message, error) {
	return a.client.ForwardMessage(ctx, sender.ForwardMessageRequest{
		ChatID:     chatID,
		FromChatID: fromChatID,
		MessageID:  messageID,
	})
}

// CopyMessage copies a message.
func (a *SenderAdapter) CopyMessage(ctx context.Context, chatID, fromChatID int64, messageID int) (*tg.MessageID, error) {
	return a.client.CopyMessage(ctx, sender.CopyMessageRequest{
		ChatID:     chatID,
		FromChatID: fromChatID,
		MessageID:  messageID,
	})
}

// SendChatAction sends a chat action.
func (a *SenderAdapter) SendChatAction(ctx context.Context, chatID int64, action string) error {
	return a.client.SendChatAction(ctx, chatID, action)
}

// mediaInputToInputFile converts MediaInput to sender.InputFile.
func mediaInputToInputFile(m MediaInput) sender.InputFile {
	if m.FileID != "" {
		return sender.FromFileID(m.FileID)
	}
	if m.URL != "" {
		return sender.FromURL(m.URL)
	}
	if m.Reader != nil {
		return sender.FromReader(bytes.NewReader(m.Reader()), m.FileName)
	}
	return sender.InputFile{}
}

// SendPhoto sends a photo.
func (a *SenderAdapter) SendPhoto(ctx context.Context, chatID int64, photo MediaInput, caption string) (*tg.Message, error) {
	return a.client.SendPhoto(ctx, sender.SendPhotoRequest{
		ChatID:  chatID,
		Photo:   mediaInputToInputFile(photo),
		Caption: caption,
	})
}

// SendDocument sends a document.
func (a *SenderAdapter) SendDocument(ctx context.Context, chatID int64, document MediaInput, caption string) (*tg.Message, error) {
	return a.client.SendDocument(ctx, sender.SendDocumentRequest{
		ChatID:   chatID,
		Document: mediaInputToInputFile(document),
		Caption:  caption,
	})
}

// SendAnimation sends an animation (GIF).
func (a *SenderAdapter) SendAnimation(ctx context.Context, chatID int64, animation MediaInput, caption string) (*tg.Message, error) {
	return a.client.SendAnimation(ctx, sender.SendAnimationRequest{
		ChatID:    chatID,
		Animation: mediaInputToInputFile(animation),
		Caption:   caption,
	})
}

// SendVideo sends a video.
func (a *SenderAdapter) SendVideo(ctx context.Context, chatID int64, video MediaInput, caption string) (*tg.Message, error) {
	return a.client.SendVideo(ctx, sender.SendVideoRequest{
		ChatID:  chatID,
		Video:   mediaInputToInputFile(video),
		Caption: caption,
	})
}

// SendAudio sends an audio file.
func (a *SenderAdapter) SendAudio(ctx context.Context, chatID int64, audio MediaInput, caption string) (*tg.Message, error) {
	return a.client.SendAudio(ctx, sender.SendAudioRequest{
		ChatID:  chatID,
		Audio:   mediaInputToInputFile(audio),
		Caption: caption,
	})
}

// SendVoice sends a voice message.
func (a *SenderAdapter) SendVoice(ctx context.Context, chatID int64, voice MediaInput, caption string) (*tg.Message, error) {
	return a.client.SendVoice(ctx, sender.SendVoiceRequest{
		ChatID:  chatID,
		Voice:   mediaInputToInputFile(voice),
		Caption: caption,
	})
}

// SendSticker sends a sticker.
func (a *SenderAdapter) SendSticker(ctx context.Context, chatID int64, sticker MediaInput) (*tg.Message, error) {
	return a.client.SendSticker(ctx, sender.SendStickerRequest{
		ChatID:  chatID,
		Sticker: mediaInputToInputFile(sticker),
	})
}

// SendVideoNote sends a video note (round video).
func (a *SenderAdapter) SendVideoNote(ctx context.Context, chatID int64, videoNote MediaInput) (*tg.Message, error) {
	return a.client.SendVideoNote(ctx, sender.SendVideoNoteRequest{
		ChatID:    chatID,
		VideoNote: mediaInputToInputFile(videoNote),
	})
}

// SendMediaGroup sends a media group (album).
func (a *SenderAdapter) SendMediaGroup(ctx context.Context, chatID int64, media []MediaInput) ([]*tg.Message, error) {
	inputFiles := make([]sender.InputFile, len(media))
	for i, m := range media {
		f := mediaInputToInputFile(m)
		f.MediaType = m.Type
		inputFiles[i] = f
	}
	return a.client.SendMediaGroup(ctx, sender.SendMediaGroupRequest{
		ChatID: chatID,
		Media:  inputFiles,
	})
}

// GetFile gets file info for download.
func (a *SenderAdapter) GetFile(ctx context.Context, fileID string) (*tg.File, error) {
	return a.client.GetFile(ctx, fileID)
}

// EditMessageCaption edits a message's caption.
func (a *SenderAdapter) EditMessageCaption(ctx context.Context, chatID int64, messageID int, caption string) (*tg.Message, error) {
	return a.client.EditMessageCaption(ctx, sender.EditMessageCaptionRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Caption:   caption,
	})
}

// EditMessageReplyMarkup edits a message's reply markup.
func (a *SenderAdapter) EditMessageReplyMarkup(ctx context.Context, chatID int64, messageID int, markup *tg.InlineKeyboardMarkup) (*tg.Message, error) {
	return a.client.EditMessageReplyMarkup(ctx, sender.EditMessageReplyMarkupRequest{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: markup,
	})
}

// EditMessageMedia edits a message's media content.
func (a *SenderAdapter) EditMessageMedia(ctx context.Context, chatID int64, messageID int, media sender.InputMedia) (*tg.Message, error) {
	return a.client.EditMessageMedia(ctx, sender.EditMessageMediaRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Media:     media,
	})
}

// Ensure SenderAdapter implements SenderClient.
var _ SenderClient = (*SenderAdapter)(nil)
