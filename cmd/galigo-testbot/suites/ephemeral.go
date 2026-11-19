package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// EphemeralFakeCallbackQueryID is a callback query id the server does not know (S73).
const EphemeralFakeCallbackQueryID = "65000000000000001"

// S73_EphemeralMessages proves that ephemeral_message_parameters (Bot API 10.3/10.4) reaches the server. First an
// ephemeral reply to an unknown callback query without chat_id (10.4) must be refused by the callback check; this
// step needs no admin rights. Then, if the bot is an administrator of the test chat (else the scenario is skipped
// here), an ephemeral message to the first admin on the JSON path (sendMessage) and the form path (sendPhoto with an
// upload), which the cleanup deletes with deleteEphemeralMessage.
func S73_EphemeralMessages() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName: "S73-EphemeralMessages",
		ScenarioDescription: "ephemeral_message_parameters: a reply without chat_id (10.4), sendMessage (JSON) and " +
			"sendPhoto (form) to the admin, deleteEphemeralMessage (10.3)",
		CoveredMethods:  []string{"sendMessage", "sendPhoto", "deleteEphemeralMessage"},
		ScenarioTimeout: 60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.EphemeralReplyStep{CallbackQueryID: EphemeralFakeCallbackQueryID},
			&engine.SendEphemeralStep{Method: "sendMessage"},
			&engine.SendEphemeralStep{Method: "sendPhoto"},
			&engine.CleanupStep{},
		},
	}
}

// AllEphemeralScenarios returns the ephemeral message scenarios (Bot API 10.3/10.4).
func AllEphemeralScenarios() []engine.Scenario {
	return []engine.Scenario{S73_EphemeralMessages()}
}
