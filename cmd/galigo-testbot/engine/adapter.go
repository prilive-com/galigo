package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// SenderAdapter adapts sender.Client to SenderClient interface.
type SenderAdapter struct {
	client     *sender.Client
	token      tg.SecretToken
	httpClient *http.Client
}

// NewSenderAdapter creates a new adapter wrapping a sender.Client.
func NewSenderAdapter(client *sender.Client) *SenderAdapter {
	return &SenderAdapter{client: client}
}

// WithToken sets the token for webhook/polling operations.
func (a *SenderAdapter) WithToken(token tg.SecretToken) *SenderAdapter {
	a.token = token
	return a
}

// WithHTTPClient sets the HTTP client for webhook/polling operations.
func (a *SenderAdapter) WithHTTPClient(client *http.Client) *SenderAdapter {
	a.httpClient = client
	return a
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
		return sender.FromBytes(m.Reader(), m.FileName)
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

// AnswerCallbackQuery answers a callback query.
func (a *SenderAdapter) AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string, showAlert bool) error {
	return a.client.AnswerCallbackQuery(ctx, sender.AnswerCallbackQueryRequest{
		CallbackQueryID: callbackQueryID,
		Text:            text,
		ShowAlert:       showAlert,
	})
}

// ================= Tier 2: Chat Info =================

// GetChat gets full chat info.
func (a *SenderAdapter) GetChat(ctx context.Context, chatID int64) (*tg.ChatFullInfo, error) {
	return a.client.GetChat(ctx, chatID)
}

// GetChatAdministrators returns chat admins.
func (a *SenderAdapter) GetChatAdministrators(ctx context.Context, chatID int64) ([]tg.ChatMember, error) {
	return a.client.GetChatAdministrators(ctx, chatID)
}

// GetChatMemberCount returns the member count.
func (a *SenderAdapter) GetChatMemberCount(ctx context.Context, chatID int64) (int, error) {
	return a.client.GetChatMemberCount(ctx, chatID)
}

// GetChatMember returns info about a member.
func (a *SenderAdapter) GetChatMember(ctx context.Context, chatID int64, userID int64) (tg.ChatMember, error) {
	return a.client.GetChatMember(ctx, chatID, userID)
}

// ================= Tier 2: Chat Settings =================

// SetChatTitle sets the chat title.
func (a *SenderAdapter) SetChatTitle(ctx context.Context, chatID int64, title string) error {
	return a.client.SetChatTitle(ctx, chatID, title)
}

// SetChatDescription sets the chat description.
func (a *SenderAdapter) SetChatDescription(ctx context.Context, chatID int64, description string) error {
	return a.client.SetChatDescription(ctx, chatID, description)
}

// ================= Tier 2: Pin Messages =================

// PinChatMessage pins a message.
func (a *SenderAdapter) PinChatMessage(ctx context.Context, chatID int64, messageID int, silent bool) error {
	var opts []sender.PinOption
	if silent {
		opts = append(opts, sender.WithSilentPin())
	}
	return a.client.PinChatMessage(ctx, chatID, messageID, opts...)
}

// UnpinChatMessage unpins a message.
func (a *SenderAdapter) UnpinChatMessage(ctx context.Context, chatID int64, messageID int) error {
	return a.client.UnpinChatMessage(ctx, chatID, messageID)
}

// UnpinAllChatMessages unpins all messages.
func (a *SenderAdapter) UnpinAllChatMessages(ctx context.Context, chatID int64) error {
	return a.client.UnpinAllChatMessages(ctx, chatID)
}

// ================= Tier 2: Polls =================

// SendPollSimple sends a simple poll.
func (a *SenderAdapter) SendPollSimple(ctx context.Context, chatID int64, question string, options []string) (*tg.Message, error) {
	return a.client.SendPollSimple(ctx, chatID, question, options)
}

// SendQuiz sends a quiz poll.
func (a *SenderAdapter) SendQuiz(ctx context.Context, chatID int64, question string, options []string, correctOptionID int) (*tg.Message, error) {
	return a.client.SendQuiz(ctx, chatID, question, options, correctOptionID)
}

// StopPoll stops a poll.
func (a *SenderAdapter) StopPoll(ctx context.Context, chatID int64, messageID int) (*tg.Poll, error) {
	return a.client.StopPoll(ctx, chatID, messageID)
}

// ================= Tier 2: Forum =================

// GetForumTopicIconStickers gets available forum topic icon stickers.
func (a *SenderAdapter) GetForumTopicIconStickers(ctx context.Context) ([]*tg.Sticker, error) {
	return a.client.GetForumTopicIconStickers(ctx)
}

// SetWebhook sets a webhook URL.
func (a *SenderAdapter) SetWebhook(ctx context.Context, webhookURL string) error {
	return receiver.SetWebhook(ctx, a.httpClient, a.token, webhookURL, "")
}

// DeleteWebhook removes the webhook.
func (a *SenderAdapter) DeleteWebhook(ctx context.Context) error {
	return receiver.DeleteWebhook(ctx, a.httpClient, a.token, false)
}

// GetWebhookInfo retrieves webhook configuration.
func (a *SenderAdapter) GetWebhookInfo(ctx context.Context) (*WebhookInfo, error) {
	info, err := receiver.GetWebhookInfo(ctx, a.httpClient, a.token)
	if err != nil {
		return nil, err
	}
	return &WebhookInfo{
		URL:                info.URL,
		PendingUpdateCount: info.PendingUpdateCount,
		HasCustomCert:      info.HasCustomCertificate,
	}, nil
}

// GetUpdates calls the getUpdates API directly.
func (a *SenderAdapter) GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]tg.Update, error) {
	params := url.Values{}
	params.Set("offset", strconv.FormatInt(offset, 10))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("timeout", strconv.Itoa(timeout))

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?%s", a.token.Value(), params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	client := a.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var result struct {
		OK          bool        `json:"ok"`
		Result      []tg.Update `json:"result"`
		ErrorCode   int         `json:"error_code,omitempty"`
		Description string      `json:"description,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode getUpdates response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("getUpdates failed: %d %s", result.ErrorCode, result.Description)
	}
	return result.Result, nil
}

// Ensure SenderAdapter implements SenderClient.
var _ SenderClient = (*SenderAdapter)(nil)
