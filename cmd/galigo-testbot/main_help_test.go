package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/internal/testutil"
)

// T-tb-11 (B-25 slice 2c, TEST-PLAN §4, T3-03, T3-05): the help text that handleHelp really sends names the Bot
// API 10.0 range with the poll scenarios, the polls in the api100 line, and keeps the live-photos range.
func TestHandleHelp_Text(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyMessage(w, 1)
	})
	adapter := engine.NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL()))
	handleHelp(context.Background(), adapter, testutil.TestChatID)

	require.Equal(t, 1, server.CaptureCount())
	var body struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(server.LastCapture().Body, &body))
	lines := strings.Split(body.Text, "\n")
	assert.Contains(t, lines, "Bot API 10.0 (S51-S54, S57-S62):")
	assert.Contains(t, lines, "  live-photos - Live photos (S57-S59)")
	var api100 []string
	for _, l := range lines {
		if strings.HasPrefix(l, "  api100 ") {
			api100 = append(api100, l)
		}
	}
	require.Len(t, api100, 1)
	assert.Contains(t, api100[0], "polls")
}
