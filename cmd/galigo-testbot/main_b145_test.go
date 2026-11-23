package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/evidence"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/suites"
	"github.com/prilive-com/galigo/internal/testutil"
)

// B-145: the real S80 through the real runner and adapter (TEST-PLAN E8, C5). In package main because suites and
// evidence import engine (O-T3-04). Fake ids only.

const (
	b145UID  = int64(7770007770)
	b145CID  = int64(-1009876543210)
	b145UIDs = "7770007770"
	b145Page = `{"ok":true,"result":{"total_count":0,"gifts":[]}}`
)

func b145Raw(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// b145S80 runs the real S80 on a mock whose replies are the given handlers (missing ones: S80's normal answers).
func b145S80(t *testing.T, admin int64, replies map[string]http.HandlerFunc) (*testutil.MockTelegramServer, *engine.ScenarioResult) {
	t.Helper()
	server := testutil.NewMockServer(t)
	def := map[string]http.HandlerFunc{
		"getBusinessAccountGifts": func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyError(w, 400, "Bad Request: business connection not found", nil)
		},
		"getUserGifts": b145Raw(b145Page),
		"getChat": func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyOK(w, map[string]any{"id": b145CID, "type": "supergroup", "accent_color_id": 0, "max_reaction_count": 0})
		},
		"getChatGifts": b145Raw(b145Page),
	}
	for m, h := range replies {
		def[m] = h
	}
	for m, h := range def {
		server.On("/bot"+testutil.TestToken+"/"+m, h)
	}
	rt := engine.NewRuntime(engine.NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), b145CID, admin)
	runner := engine.NewRunner(rt, engine.RunnerConfig{RetryOn429: true, Max429Retries: 1, MaxMessages: 10}, slog.New(slog.DiscardHandler))
	return server, runner.Run(context.Background(), suites.S80_OwnedGifts())
}

func b145Called(server *testutil.MockTelegramServer) []string {
	var got []string
	for _, c := range server.Captures() {
		got = append(got, strings.TrimPrefix(c.Path, "/bot"+testutil.TestToken+"/"))
	}
	return got
}

// E8: S80's order, its skip and its 429 repeat on the real runner.
func TestB145_S80Runner(t *testing.T) {
	t.Run("order", func(t *testing.T) {
		server, res := b145S80(t, b145UID, nil)
		assert.Equal(t, []string{"getBusinessAccountGifts", "getUserGifts", "getChat", "getChatGifts"}, b145Called(server))
		assert.True(t, res.Success, "error %q", res.Error)
		assert.False(t, res.Skipped)
		require.Len(t, res.Steps, 3)
		for _, s := range res.Steps {
			assert.Empty(t, s.MessageIDs)
			assert.Empty(t, s.EphemeralMessageIDs)
		}
	})
	t.Run("admin 0 skips at the user row", func(t *testing.T) {
		server, res := b145S80(t, 0, nil)
		assert.True(t, res.Skipped)
		assert.Equal(t, []string{"getBusinessAccountGifts"}, b145Called(server))
	})
	t.Run("429 repeated once", func(t *testing.T) {
		var n atomic.Int32
		server, res := b145S80(t, b145UID, map[string]http.HandlerFunc{"getUserGifts": func(w http.ResponseWriter, r *http.Request) {
			if n.Add(1) == 1 {
				testutil.ReplyRateLimit(w, 1)
				return
			}
			b145Raw(b145Page)(w, r)
		}})
		assert.True(t, res.Success, "error %q", res.Error)
		calls := 0
		for _, m := range b145Called(server) {
			if m == "getUserGifts" {
				calls++
			}
		}
		assert.Equal(t, 2, calls, "the user row is sent again once")
	})
}

// C5: a failed user row: the saved report and the summary name the step and never the user id.
func TestB145_ReportRedaction(t *testing.T) {
	server, res := b145S80(t, b145UID, map[string]http.HandlerFunc{"getUserGifts": func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyError(w, 400, "Bad Request: user "+b145UIDs+" is hidden", nil)
	}})
	n := 0
	for _, m := range b145Called(server) {
		if m == "getUserGifts" {
			n++
		}
	}
	require.Equal(t, 1, n, "the user request ran")
	require.False(t, res.Success)
	assert.Contains(t, res.Error, `step "getUserGifts" failed`)
	report := evidence.NewReport()
	report.AddScenario(res)
	report.Finalize()
	path, err := report.Save(t.TempDir())
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	summary := report.FormatSummary()
	assert.NotContains(t, string(raw), b145UIDs)
	assert.Contains(t, string(raw), "[user]")
	assert.NotContains(t, summary, b145UIDs)
	assert.Contains(t, summary, "[user]")
}
