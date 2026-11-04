package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 9.6 Scenarios ====================
//
// Every poll is non-anonymous: an anonymous poll does not allow adding options (live probe 2026-11-04).
// getManagedBotToken and replaceManagedBotToken have no scenario: they need a managed bot (can_manage_bots).

func api96Options() []sender.InputPollOption {
	return []sender.InputPollOption{{Text: "A"}, {Text: "B"}, {Text: "C"}}
}

func api96False() *bool {
	f := false
	return &f
}

// S47_QuizCorrectOptionIDs sends a quiz with two correct answers, stops it, then sends a quiz through SendQuiz.
func S47_QuizCorrectOptionIDs() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S47-QuizCorrectOptionIDs",
		ScenarioDescription: "Quiz with several correct answers, stopPoll, SendQuiz correct_option_ids (9.6)",
		CoveredMethods:      []string{"sendPoll", "stopPoll"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPoll96Step{Req: sender.SendPollRequest{
				Question: "galigo 9.6 quiz", Options: api96Options(), Type: "quiz",
				CorrectOptionIDs: []int{0, 2}, AllowsMultipleAnswers: true, IsAnonymous: api96False(),
			}},
			&engine.StopPoll96Step{},
			&engine.SendQuizIDsStep{Index: 1},
			&engine.CleanupStep{},
		},
	}
}

// S48_PollOptions96 sends a regular poll with the 9.6 options (allows_revoting=false is the non-default value).
func S48_PollOptions96() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S48-PollOptions96",
		ScenarioDescription: "Regular poll with 9.6 options and description (9.6)",
		CoveredMethods:      []string{"sendPoll"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPoll96Step{Req: sender.SendPollRequest{
				Question: "galigo 9.6 poll", Options: api96Options(), IsAnonymous: api96False(),
				AllowsRevoting: api96False(), ShuffleOptions: true, AllowAddingOptions: true,
				HideResultsUntilCloses: true, Description: "galigo 9.6 description", OpenPeriod: 2628000,
			}},
			&engine.CleanupStep{},
		},
	}
}

// S49_ReplyToPollOption sends a poll and replies to its option B.
func S49_ReplyToPollOption() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S49-ReplyToPollOption",
		ScenarioDescription: "Reply to a poll option with reply_parameters.poll_option_id (9.6)",
		CoveredMethods:      []string{"sendPoll", "sendMessage"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPoll96Step{Req: sender.SendPollRequest{
				Question: "galigo 9.6 reply", Options: api96Options(), IsAnonymous: api96False(),
			}},
			&engine.ReplyToPollOptionStep{},
			&engine.CleanupStep{},
		},
	}
}

// S50_PreparedKeyboardButton saves a request_users button for the admin user.
func S50_PreparedKeyboardButton() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S50-PreparedKeyboardButton",
		ScenarioDescription: "savePreparedKeyboardButton for the admin user (9.6)",
		CoveredMethods:      []string{"savePreparedKeyboardButton"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SavePreparedKeyboardButtonStep{Button: tg.KeyboardButton{
				Text: "galigo 9.6 users", RequestUsers: &tg.KeyboardButtonRequestUsers{RequestID: 96},
			}},
			&engine.CleanupStep{},
		},
	}
}

// AllAPI96Scenarios returns all Bot API 9.6 test scenarios.
func AllAPI96Scenarios() []engine.Scenario {
	return []engine.Scenario{
		S47_QuizCorrectOptionIDs(),
		S48_PollOptions96(),
		S49_ReplyToPollOption(),
		S50_PreparedKeyboardButton(),
	}
}
