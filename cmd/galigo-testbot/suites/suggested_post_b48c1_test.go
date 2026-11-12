package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/tg"
)

// B-48c1: S71's shape. Each send step is checked by value: method, parameters and the wanted server text.
func TestB48C1_Scenario(t *testing.T) {
	var names []string
	for _, sc := range AllSuggestedPostScenarios() {
		names = append(names, sc.Name())
	}
	assert.Equal(t, []string{"S71-SuggestedPostParameters"}, names)

	s := S71_SuggestedPostParameters()
	assert.Equal(t, "S71-SuggestedPostParameters", s.Name())
	assert.Equal(t, []string{"sendMessage", "sendPhoto"}, s.Covers())
	assert.Equal(t, 60*time.Second, s.Timeout())
	price := func(a int64) *tg.SuggestedPostParameters {
		return &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: a}}
	}
	want := []*engine.SendSuggestedPostStep{
		{Method: "sendMessage", Params: &tg.SuggestedPostParameters{}, WantError: engine.SuggestedPostWrongChat},
		{Method: "sendMessage", Params: price(5), WantError: engine.SuggestedPostWrongChat},
		{Method: "sendMessage", Params: price(1), WantError: engine.SuggestedPostWrongAmount},
		{Method: "sendPhoto", Params: price(5), WantError: engine.SuggestedPostWrongChat},
		{Method: "sendPhoto", Params: price(1), WantError: engine.SuggestedPostWrongAmount},
	}
	steps := s.Steps()
	require.Len(t, steps, len(want)+1)
	for i, w := range want {
		require.IsType(t, &engine.SendSuggestedPostStep{}, steps[i], "step %d", i)
		assert.Equal(t, w, steps[i], "step %d", i)
	}
	assert.IsType(t, &engine.CleanupStep{}, steps[len(want)])
	// the two server texts, as probed live on 2026-11-12 (E27/lookups/probe-b48c-raw-console.txt)
	assert.Equal(t, "suggested posts can be sent only to channel direct messages", engine.SuggestedPostWrongChat)
	assert.Equal(t, "invalid amount of Telegram Stars specified", engine.SuggestedPostWrongAmount)
}
