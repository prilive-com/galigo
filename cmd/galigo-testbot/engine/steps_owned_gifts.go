package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// giftsPageLimit is the page size S80's read rows ask for.
const giftsPageLimit = 1

// GetBusinessAccountGiftsStep sends getBusinessAccountGifts with the fake connection id and must get the measured
// refusal, exactly 400 "Bad Request: business connection not found" (refusal only: the bot has no business
// connection; a successful call is proved by mock tests). Any other answer fails the step. The sender error is kept
// with %w, so the runner repeats a 429.
type GetBusinessAccountGiftsStep struct{}

func (s *GetBusinessAccountGiftsStep) Name() string { return "getBusinessAccountGifts" }

func (s *GetBusinessAccountGiftsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	_, err := rt.Sender.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: giftFakeConnection})
	if err == nil {
		return nil, fmt.Errorf("getBusinessAccountGifts: the server took the call; want a 400 %q", giftConnectionNotFound)
	}
	apiErr, ok := giftRefused(err)
	if !ok {
		return nil, fmt.Errorf("getBusinessAccountGifts: want the refusal 400 %q, got: %w", giftConnectionNotFound, err)
	}
	return &StepResult{Method: "getBusinessAccountGifts",
		Evidence: map[string]any{"error_code": apiErr.Code, "description": apiErr.Description}}, nil
}

// GetUserGiftsStep reads one page (limit 1) of the first admin's gifts; skipped with no admin. The page must be a
// list of at most one gift, each with a type. Its error text never holds the user id: the id is replaced by
// "[user]", and the error still unwraps to the sender error (errors.As, the runner's 429 repeat).
type GetUserGiftsStep struct{}

func (s *GetUserGiftsStep) Name() string { return "getUserGifts" }

func (s *GetUserGiftsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.AdminUserID == 0 {
		return nil, Skip("no AdminUserID for getUserGifts")
	}
	page, err := rt.Sender.GetUserGifts(ctx, sender.GetUserGiftsRequest{UserID: rt.AdminUserID, Limit: giftsPageLimit})
	if err != nil {
		return nil, &userRedacted{err: fmt.Errorf("getUserGifts: %w", err), id: strconv.FormatInt(rt.AdminUserID, 10)}
	}
	return giftsPage("getUserGifts", page)
}

// GetChatGiftsStep reads one page (limit 1) of the test chat's gifts. It runs only in a group or a supergroup: the
// type is read from rt.ChatCtx when it is set, else from one getChat call, used only here (rt.ChatCtx is not written).
type GetChatGiftsStep struct{}

func (s *GetChatGiftsStep) Name() string { return "getChatGifts" }

func (s *GetChatGiftsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	var chatType string
	if rt.ChatCtx != nil {
		chatType = rt.ChatCtx.ChatType
	} else {
		chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
		if err != nil {
			return nil, fmt.Errorf("getChatGifts: getChat: %w", err)
		}
		chatType = chat.Type
	}
	if chatType != "group" && chatType != "supergroup" {
		return nil, Skip(fmt.Sprintf("getChatGifts needs a group or supergroup (chat type %q)", chatType))
	}
	page, err := rt.Sender.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: rt.ChatID, Limit: giftsPageLimit})
	if err != nil {
		return nil, fmt.Errorf("getChatGifts: %w", err)
	}
	return giftsPage("getChatGifts", page)
}

// giftsPage checks one read page and returns its evidence (counts only).
func giftsPage(method string, page *tg.OwnedGifts) (*StepResult, error) {
	switch {
	case page == nil || page.Gifts == nil:
		return nil, fmt.Errorf("%s: no gift list in the result", method)
	case len(page.Gifts) > giftsPageLimit:
		return nil, fmt.Errorf("%s: %d gifts returned, want at most %d", method, len(page.Gifts), giftsPageLimit)
	case page.TotalCount < len(page.Gifts):
		return nil, fmt.Errorf("%s: total_count %d is below the %d gifts returned", method, page.TotalCount, len(page.Gifts))
	}
	for i, g := range page.Gifts {
		if g.Type == "" {
			return nil, fmt.Errorf("%s: gift %d has no type", method, i)
		}
	}
	return &StepResult{Method: method, Evidence: map[string]any{
		"total_count": page.TotalCount, "gifts_returned": len(page.Gifts), "next_offset_nonempty": page.NextOffset != ""}}, nil
}

// userRedacted is an error whose text has the user id replaced by "[user]"; it unwraps to the original error.
type userRedacted struct {
	err error
	id  string
}

func (e *userRedacted) Error() string { return strings.ReplaceAll(e.err.Error(), e.id, "[user]") }
func (e *userRedacted) Unwrap() error { return e.err }
