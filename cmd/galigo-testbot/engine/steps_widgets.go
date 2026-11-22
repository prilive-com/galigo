package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Server texts for the widget methods (Bot API 10.4) for this bot, measured 2026-11-21 (B-65 65f probes 1-3):
// the server refuses sendWidget in the test group and in a private chat, so these refusals are the live proof.
const (
	WidgetNotAllowed      = "INPUT_MEDIA_WIDGET_NOT_ALLOWED"
	WidgetIDInvalid       = "invalid widget identifier specified"
	WidgetQueryInvalid    = "query is too old"
	WidgetMessageInvalid  = "MESSAGE_ID_INVALID"
	WidgetMessageNotFound = "message with widget to edit not found"
	WidgetInlineInvalid   = "invalid inline message identifier specified"
)

// widgetHTML is the html of the sendWidget row: with a non-empty html the server refuses widgets for this bot with
// WidgetNotAllowed (an empty html gets a different refusal, `parameter "html" is required`).
const widgetHTML = "<b>galigo widget</b>"

// WidgetRefusalStep calls one widget method (Bot API 10.4) that the server must refuse with 400 and WantError: the
// refusal shows that the method reached the server and took the named refusal branch (the request bodies are proved
// by mock tests). Method is sendWidget, sendSharedWidget, answerWidgetQuery, editMessageWidgetData or
// editSharedWidgetData; Variant is "target" (the last sent message, with chat_data), "missing" (that id + 1000000)
// or "inline" (a fake inline message id) for editMessageWidgetData, and empty otherwise. The step runs only in a
// group or supergroup, where the texts were measured: it probes the chat once when rt.ChatCtx is nil (a probe error
// fails the step before any write) and skips in any other chat, so a /run from a private chat makes no widget call.
// A call the server takes fails the step; a returned message is tracked for cleanup. A missing row whose target id is
// too large for target + 1000000 in an int fails before any call, also before the chat probe.
type WidgetRefusalStep struct {
	Method    string
	Variant   string
	WantError string
}

func (s *WidgetRefusalStep) Name() string {
	switch s.Variant {
	case "target":
		return s.Method + " (10.4), target message"
	case "missing":
		return s.Method + " (10.4), missing message"
	case "inline":
		return s.Method + " (10.4), inline message"
	}
	return s.Method + " (10.4)"
}

func (s *WidgetRefusalStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: s.Method}
	// The missing row's id (target + 1000000) must fit in an int; checked first, so a too large target makes no call.
	if s.Variant == "missing" && rt.LastMessage != nil && rt.LastMessage.MessageID > math.MaxInt-1000000 {
		return res, fmt.Errorf("%s: target message id %d is too large for the missing message id (target + 1000000)", s.Method, rt.LastMessage.MessageID)
	}
	if rt.ChatCtx == nil {
		if err := rt.ProbeChat(ctx); err != nil {
			return res, fmt.Errorf("%s: %w", s.Method, err)
		}
	}
	if t := rt.ChatCtx.ChatType; t != "group" && t != "supergroup" {
		return res, Skip(fmt.Sprintf("%s runs only in a group or supergroup (chat type %q)", s.Method, t))
	}
	id := 0
	if s.Variant == "target" || s.Variant == "missing" {
		if rt.LastMessage == nil {
			return res, fmt.Errorf("%s: no target message (run a send step first)", s.Method)
		}
		id = rt.LastMessage.MessageID
		if s.Variant == "missing" {
			id += 1000000
		}
	}
	data := func(v string) *string { return &v }
	var msg *tg.Message
	var err error
	switch s.Method + "/" + s.Variant {
	case "sendWidget/":
		msg, err = rt.Sender.SendWidget(ctx, sender.SendWidgetRequest{ChatID: rt.ChatID, HTML: widgetHTML})
	case "sendSharedWidget/":
		msg, err = rt.Sender.SendSharedWidget(ctx, sender.SendSharedWidgetRequest{ChatID: rt.ChatID, WidgetID: "galigo-fake-widget"})
	case "answerWidgetQuery/":
		err = rt.Sender.AnswerWidgetQuery(ctx, sender.AnswerWidgetQueryRequest{WidgetQueryID: "galigo-fake-query", Data: "x"})
	case "editMessageWidgetData/target", "editMessageWidgetData/missing":
		err = rt.Sender.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{ChatID: rt.ChatID, MessageID: id, ChatData: data("c")})
	case "editMessageWidgetData/inline":
		err = rt.Sender.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{InlineMessageID: "galigo-fake-inline", PublicData: data("p")})
	case "editSharedWidgetData/":
		err = rt.Sender.EditSharedWidgetData(ctx, sender.EditSharedWidgetDataRequest{WidgetID: "galigo-fake-widget", PublicData: ""})
	default:
		return res, fmt.Errorf("unknown widget row %q", s.Name())
	}
	if err == nil {
		if msg != nil {
			rt.TrackMessage(rt.ChatID, msg.MessageID)
			res.MessageIDs = []int{msg.MessageID}
		}
		return res, fmt.Errorf("%s: the server took the call; want a 400 error containing %q", s.Method, s.WantError)
	}
	var apiErr *tg.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 400 || !strings.Contains(apiErr.Description, s.WantError) {
		return res, fmt.Errorf("%s: want a 400 error containing %q, got: %w", s.Method, s.WantError, err)
	}
	ev := map[string]any{"error_code": apiErr.Code, "description": apiErr.Description}
	if id != 0 {
		ev["message_id"] = id
	}
	res.Evidence = ev
	return res, nil
}
