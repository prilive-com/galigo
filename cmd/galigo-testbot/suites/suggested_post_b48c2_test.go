package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// B-48c2 S6: S72's shape. The target message comes first, then each decide step by value, then the cleanup.
func TestB48C2_Scenario(t *testing.T) {
	var names []string
	for _, sc := range AllSuggestedPostScenarios() {
		names = append(names, sc.Name())
	}
	assert.Equal(t, []string{"S71-SuggestedPostParameters", "S72-DecideSuggestedPost"}, names)

	before := time.Now().Unix()
	s := S72_DecideSuggestedPost()
	after := time.Now().Unix()
	assert.Equal(t, "S72-DecideSuggestedPost", s.Name())
	assert.Equal(t, []string{"approveSuggestedPost", "declineSuggestedPost"}, s.Covers())
	assert.Equal(t, 60*time.Second, s.Timeout())
	want := []*engine.DecideSuggestedPostStep{
		{Method: "approveSuggestedPost", SendDate: -1, WantError: engine.SuggestedPostNotUsable}, // SendDate: checked below
		{Method: "approveSuggestedPost", Missing: true, WantError: engine.SuggestedPostNotFound},
		{Method: "declineSuggestedPost", Comment: "galigo test", WantError: engine.SuggestedPostNotUsable},
		{Method: "declineSuggestedPost", Missing: true, WantError: engine.SuggestedPostNotFound},
	}
	steps := s.Steps()
	require.Len(t, steps, len(want)+2)
	assert.Equal(t, &engine.SendMessageStep{Text: "galigo suggested post target"}, steps[0])
	for i, w := range want {
		require.IsType(t, &engine.DecideSuggestedPostStep{}, steps[i+1], "step %d", i+1)
		got := *steps[i+1].(*engine.DecideSuggestedPostStep)
		if w.SendDate == -1 {
			// one hour ahead of the time the scenario was built
			assert.GreaterOrEqual(t, got.SendDate, before+3600, "step %d SendDate", i+1)
			assert.LessOrEqual(t, got.SendDate, after+3600, "step %d SendDate", i+1)
			got.SendDate = -1
		}
		assert.Equal(t, *w, got, "step %d", i+1)
	}
	assert.IsType(t, &engine.CleanupStep{}, steps[len(want)+1])
}
