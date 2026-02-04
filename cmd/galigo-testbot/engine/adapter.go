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

// ================= Extended: Stickers =================

// GetStickerSet returns a sticker set by name.
func (a *SenderAdapter) GetStickerSet(ctx context.Context, name string) (*tg.StickerSet, error) {
	return a.client.GetStickerSet(ctx, name)
}

// UploadStickerFile uploads a sticker file.
func (a *SenderAdapter) UploadStickerFile(ctx context.Context, userID int64, sticker MediaInput, stickerFormat string) (*tg.File, error) {
	return a.client.UploadStickerFile(ctx, sender.UploadStickerFileRequest{
		UserID:        userID,
		Sticker:       mediaInputToInputFile(sticker),
		StickerFormat: stickerFormat,
	})
}

// CreateNewStickerSet creates a new sticker set.
func (a *SenderAdapter) CreateNewStickerSet(ctx context.Context, userID int64, name, title string, stickers []StickerInput) error {
	inputStickers := make([]sender.InputSticker, len(stickers))
	for i, s := range stickers {
		inputStickers[i] = sender.InputSticker{
			Sticker:   mediaInputToInputFile(s.Sticker),
			Format:    s.Format,
			EmojiList: s.EmojiList,
		}
	}
	return a.client.CreateNewStickerSet(ctx, sender.CreateNewStickerSetRequest{
		UserID:   userID,
		Name:     name,
		Title:    title,
		Stickers: inputStickers,
	})
}

// AddStickerToSet adds a sticker to an existing set.
func (a *SenderAdapter) AddStickerToSet(ctx context.Context, userID int64, name string, sticker StickerInput) error {
	return a.client.AddStickerToSet(ctx, sender.AddStickerToSetRequest{
		UserID: userID,
		Name:   name,
		Sticker: sender.InputSticker{
			Sticker:   mediaInputToInputFile(sticker.Sticker),
			Format:    sticker.Format,
			EmojiList: sticker.EmojiList,
		},
	})
}

// SetStickerPositionInSet moves a sticker in a set.
func (a *SenderAdapter) SetStickerPositionInSet(ctx context.Context, sticker string, position int) error {
	return a.client.SetStickerPositionInSet(ctx, sender.SetStickerPositionInSetRequest{
		Sticker:  sticker,
		Position: position,
	})
}

// DeleteStickerFromSet deletes a sticker from a set.
func (a *SenderAdapter) DeleteStickerFromSet(ctx context.Context, sticker string) error {
	return a.client.DeleteStickerFromSet(ctx, sticker)
}

// SetStickerSetTitle sets the title of a sticker set.
func (a *SenderAdapter) SetStickerSetTitle(ctx context.Context, name, title string) error {
	return a.client.SetStickerSetTitle(ctx, name, title)
}

// DeleteStickerSet deletes a sticker set.
func (a *SenderAdapter) DeleteStickerSet(ctx context.Context, name string) error {
	return a.client.DeleteStickerSet(ctx, name)
}

// SetStickerEmojiList sets the emoji list for a sticker.
func (a *SenderAdapter) SetStickerEmojiList(ctx context.Context, sticker string, emojiList []string) error {
	return a.client.SetStickerEmojiList(ctx, sender.SetStickerEmojiListRequest{
		Sticker:   sticker,
		EmojiList: emojiList,
	})
}

// ReplaceStickerInSet replaces a sticker in a set.
func (a *SenderAdapter) ReplaceStickerInSet(ctx context.Context, userID int64, name, oldSticker string, sticker StickerInput) error {
	return a.client.ReplaceStickerInSet(ctx, sender.ReplaceStickerInSetRequest{
		UserID:     userID,
		Name:       name,
		OldSticker: oldSticker,
		Sticker: sender.InputSticker{
			Sticker:   mediaInputToInputFile(sticker.Sticker),
			Format:    sticker.Format,
			EmojiList: sticker.EmojiList,
		},
	})
}

// ================= Extended: Stars & Payments =================

// GetMyStarBalance returns the bot's Star balance.
func (a *SenderAdapter) GetMyStarBalance(ctx context.Context) (*tg.StarAmount, error) {
	return a.client.GetMyStarBalance(ctx)
}

// GetStarTransactions returns the bot's Star transactions.
func (a *SenderAdapter) GetStarTransactions(ctx context.Context, limit int) (*tg.StarTransactions, error) {
	return a.client.GetStarTransactions(ctx, sender.GetStarTransactionsRequest{Limit: limit})
}

// SendInvoice sends an invoice.
func (a *SenderAdapter) SendInvoice(ctx context.Context, chatID int64, title, description, payload, currency string, prices []tg.LabeledPrice) (*tg.Message, error) {
	return a.client.SendInvoice(ctx, sender.SendInvoiceRequest{
		ChatID:      chatID,
		Title:       title,
		Description: description,
		Payload:     payload,
		Currency:    currency,
		Prices:      prices,
	})
}

// ================= Extended: Gifts =================

// GetAvailableGifts returns available gifts.
func (a *SenderAdapter) GetAvailableGifts(ctx context.Context) (*tg.Gifts, error) {
	return a.client.GetAvailableGifts(ctx)
}

// ================= Extended: Checklists =================

// SendChecklist sends a checklist message.
func (a *SenderAdapter) SendChecklist(ctx context.Context, chatID int64, title string, tasks []string) (*tg.Message, error) {
	inputTasks := make([]tg.InputChecklistTask, len(tasks))
	for i, t := range tasks {
		inputTasks[i] = tg.InputChecklistTask{ID: i + 1, Text: t}
	}
	return a.client.SendChecklist(ctx, sender.SendChecklistRequest{
		ChatID: chatID,
		Checklist: tg.InputChecklist{
			Title: title,
			Tasks: inputTasks,
		},
	})
}

// EditMessageChecklist edits a checklist message.
func (a *SenderAdapter) EditMessageChecklist(ctx context.Context, chatID int64, messageID int, title string, tasks []ChecklistTaskInput) (*tg.Message, error) {
	inputTasks := make([]tg.InputChecklistTask, len(tasks))
	for i, t := range tasks {
		inputTasks[i] = tg.InputChecklistTask{
			ID:   t.ID,
			Text: t.Text,
		}
	}
	return a.client.EditMessageChecklist(ctx, sender.EditMessageChecklistRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Checklist: tg.InputChecklist{
			Title: title,
			Tasks: inputTasks,
		},
	})
}

// ================= Geo & Contact =================

// SendLocation sends a GPS location.
func (a *SenderAdapter) SendLocation(ctx context.Context, chatID int64, lat, lon float64) (*tg.Message, error) {
	return a.client.SendLocation(ctx, sender.SendLocationRequest{
		ChatID:    chatID,
		Latitude:  lat,
		Longitude: lon,
	})
}

// SendVenue sends a venue.
func (a *SenderAdapter) SendVenue(ctx context.Context, chatID int64, lat, lon float64, title, address string) (*tg.Message, error) {
	return a.client.SendVenue(ctx, sender.SendVenueRequest{
		ChatID:    chatID,
		Latitude:  lat,
		Longitude: lon,
		Title:     title,
		Address:   address,
	})
}

// SendContact sends a phone contact.
func (a *SenderAdapter) SendContact(ctx context.Context, chatID int64, phone, firstName, lastName string) (*tg.Message, error) {
	return a.client.SendContact(ctx, sender.SendContactRequest{
		ChatID:      chatID,
		PhoneNumber: phone,
		FirstName:   firstName,
		LastName:    lastName,
	})
}

// SendDice sends an animated dice emoji.
func (a *SenderAdapter) SendDice(ctx context.Context, chatID int64, emoji string) (*tg.Message, error) {
	var opts []sender.SendDiceOption
	if emoji != "" {
		opts = append(opts, sender.WithDiceEmoji(emoji))
	}
	return a.client.SendDice(ctx, chatID, opts...)
}

// ================= Reactions & User Info =================

// SetMessageReaction sets a reaction on a message.
func (a *SenderAdapter) SetMessageReaction(ctx context.Context, chatID int64, messageID int, emoji string, isBig bool) error {
	return a.client.SetMessageReaction(ctx, sender.SetMessageReactionRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Reaction: []sender.ReactionType{
			{Type: "emoji", Emoji: emoji},
		},
		IsBig: isBig,
	})
}

// GetUserProfilePhotos returns a user's profile pictures.
func (a *SenderAdapter) GetUserProfilePhotos(ctx context.Context, userID int64) (*tg.UserProfilePhotos, error) {
	return a.client.GetUserProfilePhotos(ctx, userID)
}

// GetUserChatBoosts returns a user's boosts in the chat.
func (a *SenderAdapter) GetUserChatBoosts(ctx context.Context, chatID, userID int64) (*tg.UserChatBoosts, error) {
	return a.client.GetUserChatBoosts(ctx, sender.GetUserChatBoostsRequest{
		ChatID: chatID,
		UserID: userID,
	})
}

// ================= Bulk Operations =================

// ForwardMessages forwards multiple messages.
func (a *SenderAdapter) ForwardMessages(ctx context.Context, chatID, fromChatID int64, messageIDs []int) ([]tg.MessageID, error) {
	return a.client.ForwardMessages(ctx, sender.ForwardMessagesRequest{
		ChatID:     chatID,
		FromChatID: fromChatID,
		MessageIDs: messageIDs,
	})
}

// CopyMessages copies multiple messages.
func (a *SenderAdapter) CopyMessages(ctx context.Context, chatID, fromChatID int64, messageIDs []int) ([]tg.MessageID, error) {
	return a.client.CopyMessages(ctx, sender.CopyMessagesRequest{
		ChatID:     chatID,
		FromChatID: fromChatID,
		MessageIDs: messageIDs,
	})
}

// DeleteMessages deletes multiple messages.
func (a *SenderAdapter) DeleteMessages(ctx context.Context, chatID int64, messageIDs []int) error {
	return a.client.DeleteMessages(ctx, chatID, messageIDs)
}

// ================= Chat Settings =================

// SetChatPhoto sets the chat photo.
func (a *SenderAdapter) SetChatPhoto(ctx context.Context, chatID int64, photo sender.InputFile) error {
	return a.client.SetChatPhoto(ctx, chatID, photo)
}

// DeleteChatPhoto deletes the chat photo.
func (a *SenderAdapter) DeleteChatPhoto(ctx context.Context, chatID int64) error {
	return a.client.DeleteChatPhoto(ctx, chatID)
}

// SetChatPermissions sets the default chat permissions.
func (a *SenderAdapter) SetChatPermissions(ctx context.Context, chatID int64, perms tg.ChatPermissions) error {
	return a.client.SetChatPermissions(ctx, chatID, perms)
}

// ================= Bot Identity =================

// SetMyCommands sets the bot's command list.
func (a *SenderAdapter) SetMyCommands(ctx context.Context, commands []tg.BotCommand) error {
	return a.client.SetMyCommands(ctx, commands)
}

// GetMyCommands returns the bot's command list.
func (a *SenderAdapter) GetMyCommands(ctx context.Context) ([]tg.BotCommand, error) {
	return a.client.GetMyCommands(ctx)
}

// DeleteMyCommands removes the bot's command list.
func (a *SenderAdapter) DeleteMyCommands(ctx context.Context) error {
	return a.client.DeleteMyCommands(ctx)
}

// SetMyName sets the bot's name.
func (a *SenderAdapter) SetMyName(ctx context.Context, name string) error {
	return a.client.SetMyName(ctx, name)
}

// GetMyName returns the bot's name.
func (a *SenderAdapter) GetMyName(ctx context.Context) (*tg.BotName, error) {
	return a.client.GetMyName(ctx)
}

// SetMyDescription sets the bot's description.
func (a *SenderAdapter) SetMyDescription(ctx context.Context, description string) error {
	return a.client.SetMyDescription(ctx, description)
}

// GetMyDescription returns the bot's description.
func (a *SenderAdapter) GetMyDescription(ctx context.Context) (*tg.BotDescription, error) {
	return a.client.GetMyDescription(ctx)
}

// SetMyShortDescription sets the bot's short description.
func (a *SenderAdapter) SetMyShortDescription(ctx context.Context, shortDescription string) error {
	return a.client.SetMyShortDescription(ctx, shortDescription)
}

// GetMyShortDescription returns the bot's short description.
func (a *SenderAdapter) GetMyShortDescription(ctx context.Context) (*tg.BotShortDescription, error) {
	return a.client.GetMyShortDescription(ctx)
}

// SetMyDefaultAdministratorRights sets the bot's default admin rights.
func (a *SenderAdapter) SetMyDefaultAdministratorRights(ctx context.Context, rights *tg.ChatAdministratorRights, forChannels bool) error {
	var opts []sender.AdminRightsOption
	if rights != nil {
		opts = append(opts, sender.WithAdminRights(*rights))
	}
	if forChannels {
		opts = append(opts, sender.ForChannels())
	}
	return a.client.SetMyDefaultAdministratorRights(ctx, opts...)
}

// GetMyDefaultAdministratorRights returns the bot's default admin rights.
func (a *SenderAdapter) GetMyDefaultAdministratorRights(ctx context.Context, forChannels bool) (*tg.ChatAdministratorRights, error) {
	return a.client.GetMyDefaultAdministratorRights(ctx, forChannels)
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
