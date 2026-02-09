package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S0_Smoke is a quick sanity check.
func S0_Smoke() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S0-Smoke",
		ScenarioDescription: "Quick sanity check: getMe + sendMessage",
		CoveredMethods:      []string{"getMe", "sendMessage", "deleteMessage"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetMeStep{},
			&engine.SendMessageStep{Text: "galigo-testbot: smoke test"},
			&engine.DeleteLastMessageStep{},
		},
	}
}

// S1_Identity verifies bot identity.
func S1_Identity() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S1-Identity",
		ScenarioDescription: "Verify bot identity with getMe",
		CoveredMethods:      []string{"getMe"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetMeStep{},
		},
	}
}

// S2_MessageLifecycle tests send, edit, delete.
func S2_MessageLifecycle() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S2-MessageLifecycle",
		ScenarioDescription: "Send, edit, and delete a message",
		CoveredMethods:      []string{"sendMessage", "editMessageText", "deleteMessage"},
		ScenarioTimeout:     1 * time.Minute,
		ScenarioSteps: []engine.Step{
			&engine.SendMessageStep{Text: "galigo-testbot: message lifecycle test"},
			&engine.EditMessageTextStep{Text: "galigo-testbot: EDITED message"},
			&engine.DeleteLastMessageStep{},
		},
	}
}

// S4_ForwardCopy tests forward and copy operations.
func S4_ForwardCopy() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S4-ForwardCopy",
		ScenarioDescription: "Forward and copy messages",
		CoveredMethods:      []string{"sendMessage", "forwardMessage", "copyMessage"},
		ScenarioTimeout:     1 * time.Minute,
		ScenarioSteps: []engine.Step{
			&engine.SendMessageStep{Text: "galigo-testbot: source message for forward/copy"},
			&engine.ForwardMessageStep{}, // Forward to same chat
			&engine.CopyMessageStep{},    // Copy to same chat
			&engine.CleanupStep{},        // Delete all 3 messages
		},
	}
}

// S3_FormattedMessages tests ParseMode serialization in JSON requests (sendMessage).
// NOTE: This tests the JSON path only. For multipart (file uploads), see S36_MultipartParseMode.
// See: https://github.com/prilive-com/galigo/issues/5
func S3_FormattedMessages() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S3-FormattedMessages",
		ScenarioDescription: "Send messages with ParseMode (Markdown, HTML)",
		CoveredMethods:      []string{"sendMessage"},
		ScenarioTimeout:     1 * time.Minute,
		ScenarioSteps: []engine.Step{
			// Test Markdown formatting
			&engine.SendFormattedMessageStep{
				Text:      "galigo-testbot: *bold* _italic_ `code`",
				ParseMode: "Markdown",
			},
			&engine.DeleteLastMessageStep{},
			// Test HTML formatting
			&engine.SendFormattedMessageStep{
				Text:      "galigo-testbot: <b>bold</b> <i>italic</i> <code>code</code>",
				ParseMode: "HTML",
			},
			&engine.DeleteLastMessageStep{},
		},
	}
}

// S5_ChatAction tests chat action sending.
func S5_ChatAction() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S5-ChatAction",
		ScenarioDescription: "Send chat action (typing indicator)",
		CoveredMethods:      []string{"sendChatAction"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendChatActionStep{Action: "typing"},
		},
	}
}

// AllPhaseAScenarios returns all Phase A scenarios.
func AllPhaseAScenarios() []engine.Scenario {
	return []engine.Scenario{
		S0_Smoke(),
		S1_Identity(),
		S2_MessageLifecycle(),
		S3_FormattedMessages(),
		S4_ForwardCopy(),
		S5_ChatAction(),
	}
}

// PhaseACoveredMethods returns all methods covered by Phase A.
func PhaseACoveredMethods() []string {
	methods := make(map[string]bool)
	for _, s := range AllPhaseAScenarios() {
		for _, m := range s.Covers() {
			methods[m] = true
		}
	}

	result := make([]string, 0, len(methods))
	for m := range methods {
		result = append(result, m)
	}
	return result
}
