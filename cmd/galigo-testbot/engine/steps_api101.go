package engine

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 10.1 steps (rich messages) ====================
// STUB (B-25 10.1d red record): the steps exist and do nothing.

// SendRichMessageStep sends a rich message. STUB.
type SendRichMessageStep struct {
	RichMessage tg.InputRichMessage
	WantBlocks  []string
}

func (s *SendRichMessageStep) Name() string { return "sendRichMessage" }

func (s *SendRichMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	return &StepResult{}, nil
}

// EditMessageRichStep edits the last message to rich content. STUB.
type EditMessageRichStep struct {
	RichMessage tg.InputRichMessage
	WantBlocks  []string
}

func (s *EditMessageRichStep) Name() string { return "editMessageText (rich_message)" }

func (s *EditMessageRichStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	return &StepResult{}, nil
}

// SendRichMessageDraftStep streams a rich draft. STUB.
type SendRichMessageDraftStep struct {
	DraftID     int
	RichMessage tg.InputRichMessage
}

func (s *SendRichMessageDraftStep) Name() string { return "sendRichMessageDraft" }

func (s *SendRichMessageDraftStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	return &StepResult{}, nil
}
