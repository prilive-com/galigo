package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65h: GetChatShareThemeBackdropsStep (TEST-PLAN section 4). The fake returns one scripted answer and records
// the context and the call count.

type b65hFake struct {
	SenderClient
	res   *tg.ChatShareThemeBackdrops
	err   error
	ctx   context.Context
	calls int
}

func (f *b65hFake) GetChatShareThemeBackdrops(ctx context.Context) (*tg.ChatShareThemeBackdrops, error) {
	f.ctx = ctx
	f.calls++
	return f.res, f.err
}

func b65hBackdrop(id int, name string) tg.ChatShareThemeBackdrop {
	return tg.ChatShareThemeBackdrop{ID: id, Name: name, Colors: tg.UniqueGiftBackdropColors{CenterColor: 1, EdgeColor: 2, SymbolColor: 3, TextColor: 4}}
}

func b65hList(bs ...tg.ChatShareThemeBackdrop) *tg.ChatShareThemeBackdrops {
	return &tg.ChatShareThemeBackdrops{Backdrops: bs}
}

func b65hRun(t *testing.T, f *b65hFake) (*StepResult, error) {
	t.Helper()
	return (&GetChatShareThemeBackdropsStep{}).Execute(context.Background(), NewRuntime(f, testutil.TestChatID, 0))
}

type b65hCtxKey struct{}

// E-ok: the whole result, with min and max inside an unsorted list (max not last, both outside 0..79), and the
// step's own context passed to the sender.
func TestB65H_StepOK(t *testing.T) {
	f := &b65hFake{res: b65hList(b65hBackdrop(5, "a"), b65hBackdrop(-1, "b"), b65hBackdrop(80, "c"), b65hBackdrop(2, "d"))}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), b65hCtxKey{}, "run"), time.Minute)
	defer cancel()
	res, err := (&GetChatShareThemeBackdropsStep{}).Execute(ctx, NewRuntime(f, testutil.TestChatID, 0))
	require.NoError(t, err)
	assert.Equal(t, &StepResult{StepName: "getChatShareThemeBackdrops", Method: "getChatShareThemeBackdrops",
		Evidence: map[string]any{"count": 4, "min_id": -1, "max_id": 80, "empty_names": 0}}, res)
	assert.Equal(t, 1, f.calls)
	assert.True(t, f.ctx == ctx, "the step passes its own context")
}

// E-empty and E-emptyname: an empty list (nil and non-nil) and empty names are valid replies (O-P3-02); the evidence
// counts them. negative_ids: a max that starts at 0 (A-T2-01); empty_name (all positive): a min that starts at 0.
func TestB65H_StepEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  *tg.ChatShareThemeBackdrops
		want map[string]any
	}{
		{"nil_list", &tg.ChatShareThemeBackdrops{}, map[string]any{"count": 0, "empty_names": 0}},
		{"empty_list", &tg.ChatShareThemeBackdrops{Backdrops: []tg.ChatShareThemeBackdrop{}}, map[string]any{"count": 0, "empty_names": 0}},
		{"empty_name", b65hList(b65hBackdrop(3, ""), b65hBackdrop(4, "x"), b65hBackdrop(6, "")), map[string]any{"count": 3, "min_id": 3, "max_id": 6, "empty_names": 2}},
		{"single", b65hList(b65hBackdrop(9, "a")), map[string]any{"count": 1, "min_id": 9, "max_id": 9, "empty_names": 0}}, // O-T3-05
		{"negative_ids", b65hList(b65hBackdrop(-5, "a"), b65hBackdrop(-2, "b"), b65hBackdrop(-9, "c")), map[string]any{"count": 3, "min_id": -9, "max_id": -2, "empty_names": 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := b65hRun(t, &b65hFake{res: tc.res})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.want, res.Evidence)
		})
	}
}

// E-dup: a duplicate id that is not next to its twin: once the first id (a step that does not record it), once a later id
// (a step that checks only the first id; O-T2-05, O-T3-04).
func TestB65H_StepDuplicate(t *testing.T) {
	for name, ids := range map[string][]int{"first": {3, 5, 3}, "later": {7, 3, 5, 3}} {
		t.Run(name, func(t *testing.T) {
			var bs []tg.ChatShareThemeBackdrop
			for _, id := range ids {
				bs = append(bs, b65hBackdrop(id, "a"))
			}
			_, err := b65hRun(t, &b65hFake{res: b65hList(bs...)})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "duplicate backdrop id 3")
		})
	}
}

// E-color and E-color-edge: each of the four colors alone out of 0..0xFFFFFF on both sides, the bad backdrop last
// (too high) or first (too low); the edges pass in both of two backdrops.
func TestB65H_StepColors(t *testing.T) {
	fields := []struct {
		name string
		set  func(*tg.UniqueGiftBackdropColors, int)
	}{
		{"center_color", func(c *tg.UniqueGiftBackdropColors, v int) { c.CenterColor = v }},
		{"edge_color", func(c *tg.UniqueGiftBackdropColors, v int) { c.EdgeColor = v }},
		{"symbol_color", func(c *tg.UniqueGiftBackdropColors, v int) { c.SymbolColor = v }},
		{"text_color", func(c *tg.UniqueGiftBackdropColors, v int) { c.TextColor = v }},
	}
	for _, fl := range fields {
		for _, v := range []int{0x1000000, -1} {
			t.Run(fmt.Sprintf("%s_%d", fl.name, v), func(t *testing.T) {
				b := b65hBackdrop(7, "a")
				fl.set(&b.Colors, v)
				list := b65hList(b65hBackdrop(6, "z"), b) // too high: the bad backdrop last
				if v < 0 {
					list = b65hList(b, b65hBackdrop(6, "z")) // too low: the bad backdrop first (A-T2-03)
				}
				_, err := b65hRun(t, &b65hFake{res: list})
				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("backdrop 7: %s %d", fl.name, v))
			})
		}
	}
	for _, v := range []int{0, 0xFFFFFF} {
		t.Run(fmt.Sprintf("edge_%d", v), func(t *testing.T) {
			c := tg.UniqueGiftBackdropColors{CenterColor: v, EdgeColor: v, SymbolColor: v, TextColor: v}
			_, err := b65hRun(t, &b65hFake{res: b65hList(tg.ChatShareThemeBackdrop{ID: 1, Name: "a", Colors: c}, tg.ChatShareThemeBackdrop{ID: 2, Name: "b", Colors: c})})
			assert.NoError(t, err)
		})
	}
}

// E-senderr and E-nilresult: a send error is returned as it is; a nil result is an error, not a panic.
func TestB65H_StepErrors(t *testing.T) {
	t.Run("send_error", func(t *testing.T) {
		cause := errors.New("send failed")
		res, err := b65hRun(t, &b65hFake{err: cause})
		assert.Nil(t, res)
		assert.ErrorIs(t, err, cause)
	})
	t.Run("nil_result", func(t *testing.T) {
		var res *StepResult
		var err error
		require.NotPanics(t, func() {
			res, err = b65hRun(t, &b65hFake{})
		})
		assert.Nil(t, res)
		assert.Error(t, err)
	})
}

// E-adapter (O-T3-02, draft discovery ad-ctx): the real adapter returns the sender's value and passes the caller's
// context; a canceled context sends no request.
func TestB65H_Adapter(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChatShareThemeBackdrops", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, json.RawMessage(`{"backdrops":[{"id":2,"name":"a","colors":{"center_color":1,"edge_color":2,"symbol_color":3,"text_color":4}}]}`))
	})
	a := NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL()))
	got, err := a.GetChatShareThemeBackdrops(context.Background())
	require.NoError(t, err)
	assert.Equal(t, b65hList(b65hBackdrop(2, "a")), got)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = a.GetChatShareThemeBackdrops(ctx)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, server.CaptureCount(), "the canceled call sends nothing")
}

// E-name.
func TestB65H_StepName(t *testing.T) {
	assert.Equal(t, "getChatShareThemeBackdrops", (&GetChatShareThemeBackdropsStep{}).Name())
}
