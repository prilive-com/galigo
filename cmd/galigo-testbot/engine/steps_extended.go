package engine

import (
	"context"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// ================= Sticker Steps =================

// CreateStickerSetStep creates a new sticker set and tracks it for cleanup.
type CreateStickerSetStep struct {
	NameSuffix string // Appended to bot username to form full set name
	Title      string
	Stickers   []StickerInput
}

func (s *CreateStickerSetStep) Name() string { return "createNewStickerSet" }

func (s *CreateStickerSetStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.AdminUserID == 0 {
		return nil, fmt.Errorf("createNewStickerSet requires a human user_id; set AdminUserID on Runtime")
	}

	me, err := rt.Sender.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("getMe: %w", err)
	}

	setName := s.NameSuffix + "_by_" + me.Username

	// createNewStickerSet requires a real human user_id, not the bot's own ID.
	err = rt.Sender.CreateNewStickerSet(ctx, rt.AdminUserID, setName, s.Title, s.Stickers)
	if err != nil {
		return nil, err
	}

	rt.TrackStickerSet(setName)
	rt.CapturedFileIDs["sticker_set_name"] = setName

	return &StepResult{
		Method: "createNewStickerSet",
		Evidence: map[string]any{
			"set_name": setName,
			"title":    s.Title,
			"stickers": len(s.Stickers),
		},
	}, nil
}

// GetStickerSetStep gets a sticker set (uses the name from CapturedFileIDs).
type GetStickerSetStep struct{}

func (s *GetStickerSetStep) Name() string { return "getStickerSet" }

func (s *GetStickerSetStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	setName := rt.CapturedFileIDs["sticker_set_name"]
	if setName == "" {
		return nil, fmt.Errorf("no sticker set name captured")
	}

	set, err := rt.Sender.GetStickerSet(ctx, setName)
	if err != nil {
		return nil, err
	}

	// Capture first sticker's file_id for later steps
	if len(set.Stickers) > 0 {
		rt.CapturedFileIDs["sticker_file_id"] = set.Stickers[0].FileID
	}

	return &StepResult{
		Method: "getStickerSet",
		Evidence: map[string]any{
			"name":          set.Name,
			"title":         set.Title,
			"sticker_count": len(set.Stickers),
		},
	}, nil
}

// AddStickerToSetStep adds a sticker to the tracked set.
type AddStickerToSetStep struct {
	Sticker StickerInput
}

func (s *AddStickerToSetStep) Name() string { return "addStickerToSet" }

func (s *AddStickerToSetStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	setName := rt.CapturedFileIDs["sticker_set_name"]
	if setName == "" {
		return nil, fmt.Errorf("no sticker set name captured")
	}

	me, err := rt.Sender.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("getMe: %w", err)
	}

	if err := rt.Sender.AddStickerToSet(ctx, me.ID, setName, s.Sticker); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "addStickerToSet",
		Evidence: map[string]any{
			"set_name": setName,
		},
	}, nil
}

// SetStickerPositionStep moves a sticker in the set.
type SetStickerPositionStep struct {
	Position int
}

func (s *SetStickerPositionStep) Name() string { return "setStickerPositionInSet" }

func (s *SetStickerPositionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	fileID := rt.CapturedFileIDs["sticker_file_id"]
	if fileID == "" {
		return nil, fmt.Errorf("no sticker file_id captured")
	}

	if err := rt.Sender.SetStickerPositionInSet(ctx, fileID, s.Position); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setStickerPositionInSet",
		Evidence: map[string]any{
			"sticker":  fileID,
			"position": s.Position,
		},
	}, nil
}

// SetStickerEmojiListStep changes emoji for the captured sticker.
type SetStickerEmojiListStep struct {
	EmojiList []string
}

func (s *SetStickerEmojiListStep) Name() string { return "setStickerEmojiList" }

func (s *SetStickerEmojiListStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	fileID := rt.CapturedFileIDs["sticker_file_id"]
	if fileID == "" {
		return nil, fmt.Errorf("no sticker file_id captured")
	}

	if err := rt.Sender.SetStickerEmojiList(ctx, fileID, s.EmojiList); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setStickerEmojiList",
		Evidence: map[string]any{
			"sticker":    fileID,
			"emoji_list": s.EmojiList,
		},
	}, nil
}

// SetStickerSetTitleStep changes the title of the tracked sticker set.
type SetStickerSetTitleStep struct {
	Title string
}

func (s *SetStickerSetTitleStep) Name() string { return "setStickerSetTitle" }

func (s *SetStickerSetTitleStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	setName := rt.CapturedFileIDs["sticker_set_name"]
	if setName == "" {
		return nil, fmt.Errorf("no sticker set name captured")
	}

	if err := rt.Sender.SetStickerSetTitle(ctx, setName, s.Title); err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setStickerSetTitle",
		Evidence: map[string]any{
			"set_name": setName,
			"title":    s.Title,
		},
	}, nil
}

// DeleteStickerFromSetStep deletes the captured sticker from the set.
type DeleteStickerFromSetStep struct{}

func (s *DeleteStickerFromSetStep) Name() string { return "deleteStickerFromSet" }

func (s *DeleteStickerFromSetStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	fileID := rt.CapturedFileIDs["sticker_file_id"]
	if fileID == "" {
		return nil, fmt.Errorf("no sticker file_id captured")
	}

	if err := rt.Sender.DeleteStickerFromSet(ctx, fileID); err != nil {
		return nil, err
	}

	delete(rt.CapturedFileIDs, "sticker_file_id")

	return &StepResult{
		Method: "deleteStickerFromSet",
		Evidence: map[string]any{
			"sticker": fileID,
		},
	}, nil
}

// DeleteStickerSetStep deletes the tracked sticker set.
type DeleteStickerSetStep struct{}

func (s *DeleteStickerSetStep) Name() string { return "deleteStickerSet" }

func (s *DeleteStickerSetStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	setName := rt.CapturedFileIDs["sticker_set_name"]
	if setName == "" {
		return nil, fmt.Errorf("no sticker set name captured")
	}

	if err := rt.Sender.DeleteStickerSet(ctx, setName); err != nil {
		return nil, err
	}

	// Remove from cleanup list since we deleted it manually
	for i, name := range rt.CreatedStickerSets {
		if name == setName {
			rt.CreatedStickerSets = append(rt.CreatedStickerSets[:i], rt.CreatedStickerSets[i+1:]...)
			break
		}
	}
	delete(rt.CapturedFileIDs, "sticker_set_name")

	return &StepResult{
		Method: "deleteStickerSet",
		Evidence: map[string]any{
			"set_name": setName,
		},
	}, nil
}

// ================= Stars & Payments Steps =================

// GetMyStarBalanceStep gets the bot's Star balance.
type GetMyStarBalanceStep struct{}

func (s *GetMyStarBalanceStep) Name() string { return "getMyStarBalance" }

func (s *GetMyStarBalanceStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	balance, err := rt.Sender.GetMyStarBalance(ctx)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getMyStarBalance",
		Evidence: map[string]any{
			"amount":          balance.Amount,
			"nanostar_amount": balance.NanostarAmount,
		},
	}, nil
}

// GetStarTransactionsStep gets recent Star transactions.
type GetStarTransactionsStep struct {
	Limit int
}

func (s *GetStarTransactionsStep) Name() string { return "getStarTransactions" }

func (s *GetStarTransactionsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	limit := s.Limit
	if limit == 0 {
		limit = 10
	}

	result, err := rt.Sender.GetStarTransactions(ctx, limit)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getStarTransactions",
		Evidence: map[string]any{
			"transaction_count": len(result.Transactions),
		},
	}, nil
}

// SendInvoiceStep sends an invoice message.
type SendInvoiceStep struct {
	Title       string
	Description string
	Payload     string
	Currency    string
	Prices      []tg.LabeledPrice
}

func (s *SendInvoiceStep) Name() string { return "sendInvoice" }

func (s *SendInvoiceStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendInvoice(ctx, rt.ChatID, s.Title, s.Description, s.Payload, s.Currency, s.Prices)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendInvoice",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"title":      s.Title,
		},
	}, nil
}

// ================= Gifts Steps =================

// GetAvailableGiftsStep gets available gifts.
type GetAvailableGiftsStep struct{}

func (s *GetAvailableGiftsStep) Name() string { return "getAvailableGifts" }

func (s *GetAvailableGiftsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	gifts, err := rt.Sender.GetAvailableGifts(ctx)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getAvailableGifts",
		Evidence: map[string]any{
			"gift_count": len(gifts.Gifts),
		},
	}, nil
}

// ================= Checklist Steps =================

// SendChecklistStep sends a checklist message.
type SendChecklistStep struct {
	Title string
	Tasks []string
}

func (s *SendChecklistStep) Name() string { return "sendChecklist" }

func (s *SendChecklistStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendChecklist(ctx, rt.ChatID, s.Title, s.Tasks)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendChecklist",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"title":      s.Title,
			"task_count": len(s.Tasks),
		},
	}, nil
}

// EditChecklistStep edits the last checklist message.
type EditChecklistStep struct {
	Title string
	Tasks []ChecklistTaskInput
}

func (s *EditChecklistStep) Name() string { return "editChecklist" }

func (s *EditChecklistStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no last message to edit")
	}

	msg, err := rt.Sender.EditChecklist(ctx, rt.ChatID, rt.LastMessage.MessageID, s.Title, s.Tasks)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg

	return &StepResult{
		Method:     "editChecklist",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"title":      s.Title,
		},
	}, nil
}
