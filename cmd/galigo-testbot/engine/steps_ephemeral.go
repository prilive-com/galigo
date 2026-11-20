package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// SendEphemeralStep sends an ephemeral message (Bot API 10.3) to rt.AdminUserID in rt.ChatID. Method is
// "sendMessage" (JSON body) or "sendPhoto" (an upload: form body). The bot must be an administrator of rt.ChatID
// (else the step is skipped), and the admin a member. The server must answer with message_id 0, an
// ephemeral_message_id and receiver_user = the admin. The step records the ephemeral message as soon as the server
// returns it, so CleanupStep (or the stop cleanup) deletes it with deleteEphemeralMessage, and a retried delete never
// sends again. A message with a message_id was posted for everyone (galigo dropped the parameter): it is tracked for
// cleanup and the step fails.
type SendEphemeralStep struct {
	Method string
}

func (s *SendEphemeralStep) Name() string { return s.Method + " ephemeral_message_parameters (10.3)" }

func (s *SendEphemeralStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: s.Method}
	if rt.AdminUserID <= 0 { // a receiver must be a positive user id
		return res, fmt.Errorf("%s: an ephemeral message needs a receiver (a positive user id); set AdminUserID on Runtime", s.Method)
	}
	if err := RequireAdmin(ctx, rt); err != nil {
		return res, err
	}
	opt := WithEphemeralMessageParameters(&tg.EphemeralMessageParameters{ReceiverUserID: rt.AdminUserID})
	var msg *tg.Message
	var err error
	switch s.Method {
	case "sendMessage":
		msg, err = rt.Sender.SendMessage(ctx, rt.ChatID, "galigo ephemeral message", opt)
	case "sendPhoto":
		photo := MediaFromBytes(fixtures.PhotoBytes(), "galigo-ephemeral.jpg", "photo")
		msg, err = rt.Sender.SendPhoto(ctx, rt.ChatID, photo, opt)
	default:
		return res, fmt.Errorf("unknown method %q", s.Method)
	}
	if err != nil {
		return res, fmt.Errorf("%s: %w", s.Method, err)
	}
	if msg == nil {
		return res, fmt.Errorf("%s: no message returned", s.Method)
	}
	// Clean up in the chat the reply names (the request named rt.ChatID, the fallback).
	chatID := rt.ChatID
	if msg.Chat != nil && msg.Chat.ID != 0 {
		chatID = msg.Chat.ID
	}
	if msg.MessageID != 0 {
		rt.TrackMessage(chatID, msg.MessageID)
		res.MessageIDs = []int{msg.MessageID}
		return res, fmt.Errorf("%s: the server posted message %d for everyone; galigo did not send "+
			"ephemeral_message_parameters", s.Method, msg.MessageID)
	}
	if msg.EphemeralMessageID == 0 {
		return res, fmt.Errorf("%s: no ephemeral_message_id in the reply", s.Method)
	}
	// Delete it for the receiver the server names (the admin, unless the server is wrong); a receiver that is not a
	// positive user id cannot be deleted for, so the admin is used.
	receiver := rt.AdminUserID
	if msg.ReceiverUser != nil && msg.ReceiverUser.ID > 0 {
		receiver = msg.ReceiverUser.ID
	}
	rt.recordEphemeral(chatID, receiver, msg.EphemeralMessageID)
	res.EphemeralMessageIDs = []int{msg.EphemeralMessageID}
	receiverOK := msg.ReceiverUser != nil && msg.ReceiverUser.ID == rt.AdminUserID
	res.Evidence = map[string]any{"message_id": msg.MessageID, "ephemeral_message_id": msg.EphemeralMessageID,
		"receiver_is_admin": receiverOK}
	if !receiverOK {
		return res, fmt.Errorf("%s: receiver_user is not the admin", s.Method)
	}
	return res, nil
}

// EphemeralReplyStep sends an ephemeral reply to a callback query (Bot API 10.4) with no chat_id and a callback
// query id the server does not know. The server must refuse it with 400 "query is too old and response timeout
// expired or query ID is invalid" (tg.ErrCallbackExpired): Telegram took the parameter and reached its callback
// check (with no ephemeral_message_parameters it answers "chat_id is empty"). The server gives the same answer when
// chat_id is set, so only the mock tests check that galigo leaves chat_id out. If the server takes the reply, the
// step fails and reports what the server made; the cleanup deletes it only in the chat (and for the receiver) the
// reply names, as the request had no chat_id; without them the error says it cannot be deleted.
type EphemeralReplyStep struct {
	CallbackQueryID string
}

func (s *EphemeralReplyStep) Name() string {
	return "sendMessage ephemeral reply without chat_id (10.4)"
}

func (s *EphemeralReplyStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendMessage"}
	msg, err := rt.Sender.SendEphemeralReply(ctx, s.CallbackQueryID, "galigo ephemeral reply")
	if err == nil {
		err = errors.New("sendMessage: the server took an ephemeral reply to an unknown callback query")
		if msg == nil {
			return res, err
		}
		// The request had no chat_id: clean up only in the chat (and for the receiver) the reply names.
		var chatID, receiver int64
		if msg.Chat != nil {
			chatID = msg.Chat.ID
		}
		if msg.ReceiverUser != nil {
			receiver = msg.ReceiverUser.ID
		}
		switch {
		case msg.MessageID != 0:
			res.MessageIDs = []int{msg.MessageID}
			if chatID == 0 {
				return res, fmt.Errorf("%w; message %d cannot be deleted: the reply names no chat", err, msg.MessageID)
			}
			rt.TrackMessage(chatID, msg.MessageID)
		case msg.EphemeralMessageID != 0:
			res.EphemeralMessageIDs = []int{msg.EphemeralMessageID}
			if chatID == 0 || receiver <= 0 { // a receiver must be a positive user id
				return res, fmt.Errorf("%w; ephemeral message %d cannot be deleted: the reply names no chat or receiver",
					err, msg.EphemeralMessageID)
			}
			rt.recordEphemeral(chatID, receiver, msg.EphemeralMessageID)
		}
		return res, err
	}
	var apiErr *tg.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 400 || !errors.Is(err, tg.ErrCallbackExpired) {
		return res, fmt.Errorf("sendMessage: want a 400 error for an unknown callback query (tg.ErrCallbackExpired), "+
			"got: %w", err)
	}
	res.Evidence = map[string]any{"error_code": apiErr.Code, "description": apiErr.Description}
	return res, nil
}

// EditEphemeralStep edits the newest ephemeral message the scenario recorded (Bot API 10.3 changes). Method is
// "editEphemeralMessageText" (a rich message with one uploaded photo: the form path), "editEphemeralMessageReplyMarkup"
// (one callback button), "editEphemeralMessageCaption" (show_caption_above_media) or "editEphemeralMessageMedia" (a
// photo upload). The server answers True, so the step checks only that the edit was accepted. An edit sends no new
// message: it adds nothing to the message budget, and the record stays for the cleanup.
type EditEphemeralStep struct {
	Method string
}

func (s *EditEphemeralStep) Name() string { return s.Method + " (10.3)" }

func (s *EditEphemeralStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: s.Method}
	if len(rt.ephemeralMessages) == 0 {
		return res, fmt.Errorf("%s: no ephemeral message to edit", s.Method)
	}
	m := rt.ephemeralMessages[len(rt.ephemeralMessages)-1]
	photo := func() sender.InputMedia {
		return sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "galigo-ephemeral-edit.jpg"))
	}
	var err error
	switch s.Method {
	case "editEphemeralMessageText":
		err = rt.Sender.EditEphemeralMessageText(ctx, sender.EditEphemeralMessageTextRequest{
			ChatID: m.chatID, ReceiverUserID: m.receiverUserID, EphemeralMessageID: m.messageID,
			RichMessage: &tg.InputRichMessage{Markdown: "galigo ephemeral **rich** edit\n\n![](tg://photo?id=e)",
				Media: []tg.InputRichMessageMedia{{ID: "e", Media: photo()}}}})
	case "editEphemeralMessageReplyMarkup":
		err = rt.Sender.EditEphemeralMessageReplyMarkup(ctx, sender.EditEphemeralMessageReplyMarkupRequest{
			ChatID: m.chatID, ReceiverUserID: m.receiverUserID, EphemeralMessageID: m.messageID,
			ReplyMarkup: &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "galigo", CallbackData: "b65b"}}}}})
	case "editEphemeralMessageCaption":
		err = rt.Sender.EditEphemeralMessageCaption(ctx, sender.EditEphemeralMessageCaptionRequest{
			ChatID: m.chatID, ReceiverUserID: m.receiverUserID, EphemeralMessageID: m.messageID,
			Caption: "galigo ephemeral caption", ShowCaptionAboveMedia: true})
	case "editEphemeralMessageMedia":
		err = rt.Sender.EditEphemeralMessageMedia(ctx, sender.EditEphemeralMessageMediaRequest{
			ChatID: m.chatID, ReceiverUserID: m.receiverUserID, EphemeralMessageID: m.messageID, Media: photo()})
	default:
		return res, fmt.Errorf("unknown method %q", s.Method)
	}
	if err != nil {
		return res, fmt.Errorf("%s: %w", s.Method, err)
	}
	res.Evidence = map[string]any{"ephemeral_message_id": m.messageID}
	return res, nil
}
