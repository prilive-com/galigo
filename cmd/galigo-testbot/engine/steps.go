package engine

import (
	"context"
	"fmt"

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
			"original_message_id": rt.LastMessage.MessageID,
			"forwarded_message_id": msg.MessageID,
			"to_chat_id":          toChatID,
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
	msg, err := rt.Sender.SendPhoto(ctx, rt.ChatID, s.Photo, s.Caption)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Capture file_id for reuse
	var fileID string
	if msg.Photo != nil && len(msg.Photo) > 0 {
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
