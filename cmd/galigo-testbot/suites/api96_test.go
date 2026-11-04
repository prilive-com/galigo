package suites

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/registry"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// T-tb-1 (B-25 slice 1, TEST-PLAN §4): the 9.6 scenarios, their covered methods, their exact step sequences and
// step inputs, and the registry lines. `package suites` because package engine cannot import suites.
func TestAPI96Scenarios(t *testing.T) {
	f := false
	opts := []sender.InputPollOption{{Text: "A"}, {Text: "B"}, {Text: "C"}}
	s47 := sender.SendPollRequest{Question: "galigo 9.6 quiz", Options: opts, Type: "quiz",
		CorrectOptionIDs: []int{0, 2}, AllowsMultipleAnswers: true, IsAnonymous: &f}
	s48 := sender.SendPollRequest{Question: "galigo 9.6 poll", Options: opts, IsAnonymous: &f, AllowsRevoting: &f,
		ShuffleOptions: true, AllowAddingOptions: true, HideResultsUntilCloses: true,
		Description: "galigo 9.6 description", OpenPeriod: 2628000}
	s49 := sender.SendPollRequest{Question: "galigo 9.6 reply", Options: opts, IsAnonymous: &f}
	button := tg.KeyboardButton{Text: "galigo 9.6 users", RequestUsers: &tg.KeyboardButtonRequestUsers{RequestID: 96}}

	want := []struct {
		name   string
		covers []string
		steps  []engine.Step
	}{
		{"S47-QuizCorrectOptionIDs", []string{"sendPoll", "stopPoll"}, []engine.Step{
			&engine.SendPoll96Step{Req: s47}, &engine.StopPoll96Step{}, &engine.SendQuizIDsStep{Index: 1}, &engine.CleanupStep{}}},
		{"S48-PollOptions96", []string{"sendPoll"}, []engine.Step{
			&engine.SendPoll96Step{Req: s48}, &engine.CleanupStep{}}},
		{"S49-ReplyToPollOption", []string{"sendPoll", "sendMessage"}, []engine.Step{
			&engine.SendPoll96Step{Req: s49}, &engine.ReplyToPollOptionStep{}, &engine.CleanupStep{}}},
		{"S50-PreparedKeyboardButton", []string{"savePreparedKeyboardButton"}, []engine.Step{
			&engine.SavePreparedKeyboardButtonStep{Button: button}, &engine.CleanupStep{}}},
	}

	got := AllAPI96Scenarios()
	require.Len(t, got, len(want))
	for i, w := range want {
		t.Run(w.name, func(t *testing.T) {
			assert.Equal(t, w.name, got[i].Name())
			assert.Equal(t, w.covers, got[i].Covers())
			// whole values: the step types in order and each step's input (Equal compares the pointed-to values)
			assert.Equal(t, w.steps, got[i].Steps())
		})
	}

	t.Run("registry", func(t *testing.T) {
		names := registry.MethodNames()
		for _, m := range []string{"getManagedBotToken", "replaceManagedBotToken", "savePreparedKeyboardButton"} {
			assert.Contains(t, names, m)
		}
	})
}
