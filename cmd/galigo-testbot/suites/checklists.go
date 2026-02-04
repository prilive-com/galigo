package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S24_Checklists tests checklist send and edit lifecycle.
func S24_Checklists() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S24-Checklists",
		ScenarioDescription: "Send and edit a checklist message",
		CoveredMethods:      []string{"sendChecklist", "editMessageChecklist"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendChecklistStep{
				Title: "[galigo-test] Test Checklist",
				Tasks: []string{"Task 1: Buy groceries", "Task 2: Write tests", "Task 3: Deploy"},
			},
			&engine.EditMessageChecklistStep{
				Title: "[galigo-test] Updated Checklist",
				Tasks: []engine.ChecklistTaskInput{
					{ID: 1, Text: "Task 1: Buy groceries (done)"},
					{ID: 2, Text: "Task 2: Write tests (done)"},
					{ID: 3, Text: "Task 3: Deploy"},
					{ID: 4, Text: "Task 4: Celebrate"},
				},
			},
			&engine.CleanupStep{},
		},
	}
}

// AllChecklistScenarios returns all checklist scenarios.
func AllChecklistScenarios() []engine.Scenario {
	return []engine.Scenario{
		S24_Checklists(),
	}
}
