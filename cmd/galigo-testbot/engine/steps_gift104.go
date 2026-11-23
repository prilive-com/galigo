package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Fixed inputs and the measured refusal of S79's delete row (Bot API 10.4; probes 2026-11-23): getBusinessConnection
// and deleteBusinessAccountGift both answer 400 "Bad Request: business connection not found" for a connection id the
// bot does not have.
const (
	giftFakeConnection     = "zzNoSuchConnection"
	giftFakeOwnedGift      = "zzNoSuchGift"
	giftConnectionNotFound = "Bad Request: business connection not found"
)

// GetUniqueGiftStep gets a unique gift by its name (Bot API 10.4). With an empty WantError the gift must have the
// asked name, a gift_id, a base name and a number >= 1. With WantError the call must fail with a 400 whose
// description contains WantError. The step sends no message and runs in any chat. A sender error is kept with %w, so
// the runner repeats a 429.
type GetUniqueGiftStep struct {
	GiftName  string
	WantError string
}

func (s *GetUniqueGiftStep) Name() string { return "getUniqueGift" }

func (s *GetUniqueGiftStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	gift, err := rt.Sender.GetUniqueGift(ctx, s.GiftName)
	if s.WantError != "" {
		if err == nil {
			return nil, fmt.Errorf("getUniqueGift: the server took the call; want a 400 error containing %q", s.WantError)
		}
		var apiErr *tg.APIError
		if !errors.As(err, &apiErr) {
			return nil, fmt.Errorf("getUniqueGift: want an API refusal (400 containing %q), got: %w", s.WantError, err)
		}
		if apiErr.Code != 400 {
			return nil, fmt.Errorf("getUniqueGift: want a 400 error, got code %d: %w", apiErr.Code, err)
		}
		if !strings.Contains(apiErr.Description, s.WantError) {
			return nil, fmt.Errorf("getUniqueGift: the error does not contain %q: %w", s.WantError, err)
		}
		return &StepResult{StepName: s.Name(), Method: "getUniqueGift",
			Evidence: map[string]any{"error_code": apiErr.Code, "description": apiErr.Description}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getUniqueGift: %w", err)
	}
	switch {
	case gift == nil:
		return nil, errors.New("getUniqueGift: no result")
	case gift.Name != s.GiftName:
		return nil, fmt.Errorf("getUniqueGift: name %q, want %q", gift.Name, s.GiftName)
	case gift.GiftID == "":
		return nil, errors.New("getUniqueGift: empty gift_id")
	case gift.BaseName == "":
		return nil, errors.New("getUniqueGift: empty base_name")
	case gift.Number < 1:
		return nil, fmt.Errorf("getUniqueGift: number %d, want >= 1", gift.Number)
	}
	return &StepResult{StepName: s.Name(), Method: "getUniqueGift",
		Evidence: map[string]any{"name": gift.Name, "gift_id": gift.GiftID, "number": gift.Number}}, nil
}

// DeleteBusinessAccountGiftStep sends deleteBusinessAccountGift (Bot API 10.4) with fixed fake ids and must get the
// measured refusal (refusal only: the bot has no business connection; a successful delete is proved by mock tests).
// Guard first: getBusinessConnection for the fake connection id must be refused with exactly 400 "Bad Request:
// business connection not found", else the delete is not sent; a found connection gives a fixed error that names no id.
// The delete must then be refused with exactly that 400. Both errors are kept with %w, so the runner repeats a 429
// (the whole step again, guard first). The guard call is not a covered method.
type DeleteBusinessAccountGiftStep struct{}

func (s *DeleteBusinessAccountGiftStep) Name() string { return "deleteBusinessAccountGift" }

// giftRefused returns the API error when err is exactly the measured 400 refusal.
func giftRefused(err error) (*tg.APIError, bool) {
	var apiErr *tg.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 400 || apiErr.Description != giftConnectionNotFound {
		return nil, false
	}
	return apiErr, true
}

func (s *DeleteBusinessAccountGiftStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	_, err := rt.Sender.GetBusinessConnection(ctx, giftFakeConnection)
	if err == nil {
		return nil, errors.New("deleteBusinessAccountGift: guard: getBusinessConnection did not refuse the test connection; the delete is not sent")
	}
	guard, ok := giftRefused(err)
	if !ok {
		return nil, fmt.Errorf("deleteBusinessAccountGift: guard: want getBusinessConnection refused with 400 %q; the delete is not sent: %w", giftConnectionNotFound, err)
	}
	err = rt.Sender.DeleteBusinessAccountGift(ctx, sender.DeleteBusinessAccountGiftRequest{BusinessConnectionID: giftFakeConnection, OwnedGiftID: giftFakeOwnedGift})
	if err == nil {
		return nil, fmt.Errorf("deleteBusinessAccountGift: the server took the call; want a 400 %q", giftConnectionNotFound)
	}
	del, ok := giftRefused(err)
	if !ok {
		return nil, fmt.Errorf("deleteBusinessAccountGift: want the refusal 400 %q, got: %w", giftConnectionNotFound, err)
	}
	return &StepResult{StepName: s.Name(), Method: "deleteBusinessAccountGift", Evidence: map[string]any{
		"guard_error_code": guard.Code, "guard_description": guard.Description,
		"error_code": del.Code, "description": del.Description}}, nil
}
