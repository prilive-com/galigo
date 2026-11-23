package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// GetChatShareThemeBackdropsStep calls getChatShareThemeBackdrops (Bot API 10.4, read only). It fails on a
// duplicate id or a color outside 0..0xFFFFFF. An empty list and an empty name are valid replies.
type GetChatShareThemeBackdropsStep struct{}

func (s *GetChatShareThemeBackdropsStep) Name() string { return "getChatShareThemeBackdrops" }

func (s *GetChatShareThemeBackdropsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res, err := rt.Sender.GetChatShareThemeBackdrops(ctx)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, errors.New("getChatShareThemeBackdrops: no result")
	}
	seen := make(map[int]bool, len(res.Backdrops))
	emptyNames := 0
	minID, maxID := 0, 0
	for i, b := range res.Backdrops {
		if seen[b.ID] {
			return nil, fmt.Errorf("getChatShareThemeBackdrops: duplicate backdrop id %d", b.ID)
		}
		seen[b.ID] = true
		if err := checkBackdropColors(b.ID, b.Colors); err != nil {
			return nil, err
		}
		if b.Name == "" {
			emptyNames++
		}
		if i == 0 || b.ID < minID {
			minID = b.ID
		}
		if i == 0 || b.ID > maxID {
			maxID = b.ID
		}
	}
	evidence := map[string]any{"count": len(res.Backdrops), "empty_names": emptyNames}
	if len(res.Backdrops) > 0 {
		evidence["min_id"] = minID
		evidence["max_id"] = maxID
	}
	return &StepResult{StepName: s.Name(), Method: "getChatShareThemeBackdrops", Evidence: evidence}, nil
}

// checkBackdropColors checks that each color is an RGB value, 0..0xFFFFFF.
func checkBackdropColors(id int, c tg.UniqueGiftBackdropColors) error {
	for _, f := range []struct {
		name  string
		value int
	}{{"center_color", c.CenterColor}, {"edge_color", c.EdgeColor}, {"symbol_color", c.SymbolColor}, {"text_color", c.TextColor}} {
		if f.value < 0 || f.value > 0xFFFFFF {
			return fmt.Errorf("getChatShareThemeBackdrops: backdrop %d: %s %d is not an RGB color", id, f.name, f.value)
		}
	}
	return nil
}
