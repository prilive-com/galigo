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
// upload), which the cleanup deletes with deleteEphemeralMessage. Each is edited first with the Bot API 10.3 changes:
// the text message to a rich message with an uploaded photo, then its inline keyboard; the photo's caption with
// show_caption_above_media, then its media with an upload.
func S73_EphemeralMessages() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName: "S73-EphemeralMessages",
		ScenarioDescription: "ephemeral_message_parameters: a reply without chat_id (10.4), sendMessage (JSON) and " +
			"sendPhoto (form) to the admin (10.3), the four edit methods with the 10.3 changes, deleteEphemeralMessage",
		CoveredMethods: []string{"sendMessage", "sendPhoto", "deleteEphemeralMessage", "editEphemeralMessageText",
			"editEphemeralMessageReplyMarkup", "editEphemeralMessageCaption", "editEphemeralMessageMedia"},
		ScenarioTimeout: 90 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.EphemeralReplyStep{CallbackQueryID: EphemeralFakeCallbackQueryID},
			&engine.SendEphemeralStep{Method: "sendMessage"},
			&engine.EditEphemeralStep{Method: "editEphemeralMessageText"},
			&engine.EditEphemeralStep{Method: "editEphemeralMessageReplyMarkup"},
			&engine.SendEphemeralStep{Method: "sendPhoto"},
			&engine.EditEphemeralStep{Method: "editEphemeralMessageCaption"},
			&engine.EditEphemeralStep{Method: "editEphemeralMessageMedia"},
			&engine.CleanupStep{},
		},
	}
}

// AllEphemeralScenarios returns the ephemeral message scenarios (Bot API 10.3/10.4).
func AllEphemeralScenarios() []engine.Scenario {
	return []engine.Scenario{S73_EphemeralMessages()}
}
