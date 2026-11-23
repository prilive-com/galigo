package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S23_Gifts tests gift availability retrieval (read-only, safe).
func S23_Gifts() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S23-Gifts",
		ScenarioDescription: "Get available gifts catalog",
		CoveredMethods:      []string{"getAvailableGifts"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetAvailableGiftsStep{},
		},
	}
}

// S80_OwnedGifts reads the owned gift lists (no message is sent). The business row comes first and is a refusal
// only (the bot has no business connection); a skip ends the scenario, so the user row (skipped with no admin) and
// the chat row (skipped outside a group or supergroup) come after it.
func S80_OwnedGifts() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S80-OwnedGifts",
		ScenarioDescription: "Owned gift lists: getBusinessAccountGifts refused on a fake business connection; getUserGifts for the first admin and getChatGifts for the test chat (group or supergroup), one gift each",
		CoveredMethods:      []string{"getBusinessAccountGifts", "getUserGifts", "getChatGifts"},
		ScenarioSteps: []engine.Step{
			&engine.GetBusinessAccountGiftsStep{},
			&engine.GetUserGiftsStep{},
			&engine.GetChatGiftsStep{},
		},
		ScenarioTimeout: 30 * time.Second,
	}
}

// AllGiftScenarios returns all gift scenarios.
func AllGiftScenarios() []engine.Scenario {
	return []engine.Scenario{
		S23_Gifts(),
		S80_OwnedGifts(),
	}
}
