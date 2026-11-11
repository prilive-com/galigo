package suites

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/registry"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
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
		// API100B T-tb-1: the 10.0b live photo scenarios.
		{"S57-SendLivePhoto", []string{"sendLivePhoto"}, []engine.Step{
			&engine.SendLivePhoto100Step{},
			&engine.SendLivePhoto100Step{ByFileID: true},
			&engine.CleanupStep{},
		}},
		{"S58-LivePhotoAlbum", []string{"sendMediaGroup"}, []engine.Step{
			&engine.SendLivePhoto100Step{},
			&engine.LivePhotoAlbum100Step{},
			&engine.LivePhotoAlbum100Step{ByFileID: true},
			&engine.CleanupStep{},
		}},
		{"S59-EditToLivePhoto", []string{"editMessageMedia"}, []engine.Step{
			&engine.SendLivePhoto100Step{},
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromBytes(fixtures.PhotoBytes(), "galigo-live-edit.jpg", "photo"),
				Caption: "galigo 10.0 live photo: plain photo, will be replaced",
			},
			&engine.EditToLivePhoto100Step{},
			&engine.CleanupStep{},
		}},
		// API100C T-tb-9: the 10.0c poll scenarios.
		{"S60-PollOptionCount", []string{"sendPoll"}, []engine.Step{
			&engine.SendPollSimpleStep{Question: "galigo 10.0 poll, 1 option", Options: []string{"only"}},
			&engine.SendPollSimpleStep{Question: "galigo 10.0 poll, 12 options", Options: []string{
				"o1", "o2", "o3", "o4", "o5", "o6", "o7", "o8", "o9", "o10", "o11", "o12"}},
			&engine.CleanupStep{},
		}},
		{"S61-PollMembersOnly", []string{"sendPoll"}, []engine.Step{
			&engine.SendPoll100Step{Req: sender.SendPollRequest{Question: "galigo 10.0 poll, members only",
				Options: []sender.InputPollOption{{Text: "a"}, {Text: "b"}}, MembersOnly: true}},
			&engine.CleanupStep{},
		}},
		{"S62-PollCountryCodes", []string{"sendPoll"}, []engine.Step{
			&engine.SendPoll100Step{Req: sender.SendPollRequest{Question: "galigo 10.0 poll, country codes",
				Options: []sender.InputPollOption{{Text: "a"}, {Text: "b"}}, CountryCodes: []string{"DE", "FT"}}},
			&engine.CleanupStep{},
		}},
		// API100D T-tb-7: the 10.0d poll media scenarios (plan §8 T).
		{"S63-PollMediaDescription", []string{"sendPoll"}, []engine.Step{
			&engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.0 poll, description photo",
				Options: plain(), Description: "d", Media: ptr(sender.NewInputMediaPhoto(
					sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg")))},
				WantMedia: "photo", WantOptions: []string{"", ""}},
			&engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.0 poll, description location",
				Options: plain(), Media: ptr(sender.NewInputMediaLocation(52.52, 13.405))},
				WantMedia: "location", WantOptions: []string{"", ""}},
			&engine.CleanupStep{},
		}},
		{"S64-PollOptionMedia", []string{"sendPoll"}, []engine.Step{s64Step(fixtures.StickerBytes()), &engine.CleanupStep{}}},
		{"S65-QuizExplanationMedia", []string{"sendPoll"}, []engine.Step{
			&engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.0 quiz, explanation media",
				Type: "quiz", IsAnonymous: ptr(false), Options: []sender.InputPollOption{
					{Text: "photo", Media: ptr(sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg")))},
					{Text: "plain"}},
				CorrectOptionIDs: []int{0}, Explanation: "e",
				ExplanationMedia: ptr(sender.NewInputMediaDocument(sender.FromBytes(fixtures.DocumentBytes(), "document.txt")))},
				WantOptions: []string{"photo", ""}, WantExplanation: "document"},
			&engine.CleanupStep{},
		}},
	}

	got := AllAPI100Scenarios()
	require.Len(t, got, len(want))
	for i, w := range want {
		t.Run(w.name, func(t *testing.T) {
			assert.Equal(t, w.name, got[i].Name())
			assert.Equal(t, w.covers, got[i].Covers())
			// whole values: the step types in order and each step's input (viewSteps reads an upload's bytes)
			assert.Equal(t, viewSteps(w.steps), viewSteps(got[i].Steps()))
		})
	}

	t.Run("live-photos sub-suite", func(t *testing.T) { // API100B T-tb-1
		var names []string
		for _, s := range AllLivePhotoScenarios() {
			names = append(names, s.Name())
		}
		assert.Equal(t, []string{"S57-SendLivePhoto", "S58-LivePhotoAlbum", "S59-EditToLivePhoto"}, names)
	})

	t.Run("registry sendLivePhoto", func(t *testing.T) { // API100B T-tb-1 (T3-07: the whole entry)
		var found []registry.Method
		for _, m := range registry.AllMethods {
			if m.Name == "sendLivePhoto" {
				found = append(found, m)
			}
		}
		assert.Equal(t, []registry.Method{{Name: "sendLivePhoto", Category: registry.CategoryMessaging}}, found)
	})

	t.Run("registry", func(t *testing.T) {
		names := registry.MethodNames()
		for _, m := range []string{"answerGuestQuery", "deleteMessageReaction", "deleteAllMessageReactions",
			"getManagedBotAccessSettings", "setManagedBotAccessSettings", "getUserPersonalChatMessages"} {
			assert.Contains(t, names, m)
		}
	})
}

func ptr[T any](v T) *T { return &v }

func plain() []sender.InputPollOption { return []sender.InputPollOption{{Text: "a"}, {Text: "b"}} }

// s64Step is S64's poll step with the given sticker bytes (API100D T-tb-7, T-tb-8).
func s64Step(sticker []byte) *engine.SendPollMediaStep {
	loc := sender.NewInputMediaLocation(52.52, 13.405)
	loc.HorizontalAccuracy = 50
	st := sender.NewInputMediaSticker(sender.FromBytes(sticker, "sticker.png"))
	st.Emoji = "👍"
	return &engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.0 poll, option media",
		Options: []sender.InputPollOption{
			{Text: "location", Media: &loc},
			{Text: "venue", Media: ptr(sender.NewInputMediaVenue(48.8584, 2.2945, "Eiffel Tower", "Paris"))},
			{Text: "sticker", Media: &st},
			{Text: "photo", Media: ptr(sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg")))},
			{Text: "plain"}}},
		WantOptions: []string{"location", "venue", "sticker", "photo", ""}}
}

// api100Scenario returns the scenario of AllAPI100Scenarios with this name, or fails.
func api100Scenario(t *testing.T, name string) engine.Scenario {
	t.Helper()
	for _, s := range AllAPI100Scenarios() {
		if s.Name() == name {
			return s
		}
	}
	t.Fatalf("scenario %s not found", name)
	return nil
}

// T-tb-8 (API100D, T1-02 google): the T-tb-6 view reads the bytes of every upload, top level, option and nested
// live-photo still, so a changed byte makes it unequal and two fresh builds are equal.
func TestAPI100D_ViewSeesBytes(t *testing.T) {
	t.Run("S64", func(t *testing.T) {
		a := api100Scenario(t, "S64-PollOptionMedia").Steps()[0]
		b := api100Scenario(t, "S64-PollOptionMedia").Steps()[0]
		assert.Equal(t, viewStep(a), viewStep(b))
		changed := bytes.Clone(fixtures.StickerBytes())
		changed[len(changed)-1] ^= 1
		assert.Equal(t, viewStep(a), viewStep(s64Step(fixtures.StickerBytes())))
		assert.NotEqual(t, viewStep(a), viewStep(s64Step(changed)))
	})
	t.Run("live_photo_still", func(t *testing.T) {
		v, p := []byte("video bytes"), []byte("still bytes")
		build := func(v, p []byte) engine.Step {
			lp := sender.NewInputMediaLivePhoto(sender.FromBytes(v, "v.mp4"), sender.FromBytes(p, "p.jpg"))
			return &engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "Q",
				Options: []sender.InputPollOption{{Text: "a", Media: &lp}, {Text: "b"}}}}
		}
		assert.Equal(t, viewStep(build(bytes.Clone(v), bytes.Clone(p))), viewStep(build(bytes.Clone(v), bytes.Clone(p))))
		p2 := bytes.Clone(p)
		p2[0] ^= 1
		assert.NotEqual(t, viewStep(build(bytes.Clone(v), bytes.Clone(p))), viewStep(build(bytes.Clone(v), p2)))
	})
}

// fakeLive is a SenderClient for running S58 on the real runner: it embeds the interface (any other method,
// such as the cleanup's deleteMessage, panics) and answers sendLivePhoto and sendMediaGroup.
type fakeLive struct {
	engine.SenderClient
	album   []*tg.Message
	deleted [][2]int64 // B-30: the runner's cleanup after a stop deletes what the failed scenario tracked
}

func (f *fakeLive) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	f.deleted = append(f.deleted, [2]int64{chatID, int64(messageID)})
	return nil
}

func (f *fakeLive) SendLivePhoto(_ context.Context, _ int64, _, _ engine.MediaInput, _ string) (*tg.Message, error) {
	return liveMsg(901), nil
}

func (f *fakeLive) SendMediaGroup(_ context.Context, _ int64, _ []engine.MediaInput) ([]*tg.Message, error) {
	return f.album, nil
}

func liveMsg(id int) *tg.Message {
	sizes := []tg.PhotoSize{{FileID: "still-90"}, {FileID: "still"}}
	return &tg.Message{MessageID: id, Chat: &tg.Chat{ID: testutil.TestChatID},
		LivePhoto: &tg.LivePhoto{FileID: "video", Photo: sizes}, Photo: sizes}
}

// T-tb-8: a failed album check keeps the album's message ids in the scenario report, and cleanup does not run.
func TestAPI100B_S58FailedAlbumKeepsIDs(t *testing.T) { // API100B T-tb-8
	bad := liveMsg(912)
	bad.LivePhoto = nil
	f := &fakeLive{album: []*tg.Message{liveMsg(911), bad}}
	rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))

	res := runner.Run(context.Background(), S58_LivePhotoAlbum())
	b, err := json.Marshal(res)
	require.NoError(t, err)
	var report struct {
		Success bool `json:"success"`
		Steps   []struct {
			StepName   string `json:"step_name"`
			Method     string `json:"method"`
			MessageIDs []int  `json:"message_ids"`
		} `json:"steps"`
	}
	require.NoError(t, json.Unmarshal(b, &report))
	assert.False(t, report.Success)
	require.Len(t, report.Steps, 3, "the scenario's own cleanup step did not run; the runner's cleanup after stop did")
	assert.Equal(t, "sendMediaGroup", report.Steps[1].Method)
	assert.Equal(t, []int{911, 912}, report.Steps[1].MessageIDs)
	assert.Equal(t, "deleteMessage", report.Steps[2].Method) // B-30: cleanup after stop
	assert.Equal(t, "cleanup after stop", report.Steps[2].StepName)
	assert.Equal(t, [][2]int64{{testutil.TestChatID, 901}, {testutil.TestChatID, 911}, {testutil.TestChatID, 912}}, f.deleted)
	assert.Empty(t, rt.CreatedMessages)
}
