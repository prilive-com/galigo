package suites

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/registry"
)

// T-tb-1 (B-25 slice 2a, TEST-PLAN §4): the 10.0a scenarios, their covered methods, their exact step sequences and
// step inputs, and the registry lines. `package suites` because package engine cannot import suites.
func TestAPI100Scenarios(t *testing.T) {
	// API100 T-tb-1
	want := []struct {
		name   string
		covers []string
		steps  []engine.Step
	}{
		{"S51-DeleteReactions", []string{"deleteMessageReaction", "deleteAllMessageReactions"}, []engine.Step{
			&engine.RequireDeleteMessages100Step{},
			&engine.SendMessageStep{Text: "galigo 10.0 reactions"},
			&engine.SetMessageReactionStep{Emoji: "👍"},
			&engine.DeleteReactions100Step{Kind: "single"},
			&engine.SetMessageReactionStep{Emoji: "👍"},
			&engine.DeleteReactions100Step{Kind: "all"},
			&engine.CleanupStep{},
		}},
		{"S52-ChatPermissionsReact", []string{"getChat"}, []engine.Step{&engine.ChatPermissionsReact100Step{}}},
		{"S53-AdminsWithBots", []string{"getChatAdministrators"}, []engine.Step{&engine.GetAdminsWithBots100Step{}}},
		{"S54-EmptyDraft", []string{"sendMessageDraft"}, []engine.Step{&engine.EmptyDraft100Step{}}},
	}

	got := AllAPI100Scenarios()
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
		for _, m := range []string{"answerGuestQuery", "deleteMessageReaction", "deleteAllMessageReactions",
			"getManagedBotAccessSettings", "setManagedBotAccessSettings", "getUserPersonalChatMessages"} {
			assert.Contains(t, names, m)
		}
	})
}
