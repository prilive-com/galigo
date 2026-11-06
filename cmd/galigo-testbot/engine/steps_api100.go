package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
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

// ==================== Bot API 10.0 part b: live photos ====================
//
// Each live photo step makes one Telegram call. A send step tracks every returned message and fills MessageIDs
// before any check of the reply, and on a failed check returns that result with the error, so a failed run still
// lists every sent id for cleanup (PLAN §8 P3-01). File ids are captured under these keys.
const (
	liveVideoKey = "live_video" // LivePhoto.FileID of the last upload
	liveStillKey = "live_still" // the last LivePhoto.Photo size's FileID of the last upload
)

// liveCaptured returns the captured video and still ids, or an error naming a missing or empty key.
func liveCaptured(rt *Runtime) (string, string, error) {
	video, still := rt.CapturedFileIDs[liveVideoKey], rt.CapturedFileIDs[liveStillKey]
	if video == "" {
		return "", "", fmt.Errorf("no file_id captured for key %q", liveVideoKey)
	}
	if still == "" {
		return "", "", fmt.Errorf("no file_id captured for key %q", liveStillKey)
	}
	return video, still, nil
}

// liveIDs returns the video id and the last static size's id of a live photo message.
func liveIDs(msg *tg.Message) (string, string) {
	sizes := msg.LivePhoto.Photo
	return msg.LivePhoto.FileID, sizes[len(sizes)-1].FileID
}

// SendLivePhoto100Step sends a live photo: the fixture video and photo, or the ids captured by an earlier upload.
// An upload captures the reply's ids. The reply must be a live photo and, when Caption is set, carry it.
type SendLivePhoto100Step struct {
	ByFileID bool
	Caption  string
}

func (s *SendLivePhoto100Step) Name() string { return "sendLivePhoto (10.0)" }

func (s *SendLivePhoto100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendLivePhoto"}
	var video, photo MediaInput
	var fileIDs []string
	if s.ByFileID {
		v, p, err := liveCaptured(rt)
		if err != nil {
			return res, fmt.Errorf("sendLivePhoto: %w", err)
		}
		video, photo, fileIDs = MediaFromFileID(v), MediaFromFileID(p), []string{v, p}
	} else {
		video = MediaFromBytes(fixtures.VideoBytes(), "galigo-live.mp4", "")
		photo = MediaFromBytes(fixtures.PhotoBytes(), "galigo-live.jpg", "")
	}

	msg, err := rt.Sender.SendLivePhoto(ctx, rt.ChatID, video, photo, s.Caption)
	if err != nil {
		return res, err
	}
	if msg == nil {
		return res, errors.New("sendLivePhoto: no message in the reply")
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res.MessageIDs = []int{msg.MessageID}

	if !hasMediaKind(msg, "live_photo") {
		return res, errors.New("sendLivePhoto: the reply is not a live photo")
	}
	if s.Caption != "" && msg.Caption != s.Caption {
		return res, fmt.Errorf("sendLivePhoto: caption %q, want %q", msg.Caption, s.Caption)
	}
	if !s.ByFileID {
		v, p := liveIDs(msg)
		rt.CapturedFileIDs[liveVideoKey] = v
		rt.CapturedFileIDs[liveStillKey] = p
		fileIDs = []string{v, p}
	}

	res.FileIDs = fileIDs
	res.Evidence = map[string]any{
		"message_id": msg.MessageID,
		"by_file_id": s.ByFileID,
		"caption":    s.Caption,
		"file_ids":   fileIDs,
	}
	return res, nil
}

// LivePhotoAlbum100Step sends an album of two live photos (uploads, or the captured ids) and expects exactly two
// live photo messages back.
type LivePhotoAlbum100Step struct {
	ByFileID bool
}

func (s *LivePhotoAlbum100Step) Name() string { return "sendMediaGroup live photos (10.0)" }

func (s *LivePhotoAlbum100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendMediaGroup"}
	var media []MediaInput
	var sentIDs []string
	if s.ByFileID {
		v, p, err := liveCaptured(rt)
		if err != nil {
			return res, fmt.Errorf("sendMediaGroup: %w", err)
		}
		for range 2 {
			media = append(media, MediaInput{FileID: v, Type: "live_photo", Photo: &MediaInput{FileID: p}})
		}
		sentIDs = []string{v, p}
	} else {
		for i := 1; i <= 2; i++ {
			item := MediaFromBytes(fixtures.VideoBytes(), fmt.Sprintf("galigo-live-%d.mp4", i), "live_photo")
			still := MediaFromBytes(fixtures.PhotoBytes(), fmt.Sprintf("galigo-live-%d.jpg", i), "")
			item.Photo = &still
			media = append(media, item)
		}
	}

	messages, err := rt.Sender.SendMediaGroup(ctx, rt.ChatID, media)
	if err != nil {
		return res, err
	}
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		rt.TrackMessage(rt.ChatID, msg.MessageID)
		res.MessageIDs = append(res.MessageIDs, msg.MessageID)
	}
	if len(messages) > 0 {
		rt.LastMessage = messages[len(messages)-1]
	}

	if len(messages) != len(media) {
		return res, fmt.Errorf("sendMediaGroup: album has %d messages, want %d", len(messages), len(media))
	}
	fileIDs := sentIDs
	for i, msg := range messages {
		if !hasMediaKind(msg, "live_photo") {
			return res, fmt.Errorf("sendMediaGroup: album message %d is not a live photo", i+1)
		}
		if !s.ByFileID {
			v, p := liveIDs(msg)
			fileIDs = append(fileIDs, v, p)
		}
	}

	res.FileIDs = fileIDs
	res.Evidence = map[string]any{
		"message_count": len(messages),
		"message_ids":   res.MessageIDs,
		"by_file_id":    s.ByFileID,
		"file_ids":      fileIDs,
	}
	return res, nil
}

// EditToLivePhoto100Step edits the last message (a plain photo) into a live photo with the captured ids. The
// reply must keep the message id and be a live photo.
type EditToLivePhoto100Step struct{}

func (s *EditToLivePhoto100Step) Name() string { return "editMessageMedia live photo (10.0)" }

func (s *EditToLivePhoto100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "editMessageMedia"}
	if rt.LastMessage == nil {
		return res, errors.New("editMessageMedia: no message to edit")
	}
	v, p, err := liveCaptured(rt)
	if err != nil {
		return res, fmt.Errorf("editMessageMedia: %w", err)
	}
	msgID := rt.LastMessage.MessageID

	media := sender.NewInputMediaLivePhoto(sender.FromFileID(v), sender.FromFileID(p))
	msg, err := rt.Sender.EditMessageMedia(ctx, rt.ChatID, msgID, media)
	if err != nil {
		return res, err
	}
	if msg == nil {
		return res, errors.New("editMessageMedia: no message in the reply")
	}
	rt.LastMessage = msg

	if msg.MessageID != msgID {
		return res, fmt.Errorf("editMessageMedia: reply message id %d, want %d", msg.MessageID, msgID)
	}
	if !hasMediaKind(msg, "live_photo") {
		return res, errors.New("editMessageMedia: the edited message is not a live photo")
	}

	res.FileIDs = []string{v, p}
	res.Evidence = map[string]any{"message_id": msgID, "file_ids": res.FileIDs}
	return res, nil
}

// pollRefusal100 is the error text of a voter restriction (members_only or country_codes) outside a channel, as
// the server sent it on 2026-11-06 (E10/lookups/probe100c/10, 11).
const pollRefusal100 = "poll voters can be restricted only in channel chats"

// SendPoll100Step sends Req to rt.ChatID with sendPoll and checks that the reply echoes the option count,
// members_only and country_codes (nil and empty lists are equal; the order of the codes does not matter, because
// the API does not promise it). In a group or supergroup the server refuses a voter restriction with a 400
// "poll voters can be restricted only in channel chats"; when Req asks for one, that refusal passes with evidence
// {"refused":"channel only"}: a smoke check that a restriction reached the server, not which one. Any other error
// fails.
type SendPoll100Step struct {
	Req sender.SendPollRequest
}

func (s *SendPoll100Step) Name() string { return "sendPoll (10.0)" }

func (s *SendPoll100Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendPoll"}
	req := s.Req
	req.ChatID = rt.ChatID
	msg, err := rt.Sender.SendPoll(ctx, req)
	if err != nil {
		var apiErr *tg.APIError
		if (req.MembersOnly || len(req.CountryCodes) > 0) && errors.As(err, &apiErr) && apiErr.Code == 400 &&
			strings.Contains(apiErr.Description, pollRefusal100) {
			res.Evidence = map[string]any{"refused": "channel only"}
			return res, nil
		}
		return res, err
	}
	if msg == nil {
		return res, errors.New("sendPoll: no message in the reply")
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res.MessageIDs = []int{msg.MessageID}
	p := msg.Poll
	if p == nil {
		return res, errors.New("sendPoll: the reply has no poll")
	}
	res.Evidence = map[string]any{"message_id": msg.MessageID, "options_reply": len(p.Options),
		"members_only": p.MembersOnly, "country_codes": p.CountryCodes}
	switch {
	case len(p.Options) != len(req.Options):
		return res, fmt.Errorf("sendPoll: %d options in the reply, want %d", len(p.Options), len(req.Options))
	case p.MembersOnly != req.MembersOnly:
		return res, fmt.Errorf("sendPoll: members_only %v in the reply, want %v", p.MembersOnly, req.MembersOnly)
	case !slices.Equal(slices.Sorted(slices.Values(p.CountryCodes)), slices.Sorted(slices.Values(req.CountryCodes))):
		return res, fmt.Errorf("sendPoll: country_codes %v in the reply, want %v", p.CountryCodes, req.CountryCodes)
	}
	return res, nil
}

// pollMediaKind returns the kind of a poll media (Bot API 10.0): "" for nil, else the JSON name of its one set
// field. More than one set field is an error.
func pollMediaKind(m *tg.PollMedia) (string, error) {
	if m == nil {
		return "", nil
	}
	var kinds []string
	for _, k := range []struct {
		name string
		set  bool
	}{
		{"animation", m.Animation != nil},
		{"audio", m.Audio != nil},
		{"document", m.Document != nil},
		{"live_photo", m.LivePhoto != nil},
		{"location", m.Location != nil},
		{"photo", len(m.Photo) > 0},
		{"sticker", m.Sticker != nil},
		{"venue", m.Venue != nil},
		{"video", m.Video != nil},
	} {
		if k.set {
			kinds = append(kinds, k.name)
		}
	}
	if len(kinds) > 1 {
		return "", fmt.Errorf("poll media with %d kinds: %s", len(kinds), strings.Join(kinds, ", "))
	}
	if len(kinds) == 0 {
		return "", nil
	}
	return kinds[0], nil
}

// SendPollMediaStep sends a poll with media (Bot API 10.0: description, explanation and option media) and checks
// the media kinds of the poll in the reply. An empty wanted kind means no media there.
type SendPollMediaStep struct {
	Req             sender.SendPollRequest
	WantMedia       string
	WantExplanation string
	WantOptions     []string
}

func (s *SendPollMediaStep) Name() string { return "sendPoll (media)" }

func (s *SendPollMediaStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendPoll"}
	req := s.Req
	req.ChatID = rt.ChatID
	msg, err := rt.Sender.SendPoll(ctx, req)
	if err != nil {
		return res, err
	}
	if msg == nil {
		return res, errors.New("sendPoll: no message in the reply")
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res.MessageIDs = []int{msg.MessageID}
	p := msg.Poll
	if p == nil {
		return res, errors.New("sendPoll: the reply has no poll")
	}
	media, err := pollMediaKind(p.Media)
	if err != nil {
		return res, fmt.Errorf("sendPoll: media: %w", err)
	}
	expl, err := pollMediaKind(p.ExplanationMedia)
	if err != nil {
		return res, fmt.Errorf("sendPoll: explanation_media: %w", err)
	}
	if len(p.Options) != len(s.WantOptions) {
		return res, fmt.Errorf("sendPoll: %d options in the reply, want %d", len(p.Options), len(s.WantOptions))
	}
	opts := make([]string, len(p.Options))
	for i, o := range p.Options {
		if opts[i], err = pollMediaKind(o.Media); err != nil {
			return res, fmt.Errorf("sendPoll: option %d: %w", i, err)
		}
	}
	res.Evidence = map[string]any{"message_id": msg.MessageID, "media": media, "explanation_media": expl,
		"option_media": opts}
	if media != s.WantMedia {
		return res, fmt.Errorf("sendPoll: media %q in the reply, want %q", media, s.WantMedia)
	}
	if expl != s.WantExplanation {
		return res, fmt.Errorf("sendPoll: explanation_media %q in the reply, want %q", expl, s.WantExplanation)
	}
	for i, k := range opts {
		if k != s.WantOptions[i] {
			return res, fmt.Errorf("sendPoll: option %d media %q in the reply, want %q", i, k, s.WantOptions[i])
		}
	}
	return res, nil
}
