package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// GetMeStep verifies bot identity.
type GetMeStep struct{}

func (s *GetMeStep) Name() string { return "getMe" }

func (s *GetMeStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	user, err := rt.Sender.GetMe(ctx)
	if err != nil {
		return nil, err
	}

	if !user.IsBot {
		return nil, fmt.Errorf("expected bot, got user")
	}

	return &StepResult{
		Method: "getMe",
		Evidence: map[string]any{
			"username":   user.Username,
			"id":         user.ID,
			"first_name": user.FirstName,
		},
	}, nil
}

// SendMessageStep sends a text message.
type SendMessageStep struct {
	Text string
}

func (s *SendMessageStep) Name() string { return "sendMessage" }

func (s *SendMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, s.Text)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"text":       s.Text,
		},
	}, nil
}

// SendFormattedMessageStep sends a text message with ParseMode.
// This validates that named string types (tg.ParseMode) serialize correctly.
// See: https://github.com/prilive-com/galigo/issues/5
type SendFormattedMessageStep struct {
	Text      string
	ParseMode string // "Markdown", "MarkdownV2", "HTML"
}

func (s *SendFormattedMessageStep) Name() string { return "sendMessage" }

func (s *SendFormattedMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, s.Text, WithParseMode(s.ParseMode))
	if err != nil {
		return nil, fmt.Errorf("sendMessage with ParseMode %q: %w", s.ParseMode, err)
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"text":       s.Text,
			"parse_mode": s.ParseMode,
		},
	}, nil
}

// SendMessageWithLinkPreviewStep sends a message with LinkPreviewOptions.
// This validates that link_preview_options serializes correctly and is accepted by Telegram.
// NOTE: We do NOT assert on preview rendering (whether it shows up, size, position) since
// link_preview_options are rendering hints that Telegram may ignore.
// See: https://github.com/prilive-com/galigo/issues/6
type SendMessageWithLinkPreviewStep struct {
	Text               string
	LinkPreviewOptions *tg.LinkPreviewOptions
}

func (s *SendMessageWithLinkPreviewStep) Name() string { return "sendMessage (link_preview_options)" }

func (s *SendMessageWithLinkPreviewStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, s.Text, WithLinkPreviewOptions(s.LinkPreviewOptions))
	if err != nil {
		return nil, fmt.Errorf("sendMessage with LinkPreviewOptions: %w", err)
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id":           msg.MessageID,
			"text":                 s.Text,
			"link_preview_options": s.LinkPreviewOptions,
		},
	}, nil
}

// SendPhotoWithParseModeStep tests ParseMode in multipart requests.
// This catches the named-string-type serialization bug in BuildMultipartRequest.
// Uses tg.ParseMode for compile-time safety.
// See: https://github.com/prilive-com/galigo/issues/5
type SendPhotoWithParseModeStep struct {
	Photo     MediaInput   // Required: pass via MediaFromBytes(fixtures.PhotoBytes(), ...)
	Caption   string
	ParseMode tg.ParseMode // Use actual type for compile-time safety
}

func (s *SendPhotoWithParseModeStep) Name() string { return "sendPhoto" }

func (s *SendPhotoWithParseModeStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendPhoto(ctx, rt.ChatID, s.Photo,
		WithCaption(s.Caption),
		WithParseMode(string(s.ParseMode)),
	)
	if err != nil {
		return nil, fmt.Errorf("sendPhoto with ParseMode %q: %w", s.ParseMode, err)
	}

	// Verify formatting was applied (caption entities exist)
	if len(msg.CaptionEntities) == 0 {
		return nil, fmt.Errorf("sendPhoto: ParseMode %q accepted but no caption entities returned", s.ParseMode)
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Capture file_id for reuse
	var fileID string
	if len(msg.Photo) > 0 {
		fileID = msg.Photo[len(msg.Photo)-1].FileID
		rt.CapturedFileIDs["photo"] = fileID
	}

	return &StepResult{
		Method:     "sendPhoto",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id":           msg.MessageID,
			"caption":              s.Caption,
			"parse_mode":           s.ParseMode,
			"caption_entity_count": len(msg.CaptionEntities),
			"file_id":              fileID,
		},
	}, nil
}

// EditMessageTextStep edits the last message's text.
type EditMessageTextStep struct {
	Text string
}

func (s *EditMessageTextStep) Name() string { return "editMessageText" }

func (s *EditMessageTextStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to edit")
	}

	msg, err := rt.Sender.EditMessageText(ctx, rt.ChatID, rt.LastMessage.MessageID, s.Text)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg

	return &StepResult{
		Method: "editMessageText",
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"new_text":   s.Text,
		},
	}, nil
}

// DeleteLastMessageStep deletes the last sent message.
type DeleteLastMessageStep struct{}

func (s *DeleteLastMessageStep) Name() string { return "deleteMessage" }

func (s *DeleteLastMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to delete")
	}

	err := rt.Sender.DeleteMessage(ctx, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	// Remove from tracked messages since we deleted it
	for i, cm := range rt.CreatedMessages {
		if cm.MessageID == rt.LastMessage.MessageID && cm.ChatID == rt.ChatID {
			rt.CreatedMessages = append(rt.CreatedMessages[:i], rt.CreatedMessages[i+1:]...)
			break
		}
	}

	msgID := rt.LastMessage.MessageID
	rt.LastMessage = nil

	return &StepResult{
		Method: "deleteMessage",
		Evidence: map[string]any{
			"deleted_message_id": msgID,
		},
	}, nil
}

// ForwardMessageStep forwards the last message.
type ForwardMessageStep struct {
	ToChatID int64 // If 0, uses rt.ChatID
}

func (s *ForwardMessageStep) Name() string { return "forwardMessage" }

func (s *ForwardMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to forward")
	}

	toChatID := s.ToChatID
	if toChatID == 0 {
		toChatID = rt.ChatID
	}

	msg, err := rt.Sender.ForwardMessage(ctx, toChatID, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	rt.TrackMessage(toChatID, msg.MessageID)

	return &StepResult{
		Method:     "forwardMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"original_message_id":  rt.LastMessage.MessageID,
			"forwarded_message_id": msg.MessageID,
			"to_chat_id":           toChatID,
		},
	}, nil
}

// CopyMessageStep copies the last message.
type CopyMessageStep struct {
	ToChatID int64 // If 0, uses rt.ChatID
}

func (s *CopyMessageStep) Name() string { return "copyMessage" }

func (s *CopyMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to copy")
	}

	toChatID := s.ToChatID
	if toChatID == 0 {
		toChatID = rt.ChatID
	}

	msgID, err := rt.Sender.CopyMessage(ctx, toChatID, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	rt.LastMessageID = msgID
	rt.TrackMessage(toChatID, msgID.MessageID)

	return &StepResult{
		Method:     "copyMessage",
		MessageIDs: []int{msgID.MessageID},
		Evidence: map[string]any{
			"original_message_id": rt.LastMessage.MessageID,
			"copied_message_id":   msgID.MessageID,
			"to_chat_id":          toChatID,
		},
	}, nil
}

// SendChatActionStep sends a chat action.
type SendChatActionStep struct {
	Action string // e.g., "typing"
}

func (s *SendChatActionStep) Name() string { return "sendChatAction" }

func (s *SendChatActionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	action := s.Action
	if action == "" {
		action = "typing"
	}

	err := rt.Sender.SendChatAction(ctx, rt.ChatID, action)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "sendChatAction",
		Evidence: map[string]any{
			"action": action,
		},
	}, nil
}

// CleanupStep deletes all tracked messages.
type CleanupStep struct{}

func (s *CleanupStep) Name() string { return "cleanup" }

func (s *CleanupStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	deleted := 0
	var lastErr error

	for _, cm := range rt.CreatedMessages {
		if err := rt.Sender.DeleteMessage(ctx, cm.ChatID, cm.MessageID); err != nil {
			lastErr = err
			// Continue trying to delete other messages
		} else {
			deleted++
		}
	}

	// Clear the list
	rt.CreatedMessages = rt.CreatedMessages[:0]
	rt.LastMessage = nil

	// Clean up sticker sets (best-effort)
	for _, name := range rt.CreatedStickerSets {
		_ = rt.Sender.DeleteStickerSet(ctx, name)
	}
	rt.CreatedStickerSets = rt.CreatedStickerSets[:0]

	result := &StepResult{
		Method: "deleteMessage",
		Evidence: map[string]any{
			"deleted_count": deleted,
		},
	}

	// Only fail if we couldn't delete any messages and there were messages to delete
	if lastErr != nil && deleted == 0 {
		return result, lastErr
	}

	return result, nil
}

// ================= Phase C: Keyboard Steps =================

// SendMessageWithKeyboardStep sends a message with an inline keyboard.
type SendMessageWithKeyboardStep struct {
	Text    string
	Buttons []ButtonDef
}

// ButtonDef defines a button for inline keyboards.
type ButtonDef struct {
	Text         string
	CallbackData string
}

func (s *SendMessageWithKeyboardStep) Name() string { return "sendMessage (keyboard)" }

func (s *SendMessageWithKeyboardStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	markup := buildKeyboard(s.Buttons)
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, s.Text, WithReplyMarkup(markup))
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id":   msg.MessageID,
			"text":         s.Text,
			"has_keyboard": msg.ReplyMarkup != nil,
		},
	}, nil
}

// EditMessageReplyMarkupStep edits the reply markup of the last message.
type EditMessageReplyMarkupStep struct {
	Buttons []ButtonDef // nil = remove keyboard
}

func (s *EditMessageReplyMarkupStep) Name() string { return "editMessageReplyMarkup" }

func (s *EditMessageReplyMarkupStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to edit")
	}

	var markup *tg.InlineKeyboardMarkup
	if len(s.Buttons) > 0 {
		markup = buildKeyboard(s.Buttons)
	} else {
		// Empty keyboard removes it
		markup = &tg.InlineKeyboardMarkup{
			InlineKeyboard: [][]tg.InlineKeyboardButton{},
		}
	}

	msg, err := rt.Sender.EditMessageReplyMarkup(ctx, rt.ChatID, rt.LastMessage.MessageID, markup)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg

	return &StepResult{
		Method: "editMessageReplyMarkup",
		Evidence: map[string]any{
			"message_id":      msg.MessageID,
			"keyboard_remove": len(s.Buttons) == 0,
		},
	}, nil
}

func buildKeyboard(buttons []ButtonDef) *tg.InlineKeyboardMarkup {
	var row []tg.InlineKeyboardButton
	for _, b := range buttons {
		row = append(row, tg.InlineKeyboardButton{
			Text:         b.Text,
			CallbackData: b.CallbackData,
		})
	}
	return &tg.InlineKeyboardMarkup{
		InlineKeyboard: [][]tg.InlineKeyboardButton{row},
	}
}

// ================= Phase B: Media Steps =================

// SendPhotoStep sends a photo.
type SendPhotoStep struct {
	Photo   MediaInput
	Caption string
}

func (s *SendPhotoStep) Name() string { return "sendPhoto" }

func (s *SendPhotoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendPhoto(ctx, rt.ChatID, s.Photo, WithCaption(s.Caption))
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Capture file_id for reuse
	var fileID string
	if len(msg.Photo) > 0 {
		fileID = msg.Photo[len(msg.Photo)-1].FileID
		rt.CapturedFileIDs["photo"] = fileID
	}

	return &StepResult{
		Method:     "sendPhoto",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"caption":    s.Caption,
			"file_id":    fileID,
		},
	}, nil
}

// SendDocumentStep sends a document.
type SendDocumentStep struct {
	Document MediaInput
	Caption  string
}

func (s *SendDocumentStep) Name() string { return "sendDocument" }

func (s *SendDocumentStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendDocument(ctx, rt.ChatID, s.Document, s.Caption)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Capture file_id for reuse
	var fileID string
	if msg.Document != nil {
		fileID = msg.Document.FileID
		rt.CapturedFileIDs["document"] = fileID
	}

	return &StepResult{
		Method:     "sendDocument",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"caption":    s.Caption,
			"file_id":    fileID,
		},
	}, nil
}

// SendAnimationStep sends an animation (GIF).
type SendAnimationStep struct {
	Animation MediaInput
	Caption   string
}

func (s *SendAnimationStep) Name() string { return "sendAnimation" }

func (s *SendAnimationStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendAnimation(ctx, rt.ChatID, s.Animation, s.Caption)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Capture file_id for reuse
	var fileID string
	if msg.Animation != nil {
		fileID = msg.Animation.FileID
		rt.CapturedFileIDs["animation"] = fileID
	} else if msg.Document != nil {
		fileID = msg.Document.FileID
		rt.CapturedFileIDs["animation"] = fileID
	}

	return &StepResult{
		Method:     "sendAnimation",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"caption":    s.Caption,
			"file_id":    fileID,
		},
	}, nil
}

// SendVideoStep sends a video.
type SendVideoStep struct {
	Video   MediaInput
	Caption string
}

func (s *SendVideoStep) Name() string { return "sendVideo" }

func (s *SendVideoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendVideo(ctx, rt.ChatID, s.Video, s.Caption)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Capture file_id for reuse
	var fileID string
	if msg.Video != nil {
		fileID = msg.Video.FileID
		rt.CapturedFileIDs["video"] = fileID
	}

	return &StepResult{
		Method:     "sendVideo",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"caption":    s.Caption,
			"file_id":    fileID,
		},
	}, nil
}

// SendAudioStep sends an audio file.
type SendAudioStep struct {
	Audio   MediaInput
	Caption string
}

func (s *SendAudioStep) Name() string { return "sendAudio" }

func (s *SendAudioStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendAudio(ctx, rt.ChatID, s.Audio, s.Caption)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Capture file_id for reuse
	var fileID string
	if msg.Audio != nil {
		fileID = msg.Audio.FileID
		rt.CapturedFileIDs["audio"] = fileID
	}

	return &StepResult{
		Method:     "sendAudio",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"caption":    s.Caption,
			"file_id":    fileID,
		},
	}, nil
}

// SendVoiceStep sends a voice message.
type SendVoiceStep struct {
	Voice   MediaInput
	Caption string
}

func (s *SendVoiceStep) Name() string { return "sendVoice" }

func (s *SendVoiceStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendVoice(ctx, rt.ChatID, s.Voice, s.Caption)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	var fileID string
	if msg.Voice != nil {
		fileID = msg.Voice.FileID
		rt.CapturedFileIDs["voice"] = fileID
	}

	return &StepResult{
		Method:     "sendVoice",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"caption":    s.Caption,
			"file_id":    fileID,
		},
	}, nil
}

// SendStickerStep sends a sticker.
type SendStickerStep struct {
	Sticker MediaInput
}

func (s *SendStickerStep) Name() string { return "sendSticker" }

func (s *SendStickerStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendSticker(ctx, rt.ChatID, s.Sticker)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	var fileID string
	if msg.Sticker != nil {
		fileID = msg.Sticker.FileID
		rt.CapturedFileIDs["sticker"] = fileID
	}

	return &StepResult{
		Method:     "sendSticker",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"file_id":    fileID,
		},
	}, nil
}

// SendVideoNoteStep sends a video note (round video).
type SendVideoNoteStep struct {
	VideoNote MediaInput
}

func (s *SendVideoNoteStep) Name() string { return "sendVideoNote" }

func (s *SendVideoNoteStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendVideoNote(ctx, rt.ChatID, s.VideoNote)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	var fileID string
	if msg.VideoNote != nil {
		fileID = msg.VideoNote.FileID
		rt.CapturedFileIDs["video_note"] = fileID
	}

	return &StepResult{
		Method:     "sendVideoNote",
		MessageIDs: []int{msg.MessageID},
		FileIDs:    []string{fileID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"file_id":    fileID,
		},
	}, nil
}

// SendMediaGroupStep sends a media group (album).
type SendMediaGroupStep struct {
	Media []MediaInput
}

func (s *SendMediaGroupStep) Name() string { return "sendMediaGroup" }

func (s *SendMediaGroupStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	messages, err := rt.Sender.SendMediaGroup(ctx, rt.ChatID, s.Media)
	if err != nil {
		return nil, err
	}

	var msgIDs []int
	for _, msg := range messages {
		rt.TrackMessage(rt.ChatID, msg.MessageID)
		msgIDs = append(msgIDs, msg.MessageID)
	}

	if len(messages) > 0 {
		rt.LastMessage = messages[len(messages)-1]
	}

	return &StepResult{
		Method:     "sendMediaGroup",
		MessageIDs: msgIDs,
		Evidence: map[string]any{
			"message_count": len(messages),
			"message_ids":   msgIDs,
		},
	}, nil
}

// GetFileStep gets file info for a captured file.
type GetFileStep struct {
	FileKey string // Key in CapturedFileIDs (e.g., "photo", "document")
}

func (s *GetFileStep) Name() string { return "getFile" }

func (s *GetFileStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	fileID, ok := rt.CapturedFileIDs[s.FileKey]
	if !ok || fileID == "" {
		return nil, fmt.Errorf("no file_id captured for key %q", s.FileKey)
	}

	file, err := rt.Sender.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method:  "getFile",
		FileIDs: []string{fileID},
		Evidence: map[string]any{
			"file_id":        file.FileID,
			"file_unique_id": file.FileUniqueID,
			"file_size":      file.FileSize,
			"file_path":      file.FilePath,
		},
	}, nil
}

// EditMessageCaptionStep edits the caption of the last message.
type EditMessageCaptionStep struct {
	Caption string
}

func (s *EditMessageCaptionStep) Name() string { return "editMessageCaption" }

func (s *EditMessageCaptionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to edit")
	}

	msg, err := rt.Sender.EditMessageCaption(ctx, rt.ChatID, rt.LastMessage.MessageID, s.Caption)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg

	return &StepResult{
		Method: "editMessageCaption",
		Evidence: map[string]any{
			"message_id":  msg.MessageID,
			"new_caption": s.Caption,
		},
	}, nil
}

// EditMessageMediaStep edits the media content of the last message.
type EditMessageMediaStep struct {
	Media sender.InputMedia
}

func (s *EditMessageMediaStep) Name() string { return "editMessageMedia" }

func (s *EditMessageMediaStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to edit")
	}

	msg, err := rt.Sender.EditMessageMedia(ctx, rt.ChatID, rt.LastMessage.MessageID, s.Media)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg

	return &StepResult{
		Method: "editMessageMedia",
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"media_type": s.Media.Type,
		},
	}, nil
}

// ================= Interactive Steps (require user interaction) =================

// WaitForCallbackStep waits for a callback query on rt.CallbackChan.
type WaitForCallbackStep struct {
	Timeout time.Duration // Max wait time; defaults to 60s
}

func (s *WaitForCallbackStep) Name() string { return "waitForCallback" }

func (s *WaitForCallbackStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.CallbackChan == nil {
		return nil, fmt.Errorf("CallbackChan not set — interactive scenarios require polling mode")
	}

	timeout := s.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	select {
	case cb := <-rt.CallbackChan:
		rt.CapturedFileIDs["callback_query_id"] = cb.ID
		return &StepResult{
			Method: "waitForCallback",
			Evidence: map[string]any{
				"callback_query_id": cb.ID,
				"callback_data":     cb.Data,
				"from_user_id":      cb.From.ID,
			},
		}, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for callback query (%s) — please click a button in the chat", timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// AnswerCallbackQueryStep answers the last received callback query.
type AnswerCallbackQueryStep struct {
	Text      string
	ShowAlert bool
}

func (s *AnswerCallbackQueryStep) Name() string { return "answerCallbackQuery" }

func (s *AnswerCallbackQueryStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	cbID, ok := rt.CapturedFileIDs["callback_query_id"]
	if !ok || cbID == "" {
		return nil, fmt.Errorf("no callback_query_id captured — run WaitForCallbackStep first")
	}

	err := rt.Sender.AnswerCallbackQuery(ctx, cbID, s.Text, s.ShowAlert)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "answerCallbackQuery",
		Evidence: map[string]any{
			"callback_query_id": cbID,
			"text":              s.Text,
			"show_alert":        s.ShowAlert,
		},
	}, nil
}

// ================= Webhook & Polling Steps =================

// GetWebhookInfoStep retrieves webhook info and optionally stores the URL for restore.
type GetWebhookInfoStep struct {
	StoreAs string // If set, stores the webhook URL in CapturedFileIDs[StoreAs]
}

func (s *GetWebhookInfoStep) Name() string { return "getWebhookInfo" }

func (s *GetWebhookInfoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	info, err := rt.Sender.GetWebhookInfo(ctx)
	if err != nil {
		return nil, err
	}

	if s.StoreAs != "" {
		rt.CapturedFileIDs[s.StoreAs] = info.URL
	}

	return &StepResult{
		Method: "getWebhookInfo",
		Evidence: map[string]any{
			"url":                  info.URL,
			"pending_update_count": info.PendingUpdateCount,
			"has_custom_cert":      info.HasCustomCert,
		},
	}, nil
}

// SetWebhookStep sets a webhook URL.
type SetWebhookStep struct {
	URL string
}

func (s *SetWebhookStep) Name() string { return "setWebhook" }

func (s *SetWebhookStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.SetWebhook(ctx, s.URL)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setWebhook",
		Evidence: map[string]any{
			"url": s.URL,
		},
	}, nil
}

// DeleteWebhookStep removes the webhook.
type DeleteWebhookStep struct{}

func (s *DeleteWebhookStep) Name() string { return "deleteWebhook" }

func (s *DeleteWebhookStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.DeleteWebhook(ctx)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "deleteWebhook",
	}, nil
}

// VerifyWebhookURLStep checks that the current webhook URL matches the expected value.
type VerifyWebhookURLStep struct {
	ExpectedURL string
}

func (s *VerifyWebhookURLStep) Name() string { return "getWebhookInfo (verify)" }

func (s *VerifyWebhookURLStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	info, err := rt.Sender.GetWebhookInfo(ctx)
	if err != nil {
		return nil, err
	}

	if info.URL != s.ExpectedURL {
		return nil, fmt.Errorf("expected webhook URL %q, got %q", s.ExpectedURL, info.URL)
	}

	return &StepResult{
		Method: "getWebhookInfo",
		Evidence: map[string]any{
			"url":      info.URL,
			"expected": s.ExpectedURL,
			"match":    true,
		},
	}, nil
}

// RestoreWebhookStep restores a previously saved webhook URL.
// If the stored URL is empty, it deletes the webhook instead.
type RestoreWebhookStep struct {
	StoredKey string // Key in CapturedFileIDs to read the original URL from
}

func (s *RestoreWebhookStep) Name() string { return "restoreWebhook" }

func (s *RestoreWebhookStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	originalURL := rt.CapturedFileIDs[s.StoredKey]

	if originalURL == "" {
		// No webhook was set before — just delete
		err := rt.Sender.DeleteWebhook(ctx)
		if err != nil {
			return nil, err
		}
		return &StepResult{
			Method: "deleteWebhook",
			Evidence: map[string]any{
				"action": "deleted (no previous webhook)",
			},
		}, nil
	}

	// Restore the original webhook
	err := rt.Sender.SetWebhook(ctx, originalURL)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setWebhook",
		Evidence: map[string]any{
			"action":       "restored",
			"restored_url": originalURL,
		},
	}, nil
}

// GetUpdatesStep calls getUpdates with timeout=0 (non-blocking).
type GetUpdatesStep struct{}

func (s *GetUpdatesStep) Name() string { return "getUpdates" }

func (s *GetUpdatesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	updates, err := rt.Sender.GetUpdates(ctx, -1, 1, 0)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getUpdates",
		Evidence: map[string]any{
			"update_count": len(updates),
		},
	}, nil
}
