package engine

import (
	"context"
	"fmt"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ================= Chat Photo Steps =================

// SaveChatPhotoStep saves the current photo's file_id for restore.
type SaveChatPhotoStep struct{}

func (s *SaveChatPhotoStep) Name() string { return "saveChatPhoto" }

func (s *SaveChatPhotoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := RequireCanChangeInfo(ctx, rt); err != nil {
		return nil, err
	}

	chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	rt.OriginalChatPhoto = &ChatPhotoSnapshot{HadPhoto: false}

	if chat.Photo != nil && chat.Photo.BigFileID != "" {
		rt.OriginalChatPhoto = &ChatPhotoSnapshot{
			HadPhoto: true,
			FileID:   chat.Photo.BigFileID,
		}
	}

	return &StepResult{
		Method: "getChat",
		Evidence: map[string]any{
			"had_photo": rt.OriginalChatPhoto.HadPhoto,
		},
	}, nil
}

// SetChatPhotoStep sets the chat photo using the sticker fixture (512x512 PNG).
// Chat photos require at least 160x160 pixels.
type SetChatPhotoStep struct {
	PhotoBytes []byte // If nil, uses ChatPhotoPNG
}

func (s *SetChatPhotoStep) Name() string { return "setChatPhoto" }

func (s *SetChatPhotoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := RequireCanChangeInfo(ctx, rt); err != nil {
		return nil, err
	}

	// Use ChatPhotoPNG (160x160) by default, or custom bytes if provided
	photoBytes := s.PhotoBytes
	if photoBytes == nil {
		photoBytes = ChatPhotoPNG
	}

	photo := sender.FromBytes(photoBytes, "chatphoto.png")

	err := rt.Sender.SetChatPhoto(ctx, rt.ChatID, photo)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setChatPhoto",
		Evidence: map[string]any{
			"photo_size": len(photoBytes),
		},
	}, nil
}

// RestoreChatPhotoStep restores the original photo using FromFileID.
type RestoreChatPhotoStep struct{}

func (s *RestoreChatPhotoStep) Name() string { return "restoreChatPhoto" }

func (s *RestoreChatPhotoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.OriginalChatPhoto == nil {
		return nil, fmt.Errorf("no photo snapshot — run SaveChatPhotoStep first")
	}

	if !rt.OriginalChatPhoto.HadPhoto {
		err := rt.Sender.DeleteChatPhoto(ctx, rt.ChatID)
		if err != nil {
			return nil, err
		}
		return &StepResult{
			Method: "deleteChatPhoto",
			Evidence: map[string]any{
				"action": "deleted (no original)",
			},
		}, nil
	}

	// Restore using FromFileID — no download needed
	err := rt.Sender.SetChatPhoto(ctx, rt.ChatID,
		sender.FromFileID(rt.OriginalChatPhoto.FileID))
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setChatPhoto",
		Evidence: map[string]any{
			"action": "restored original",
		},
	}, nil
}

// ================= Chat Permissions Steps =================

// SaveChatPermissionsStep saves the current permissions for restore.
type SaveChatPermissionsStep struct{}

func (s *SaveChatPermissionsStep) Name() string { return "saveChatPermissions" }

func (s *SaveChatPermissionsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := RequireCanRestrict(ctx, rt); err != nil {
		return nil, err
	}

	chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	// SKIP if nil — cannot safely restore (Consultant 9's approach)
	if chat.Permissions == nil {
		return nil, Skip("chat.permissions is nil; cannot safely restore")
	}

	// Deep copy permissions
	permsCopy := *chat.Permissions
	rt.OriginalPermissions = &PermissionsSnapshot{
		Permissions: &permsCopy,
	}

	return &StepResult{
		Method: "getChat",
		Evidence: map[string]any{
			"saved_permissions": true,
		},
	}, nil
}

// SetChatPermissionsStep temporarily restricts to text-only using existing helpers.
type SetChatPermissionsStep struct{}

func (s *SetChatPermissionsStep) Name() string { return "setChatPermissions" }

func (s *SetChatPermissionsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if err := RequireCanRestrict(ctx, rt); err != nil {
		return nil, err
	}

	// Use existing tg.TextOnlyPermissions() — no *bool bugs
	perms := tg.TextOnlyPermissions()

	err := rt.Sender.SetChatPermissions(ctx, rt.ChatID, perms)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setChatPermissions",
		Evidence: map[string]any{
			"action": "restricted to text-only",
		},
	}, nil
}

// RestoreChatPermissionsStep restores original permissions.
type RestoreChatPermissionsStep struct{}

func (s *RestoreChatPermissionsStep) Name() string { return "restoreChatPermissions" }

func (s *RestoreChatPermissionsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.OriginalPermissions == nil || rt.OriginalPermissions.Permissions == nil {
		// Fallback to AllPermissions() — safe default
		err := rt.Sender.SetChatPermissions(ctx, rt.ChatID, tg.AllPermissions())
		if err != nil {
			return nil, err
		}
		return &StepResult{
			Method: "setChatPermissions",
			Evidence: map[string]any{
				"action": "restored defaults (AllPermissions)",
			},
		}, nil
	}

	err := rt.Sender.SetChatPermissions(ctx, rt.ChatID, *rt.OriginalPermissions.Permissions)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setChatPermissions",
		Evidence: map[string]any{
			"action": "restored original",
		},
	}, nil
}
