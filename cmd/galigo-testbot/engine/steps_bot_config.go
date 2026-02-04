package engine

import (
	"context"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// ================= Bot Commands Steps =================

// SetMyCommandsStep sets the bot's command list.
type SetMyCommandsStep struct {
	Commands []tg.BotCommand
}

func (s *SetMyCommandsStep) Name() string { return "setMyCommands" }

func (s *SetMyCommandsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := rt.Sender.SetMyCommands(ctx, s.Commands); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setMyCommands",
		Evidence: map[string]any{
			"command_count": len(s.Commands),
		},
	}, nil
}

// GetMyCommandsStep gets the bot's command list.
type GetMyCommandsStep struct {
	ExpectedCount int // If > 0, assert this many commands
}

func (s *GetMyCommandsStep) Name() string { return "getMyCommands" }

func (s *GetMyCommandsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	commands, err := rt.Sender.GetMyCommands(ctx)
	if err != nil {
		return nil, err
	}

	if s.ExpectedCount > 0 && len(commands) != s.ExpectedCount {
		return nil, fmt.Errorf("expected %d commands, got %d", s.ExpectedCount, len(commands))
	}

	return &StepResult{
		Method: "getMyCommands",
		Evidence: map[string]any{
			"command_count": len(commands),
		},
	}, nil
}

// DeleteMyCommandsStep removes the bot's command list.
type DeleteMyCommandsStep struct{}

func (s *DeleteMyCommandsStep) Name() string { return "deleteMyCommands" }

func (s *DeleteMyCommandsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := rt.Sender.DeleteMyCommands(ctx); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "deleteMyCommands",
	}, nil
}

// ================= Bot Profile Steps =================

// SetMyNameStep sets the bot's name.
type SetMyNameStep struct {
	BotName string
}

func (s *SetMyNameStep) Name() string { return "setMyName" }

func (s *SetMyNameStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := rt.Sender.SetMyName(ctx, s.BotName); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setMyName",
		Evidence: map[string]any{
			"name": s.BotName,
		},
	}, nil
}

// GetMyNameStep gets the bot's name.
type GetMyNameStep struct {
	ExpectedName string // If non-empty, assert this name
}

func (s *GetMyNameStep) Name() string { return "getMyName" }

func (s *GetMyNameStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	result, err := rt.Sender.GetMyName(ctx)
	if err != nil {
		return nil, err
	}

	if s.ExpectedName != "" && result.Name != s.ExpectedName {
		return nil, fmt.Errorf("expected name %q, got %q", s.ExpectedName, result.Name)
	}

	return &StepResult{
		Method: "getMyName",
		Evidence: map[string]any{
			"name": result.Name,
		},
	}, nil
}

// SetMyDescriptionStep sets the bot's description.
type SetMyDescriptionStep struct {
	Description string
}

func (s *SetMyDescriptionStep) Name() string { return "setMyDescription" }

func (s *SetMyDescriptionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := rt.Sender.SetMyDescription(ctx, s.Description); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setMyDescription",
		Evidence: map[string]any{
			"description": s.Description,
		},
	}, nil
}

// GetMyDescriptionStep gets the bot's description.
type GetMyDescriptionStep struct{}

func (s *GetMyDescriptionStep) Name() string { return "getMyDescription" }

func (s *GetMyDescriptionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	result, err := rt.Sender.GetMyDescription(ctx)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getMyDescription",
		Evidence: map[string]any{
			"description": result.Description,
		},
	}, nil
}

// SetMyShortDescriptionStep sets the bot's short description.
type SetMyShortDescriptionStep struct {
	ShortDescription string
}

func (s *SetMyShortDescriptionStep) Name() string { return "setMyShortDescription" }

func (s *SetMyShortDescriptionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := rt.Sender.SetMyShortDescription(ctx, s.ShortDescription); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setMyShortDescription",
		Evidence: map[string]any{
			"short_description": s.ShortDescription,
		},
	}, nil
}

// GetMyShortDescriptionStep gets the bot's short description.
type GetMyShortDescriptionStep struct{}

func (s *GetMyShortDescriptionStep) Name() string { return "getMyShortDescription" }

func (s *GetMyShortDescriptionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	result, err := rt.Sender.GetMyShortDescription(ctx)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getMyShortDescription",
		Evidence: map[string]any{
			"short_description": result.ShortDescription,
		},
	}, nil
}

// ================= Bot Admin Rights Steps =================

// SetMyDefaultAdministratorRightsStep sets the bot's default admin rights.
type SetMyDefaultAdministratorRightsStep struct {
	Rights      *tg.ChatAdministratorRights
	ForChannels bool
}

func (s *SetMyDefaultAdministratorRightsStep) Name() string { return "setMyDefaultAdministratorRights" }

func (s *SetMyDefaultAdministratorRightsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := rt.Sender.SetMyDefaultAdministratorRights(ctx, s.Rights, s.ForChannels); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setMyDefaultAdministratorRights",
		Evidence: map[string]any{
			"for_channels": s.ForChannels,
		},
	}, nil
}

// GetMyDefaultAdministratorRightsStep gets the bot's default admin rights.
type GetMyDefaultAdministratorRightsStep struct {
	ForChannels bool
}

func (s *GetMyDefaultAdministratorRightsStep) Name() string { return "getMyDefaultAdministratorRights" }

func (s *GetMyDefaultAdministratorRightsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	result, err := rt.Sender.GetMyDefaultAdministratorRights(ctx, s.ForChannels)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getMyDefaultAdministratorRights",
		Evidence: map[string]any{
			"for_channels":        s.ForChannels,
			"can_delete_messages": result.CanDeleteMessages,
			"can_invite_users":    result.CanInviteUsers,
		},
	}, nil
}
