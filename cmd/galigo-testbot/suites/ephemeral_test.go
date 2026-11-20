package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// B-65 65a T-sc: S73's shape. The reply step first (it needs no admin rights), then the two sends, then the cleanup.
// B-65 65b: each send is followed by its two edits.
func TestB65A_Scenario(t *testing.T) {
	var names []string
	for _, sc := range AllEphemeralScenarios() {
		names = append(names, sc.Name())
	}
	assert.Equal(t, []string{"S73-EphemeralMessages"}, names)
	assert.Equal(t, "65000000000000001", EphemeralFakeCallbackQueryID) // the id the live probe used

	s := S73_EphemeralMessages()
	assert.Equal(t, []string{"sendMessage", "sendPhoto", "deleteEphemeralMessage", "editEphemeralMessageText",
		"editEphemeralMessageReplyMarkup", "editEphemeralMessageCaption", "editEphemeralMessageMedia"}, s.Covers())
	assert.Equal(t, 90*time.Second, s.Timeout())
	assert.Equal(t, []engine.Step{
		&engine.EphemeralReplyStep{CallbackQueryID: "65000000000000001"},
		&engine.SendEphemeralStep{Method: "sendMessage"},
		&engine.EditEphemeralStep{Method: "editEphemeralMessageText"},
		&engine.EditEphemeralStep{Method: "editEphemeralMessageReplyMarkup"},
		&engine.SendEphemeralStep{Method: "sendPhoto"},
		&engine.EditEphemeralStep{Method: "editEphemeralMessageCaption"},
		&engine.EditEphemeralStep{Method: "editEphemeralMessageMedia"},
		&engine.CleanupStep{},
	}, s.Steps())
}
