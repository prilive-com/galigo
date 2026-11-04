package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/prilive-com/galigo/sender"
)

// ==================== Bot API 10.0 Steps ====================
//
// Each step makes exactly one Telegram call (the runner repeats a whole step after a 429), except that a check of
// chat rights probes the chat first when rt.ChatCtx is nil (ProbeChat: up to three reads, no writes). Every step
// returns a result with StepName and Method on every path, so a report lists the method of a failed or skipped
// step too.

// Check errors of the 10.0 steps: one per check, so a test can tell which check failed.
var (
	errReact100NoMessage    = errors.New("reactions: no last message")
	errReact100NoFrom       = errors.New("reactions: the last message has no sender")
	errReact100NoFromID     = errors.New("reactions: the last message's sender id is 0")
	errReact100Kind         = errors.New("reactions: unknown kind")
	errAdmins100NoBot       = errors.New("getChatAdministrators: no administrator is a bot")
	errPerm100NoPermissions = errors.New("getChat: the chat has no permissions")
	errPerm100NoReact       = errors.New("getChat: permissions have no can_react_to_messages")
)

// RequireDeleteMessages100Step skips the scenario unless the bot is an administrator with can_delete_messages.
// It sends nothing; its Method "probeChat" is a marker, not a Telegram method.
type RequireDeleteMessages100Step struct{}

func (s *RequireDeleteMessages100Step) Name() string { return "requireCanDeleteMessages (10.0)" }

func (s *RequireDeleteMessages100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "probeChat"}
	if err := RequireCanDeleteMessages(ctx, rt); err != nil {
		return res, err
	}
	return res, nil
}

// DeleteReactions100Step removes the reactions of the last message's sender: Kind "single" calls
// deleteMessageReaction on the last message, Kind "all" calls deleteAllMessageReactions in the chat. In the test
// chat the last message is the bot's own, so it removes the bot's reactions ("all": in the whole chat). It has no
// Skip path: a missing source is an error.
type DeleteReactions100Step struct {
	Kind string // "single" or "all"
}

func (s *DeleteReactions100Step) Name() string { return "deleteReactions " + s.Kind + " (10.0)" }

func (s *DeleteReactions100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name()}
	switch s.Kind {
	case "single":
		res.Method = "deleteMessageReaction"
	case "all":
		res.Method = "deleteAllMessageReactions"
	default:
		return res, fmt.Errorf("%w: %q", errReact100Kind, s.Kind)
	}

	switch {
	case rt.LastMessage == nil:
		return res, errReact100NoMessage
	case rt.LastMessage.From == nil:
		return res, errReact100NoFrom
	case rt.LastMessage.From.ID == 0:
		return res, errReact100NoFromID
	}
	userID := rt.LastMessage.From.ID

	var err error
	if s.Kind == "single" {
		err = rt.Sender.DeleteMessageReaction(ctx, sender.DeleteMessageReactionRequest{
			ChatID: rt.ChatID, MessageID: rt.LastMessage.MessageID, UserID: userID})
		res.Evidence = map[string]any{"message_id": rt.LastMessage.MessageID, "user_id": userID}
	} else {
		err = rt.Sender.DeleteAllMessageReactions(ctx, sender.DeleteAllMessageReactionsRequest{
			ChatID: rt.ChatID, UserID: userID})
		res.Evidence = map[string]any{"user_id": userID}
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

// GetAdminsWithBots100Step calls getChatAdministrators with return_bots and checks that at least one
// administrator is a bot (the current bot is always listed, so this is a smoke check of the call and its
// decode). It skips when the bot is not an administrator.
type GetAdminsWithBots100Step struct{}

func (s *GetAdminsWithBots100Step) Name() string { return "getChatAdministrators return_bots (10.0)" }

func (s *GetAdminsWithBots100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "getChatAdministrators"}
	if err := RequireAdmin(ctx, rt); err != nil {
		return res, err
	}
	admins, err := rt.Sender.GetChatAdministratorsWithBots(ctx, rt.ChatID)
	if err != nil {
		return res, err
	}
	res.Evidence = map[string]any{"administrators": len(admins)}
	for _, m := range admins {
		if u := m.GetUser(); u != nil && u.IsBot {
			return res, nil
		}
	}
	return res, errAdmins100NoBot
}

// ChatPermissionsReact100Step calls getChat and checks that the chat's permissions carry can_react_to_messages
// (true or false; a set false is the field present).
type ChatPermissionsReact100Step struct{}

func (s *ChatPermissionsReact100Step) Name() string { return "getChat can_react_to_messages (10.0)" }

func (s *ChatPermissionsReact100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "getChat"}
	chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
	if err != nil {
		return res, err
	}
	if chat.Permissions == nil {
		return res, errPerm100NoPermissions
	}
	if chat.Permissions.CanReactToMessages == nil {
		return res, errPerm100NoReact
	}
	res.Evidence = map[string]any{"can_react_to_messages": *chat.Permissions.CanReactToMessages}
	return res, nil
}

// EmptyDraft100Step sends a message draft with an empty text (allowed since 10.0) to the admin's private chat.
// It skips without an admin user. It does not use rt.ChatID: a draft needs a private chat.
type EmptyDraft100Step struct{}

func (s *EmptyDraft100Step) Name() string { return "sendMessageDraft empty text (10.0)" }

func (s *EmptyDraft100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendMessageDraft"}
	if rt.AdminUserID == 0 {
		return res, Skip("no admin user (TESTBOT_ADMINS) for a private draft")
	}
	if err := rt.Sender.SendMessageDraft(ctx, rt.AdminUserID, 100, ""); err != nil {
		return res, err
	}
	res.Evidence = map[string]any{"chat_id": rt.AdminUserID, "draft_id": 100}
	return res, nil
}
