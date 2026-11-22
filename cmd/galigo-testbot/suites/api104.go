package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// AllAPI104Scenarios returns the Bot API 10.4 scenarios (B-65 65f: S76; the 10.4 widget read side is decode-only,
// B-65 65e).
func AllAPI104Scenarios() []engine.Scenario {
	return []engine.Scenario{S76_Widgets()}
}

// S76_Widgets calls the five widget methods (Bot API 10.4), refusal only: the server refuses sendWidget for this bot
// (measured 2026-11-21 in the test group and in a private chat), so each row must get its measured 400 text. A
// refusal shows that the method reached the server and took that refusal branch; it does not show that every field
// was read (the same text comes without them), which the mock tests prove. The steps run only in a group or
// supergroup and skip elsewhere. The plain message is the target of the two editMessageWidgetData chat rows and is
// deleted by the cleanup.
func S76_Widgets() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S76-Widgets",
		ScenarioDescription: "Widget methods reach the server and get their measured refusals (Bot API 10.4)",
		CoveredMethods: []string{"sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData",
			"editSharedWidgetData", "sendMessage"},
		ScenarioTimeout: 60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.WidgetRefusalStep{Method: "sendWidget", WantError: engine.WidgetNotAllowed},
			&engine.SendMessageStep{Text: "galigo 10.4 widget target"},
			&engine.WidgetRefusalStep{Method: "sendSharedWidget", WantError: engine.WidgetIDInvalid},
			&engine.WidgetRefusalStep{Method: "answerWidgetQuery", WantError: engine.WidgetQueryInvalid},
			&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "target", WantError: engine.WidgetMessageInvalid},
			&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "missing", WantError: engine.WidgetMessageNotFound},
			&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "inline", WantError: engine.WidgetInlineInvalid},
			&engine.WidgetRefusalStep{Method: "editSharedWidgetData", WantError: engine.WidgetIDInvalid},
			&engine.CleanupStep{},
		},
	}
}
