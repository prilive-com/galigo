package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// B-145 C1: S80 owned gift lists (TEST-PLAN section 4). Every value is written out here; the business row is a
// refusal and comes first, because a skip ends the scenario.
func TestB145_S80(t *testing.T) {
	assert.Equal(t, &engine.BaseScenario{
		ScenarioName:        "S80-OwnedGifts",
		ScenarioDescription: "Owned gift lists: getBusinessAccountGifts refused on a fake business connection; getUserGifts for the first admin and getChatGifts for the test chat (group or supergroup), one gift each",
		CoveredMethods:      []string{"getBusinessAccountGifts", "getUserGifts", "getChatGifts"},
		ScenarioSteps: []engine.Step{
			&engine.GetBusinessAccountGiftsStep{},
			&engine.GetUserGiftsStep{},
			&engine.GetChatGiftsStep{},
		},
		ScenarioTimeout: 30 * time.Second,
	}, S80_OwnedGifts())
	var names []string
	for _, s := range AllGiftScenarios() {
		names = append(names, s.Name())
	}
	assert.Equal(t, []string{"S23-Gifts", "S80-OwnedGifts"}, names)
}
