package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/tg"
)

// S71_SuggestedPostParameters proves that suggested_post_parameters (Bot API 9.2) reaches the server on the JSON
// path (sendMessage) and the form path (sendPhoto with an upload), with its price. The test chat is not a channel
// direct messages chat, so the server refuses every call with a 400 whose text shows what it parsed; nothing is
// posted. If a call is taken, the step fails and the cleanup deletes the message.
func S71_SuggestedPostParameters() engine.Scenario {
	priced := func(amount int64) *tg.SuggestedPostParameters {
		return &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: amount}}
	}
	return &engine.BaseScenario{
		ScenarioName:        "S71-SuggestedPostParameters",
		ScenarioDescription: "suggested_post_parameters reaches the server: sendMessage (JSON) and sendPhoto (form), with the price (9.2)",
		CoveredMethods:      []string{"sendMessage", "sendPhoto"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendSuggestedPostStep{Method: "sendMessage", Params: &tg.SuggestedPostParameters{}, WantError: engine.SuggestedPostWrongChat},
			&engine.SendSuggestedPostStep{Method: "sendMessage", Params: priced(5), WantError: engine.SuggestedPostWrongChat},
			&engine.SendSuggestedPostStep{Method: "sendMessage", Params: priced(1), WantError: engine.SuggestedPostWrongAmount},
			&engine.SendSuggestedPostStep{Method: "sendPhoto", Params: priced(5), WantError: engine.SuggestedPostWrongChat},
			&engine.SendSuggestedPostStep{Method: "sendPhoto", Params: priced(1), WantError: engine.SuggestedPostWrongAmount},
			&engine.CleanupStep{},
		},
	}
}

// S72_DecideSuggestedPost proves that approveSuggestedPost and declineSuggestedPost (Bot API 9.2) reach the server
// and that their message_id is read. The test chat is not a channel direct messages chat, so every call must be
// refused: a plain message gives SuggestedPostNotUsable and a message id with no message gives SuggestedPostNotFound.
// The approve call also sends a send_date one hour ahead, and the decline call a comment.
func S72_DecideSuggestedPost() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S72-DecideSuggestedPost",
		ScenarioDescription: "approveSuggestedPost and declineSuggestedPost reach the server and read message_id (9.2)",
		CoveredMethods:      []string{"approveSuggestedPost", "declineSuggestedPost"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendMessageStep{Text: "galigo suggested post target"},
			&engine.DecideSuggestedPostStep{Method: "approveSuggestedPost", SendDate: time.Now().Add(time.Hour).Unix(), WantError: engine.SuggestedPostNotUsable},
			&engine.DecideSuggestedPostStep{Method: "approveSuggestedPost", Missing: true, WantError: engine.SuggestedPostNotFound},
			&engine.DecideSuggestedPostStep{Method: "declineSuggestedPost", Comment: "galigo test", WantError: engine.SuggestedPostNotUsable},
			&engine.DecideSuggestedPostStep{Method: "declineSuggestedPost", Missing: true, WantError: engine.SuggestedPostNotFound},
			&engine.CleanupStep{},
		},
	}
}

// AllSuggestedPostScenarios returns the suggested post scenarios (Bot API 9.2).
func AllSuggestedPostScenarios() []engine.Scenario {
	return []engine.Scenario{S71_SuggestedPostParameters(), S72_DecideSuggestedPost()}
}
