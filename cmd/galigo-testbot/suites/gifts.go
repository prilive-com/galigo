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

// AllGiftScenarios returns all gift scenarios.
func AllGiftScenarios() []engine.Scenario {
	return []engine.Scenario{
		S23_Gifts(),
	}
}
