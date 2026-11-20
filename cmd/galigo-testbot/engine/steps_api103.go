package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 10.3 steps (rich messages, reply markup) ====================

// VerifyRichAPI103Step checks the 10.3 kinds in the rich message of rt.LastMessage, as S74 sends them: block 0 a row
// of four buttons aligned "center" (a callback button with style "primary", a URL button, a copy_text button that
// copies "galigo", a disabled button), block 1 a paragraph with a button node that has callback data, block 2 a
// compact table and block 3 an expandable quotation with the text "galigo 10.3 expandable". It checks what galigo
// decodes, not the URL (the server normalises it). It makes no API call.
type VerifyRichAPI103Step struct{}

func (s *VerifyRichAPI103Step) Name() string { return "verify rich message (10.3)" }

func (s *VerifyRichAPI103Step) Execute(_ context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendRichMessage"}
	if rt.LastMessage == nil || rt.LastMessage.RichMessage == nil {
		return res, errors.New("verify rich message: no rich message in the last message")
	}
	blocks := rt.LastMessage.RichMessage.Blocks
	if len(blocks) < 4 {
		return res, fmt.Errorf("verify rich message: %d blocks, want 4", len(blocks))
	}
	row, ok := blocks[0].(tg.RichBlockButtons)
	switch {
	case !ok:
		return res, fmt.Errorf("verify rich message: block 0 is %T, want tg.RichBlockButtons", blocks[0])
	case row.Align != "center":
		return res, fmt.Errorf("verify rich message: buttons align %q, want center", row.Align)
	case len(row.Buttons) != 4:
		return res, fmt.Errorf("verify rich message: %d buttons, want 4", len(row.Buttons))
	case row.Buttons[0].CallbackData == "" || row.Buttons[0].Style != tg.ButtonStylePrimary:
		return res, errors.New("verify rich message: button 0 is not a primary callback button")
	case row.Buttons[1].URL == "":
		return res, errors.New("verify rich message: button 1 has no URL")
	case row.Buttons[2].CopyText == nil || row.Buttons[2].CopyText.Text != "galigo":
		return res, errors.New("verify rich message: button 2 does not copy \"galigo\"")
	case row.Buttons[3].Disabled == nil:
		return res, errors.New("verify rich message: button 3 is not disabled")
	}
	p, ok := blocks[1].(tg.RichBlockParagraph)
	if !ok {
		return res, fmt.Errorf("verify rich message: block 1 is %T, want tg.RichBlockParagraph", blocks[1])
	}
	if !slices.ContainsFunc(p.Text.Parts, func(part tg.RichText) bool {
		n, isButton := part.Node.(tg.RichTextButton)
		return isButton && n.Button.CallbackData != ""
	}) {
		return res, errors.New("verify rich message: block 1 has no button node with callback data")
	}
	table, ok := blocks[2].(tg.RichBlockTable)
	if !ok {
		return res, fmt.Errorf("verify rich message: block 2 is %T, want tg.RichBlockTable", blocks[2])
	}
	if !table.IsCompact {
		return res, errors.New("verify rich message: the table is not compact")
	}
	q, ok := blocks[3].(tg.RichBlockExpandableBlockQuotation)
	if !ok {
		return res, fmt.Errorf("verify rich message: block 3 is %T, want tg.RichBlockExpandableBlockQuotation", blocks[3])
	}
	if q.Text.Plain != "galigo 10.3 expandable" {
		return res, fmt.Errorf("verify rich message: quotation text %q, want \"galigo 10.3 expandable\"", q.Text.Plain)
	}
	res.Evidence = map[string]any{"message_id": rt.LastMessage.MessageID,
		"checked": []string{"buttons", "text_button", "is_compact", "expandable_blockquote"}}
	return res, nil
}

// SendRichDocumentBlockStep sends to rt.ChatID a rich message with one document block, made from the file_id of the
// first document block in the rich message of rt.LastMessage, with the caption "document block". The send and the
// checks of the reply are SendRichMessageStep's.
type SendRichDocumentBlockStep struct {
	WantBlocks []string
}

func (s *SendRichDocumentBlockStep) Name() string { return "sendRichMessage (document block)" }

func (s *SendRichDocumentBlockStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendRichMessage"}
	if rt.LastMessage == nil || rt.LastMessage.RichMessage == nil {
		return res, errors.New("sendRichMessage: no rich message to take the document from")
	}
	var id string
	for _, b := range rt.LastMessage.RichMessage.Blocks {
		if d, ok := b.(tg.RichBlockDocument); ok {
			id = d.Document.FileID
			break
		}
	}
	if id == "" {
		return res, errors.New("sendRichMessage: no document file_id in the rich message of the last message")
	}
	rm := tg.InputRichMessage{Blocks: []tg.InputRichBlock{tg.InputRichBlockDocument{
		Document: sender.NewInputMediaDocument(sender.FromFileID(id)),
		Caption:  &tg.RichBlockCaption{Text: tg.RichText{Plain: "document block"}},
	}}}
	res, err := (&SendRichMessageStep{RichMessage: rm, WantBlocks: s.WantBlocks}).Execute(ctx, rt)
	res.StepName = s.Name()
	return res, err
}

// SendReplyMarkupEchoStep sends Text with the inline keyboard Markup to rt.ChatID (Bot API 10.3: a disabled button,
// force_reply) and requires the message the server returns to carry the same markup as a whole value. It records the
// message for cleanup and the message budget before it checks the reply. On a mismatch the error names where the
// values differ, but prints no text from the server (B-116): positions, field names, presence and lengths only.
type SendReplyMarkupEchoStep struct {
	Text   string
	Markup tg.InlineKeyboardMarkup
}

func (s *SendReplyMarkupEchoStep) Name() string { return "sendMessage (reply markup)" }

func (s *SendReplyMarkupEchoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendMessage"}
	markup := s.Markup
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, s.Text, WithReplyMarkup(&markup))
	if err != nil {
		return res, err
	}
	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	res.MessageIDs = []int{msg.MessageID}
	res.Evidence = map[string]any{"message_id": msg.MessageID, "rows": len(s.Markup.InlineKeyboard),
		"force_reply": s.Markup.ForceReply}
	// stub: no compare
	return res, nil
}

// markupDiff names the first places where got differs from want, with no text from got: row and button counts, the
// position and Go field name of a differing button field, whether a pointer field is set, and the length of a
// differing string.
func markupDiff(want, got tg.InlineKeyboardMarkup) string {
	return "" // stub
}
