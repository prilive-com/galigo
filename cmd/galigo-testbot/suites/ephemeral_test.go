package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// B-65 65a T-sc: S73's shape. The reply step first (it needs no admin rights), then the two sends, then the cleanup.
func TestB65A_Scenario(t *testing.T) {
	var names []string
	for _, sc := range AllEphemeralScenarios() {
		names = append(names, sc.Name())
	}
	assert.Equal(t, []string{"S73-EphemeralMessages"}, names)
	assert.Equal(t, "65000000000000001", EphemeralFakeCallbackQueryID) // the id the live probe used

	s := S73_EphemeralMessages()
	assert.Equal(t, []string{"sendMessage", "sendPhoto", "deleteEphemeralMessage"}, s.Covers())
	assert.Equal(t, 60*time.Second, s.Timeout())
	steps := s.Steps()
	require.Len(t, steps, 4)
	assert.Equal(t, &engine.EphemeralReplyStep{CallbackQueryID: "65000000000000001"}, steps[0])
	assert.Equal(t, &engine.SendEphemeralStep{Method: "sendMessage"}, steps[1])
	assert.Equal(t, &engine.SendEphemeralStep{Method: "sendPhoto"}, steps[2])
	assert.Equal(t, &engine.CleanupStep{}, steps[3])
}
