package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S12_CallbackQuery tests the full callback query flow:
// send message with inline keyboard → wait for user click → answer callback query.
// This scenario requires user interaction and is excluded from --run all.
func S12_CallbackQuery() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S12_CallbackQuery",
		ScenarioDescription: "Test answerCallbackQuery (interactive — requires user to click a button)",
		CoveredMethods:      []string{"sendMessage", "answerCallbackQuery"},
		ScenarioTimeout:     90 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send message with inline keyboard for user to click
			&engine.SendMessageWithKeyboardStep{
				Text: "galigo interactive test — please click a button below:",
				Buttons: []engine.ButtonDef{
					{Text: "Click Me", CallbackData: "galigo_test_cb"},
				},
			},
			// Wait for user to click the button
			&engine.WaitForCallbackStep{
				Timeout: 60 * time.Second,
			},
			// Answer the callback query with a notification
			&engine.AnswerCallbackQueryStep{
				Text:      "galigo: callback received!",
				ShowAlert: false,
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// AllInteractiveScenarios returns all interactive scenarios (excluded from --run all).
func AllInteractiveScenarios() []engine.Scenario {
	return []engine.Scenario{
		S12_CallbackQuery(),
	}
}
