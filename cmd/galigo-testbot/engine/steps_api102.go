package engine

import (
	"context"
	"errors"
)

// SendRichMediaBlocksStep is a stub for the red record (TEST-PLAN §7).
type SendRichMediaBlocksStep struct {
	WantBlocks []string
}

func (s *SendRichMediaBlocksStep) Name() string { return "sendRichMessage (media blocks)" }

func (s *SendRichMediaBlocksStep) Execute(context.Context, *Runtime) (*StepResult, error) {
	return &StepResult{StepName: s.Name()}, errors.New("stub")
}
