package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S10_InlineKeyboard tests sending and editing inline keyboards.
func S10_InlineKeyboard() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S10_InlineKeyboard",
		ScenarioDescription: "Test sending message with inline keyboard and editing reply markup",
		CoveredMethods:      []string{"sendMessage", "editMessageReplyMarkup"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send message with inline keyboard
			&engine.SendMessageWithKeyboardStep{
				Text: "galigo keyboard test",
				Buttons: []engine.ButtonDef{
					{Text: "Button A", CallbackData: "test_a"},
					{Text: "Button B", CallbackData: "test_b"},
				},
			},
			// Edit keyboard: change buttons
			&engine.EditMessageReplyMarkupStep{
				Buttons: []engine.ButtonDef{
					{Text: "Updated A", CallbackData: "test_a_v2"},
					{Text: "Updated B", CallbackData: "test_b_v2"},
					{Text: "New C", CallbackData: "test_c"},
				},
			},
			// Remove keyboard
			&engine.EditMessageReplyMarkupStep{
				Buttons: nil, // removes keyboard
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// AllPhaseCScenarios returns all Phase C (keyboard/callback) scenarios.
func AllPhaseCScenarios() []engine.Scenario {
	return []engine.Scenario{
		S10_InlineKeyboard(),
	}
}
