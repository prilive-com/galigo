package suites

import (
	"context"
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

// s66Steps are S66's steps (B-25 10.1a, TEST-PLAN T-tb-7) with the given first URL and second wanted link.
func s66Steps(url1, wantLink2 string) []engine.Step {
	return []engine.Step{
		&engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.1 poll, option links",
			Options: []sender.InputPollOption{
				{Text: "link 1", Media: ptr(sender.NewInputMediaLink(url1))},
				{Text: "link 2", Media: ptr(sender.NewInputMediaLink("https://example.com/2"))},
				{Text: "plain"}}},
			WantOptions: []string{"link", "link", ""}, WantLinks: []string{"https://example.com/1", wantLink2, ""}},
		&engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.1 poll, link and upload",
			Options: []sender.InputPollOption{
				{Text: "link", Media: ptr(sender.NewInputMediaLink("https://example.com/l"))},
				{Text: "photo", Media: ptr(sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg")))}}},
			WantOptions: []string{"link", "photo"}, WantLinks: []string{"https://example.com/l", ""}},
		&engine.CleanupStep{},
	}
}

// T-tb-7, T-tb-8: the 10.1 suite is S66 then S67 (B-25 10.1d T-tb-5), with these steps (whole values through
// viewSteps); the view sees a changed URL and a changed wanted link.
func TestAPI101Scenarios(t *testing.T) {
	got := AllAPI101Scenarios()
	require.Len(t, got, 2)
	assert.Equal(t, "S66-PollOptionLink", got[0].Name())
	assert.Equal(t, []string{"sendPoll"}, got[0].Covers())
	assert.Equal(t, viewSteps(s66Steps("https://example.com/1", "https://example.com/2")), viewSteps(got[0].Steps()))

	// B-25 10.1d T-tb-5: S67, its covered methods and its exact steps.
	assert.Equal(t, "S67-RichMessage", got[1].Name())
	assert.Equal(t, []string{"sendRichMessage", "editMessageText", "sendRichMessageDraft"}, got[1].Covers())
	assert.Equal(t, viewSteps([]engine.Step{
		&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Markdown: "galigo 10.1 **rich** message"},
			WantBlocks: []string{"paragraph"}},
		&engine.EditMessageRichStep{RichMessage: tg.InputRichMessage{HTML: "<p>edited <b>rich</b></p>"},
			WantBlocks: []string{"paragraph"}},
		&engine.CleanupStep{}, // before the draft: a Skip ends the scenario (C1-01)
		&engine.SendRichMessageDraftStep{DraftID: 1, RichMessage: tg.InputRichMessage{Markdown: "galigo 10.1 draft"}},
	}), viewSteps(got[1].Steps()))

	t.Run("view_sees_links", func(t *testing.T) {
		base := viewSteps(s66Steps("https://example.com/1", "https://example.com/2"))
		assert.Equal(t, base, viewSteps(S66_PollOptionLink().Steps()), "two fresh builds are equal")
		assert.NotEqual(t, base, viewSteps(s66Steps("https://example.com/9", "https://example.com/2")))
		assert.NotEqual(t, base, viewSteps(s66Steps("https://example.com/1", "https://example.com/9")))
	})

	// B-25 10.1b T-tb-1: the two join request query methods are in the registry, once each, with no scenario
	// (they need a guard bot).
	t.Run("registry_join_request_queries", func(t *testing.T) {
		for _, name := range []string{"answerChatJoinRequestQuery", "sendChatJoinRequestWebApp"} {
			var found []registry.Method
			for _, m := range registry.AllMethods {
				if m.Name == name {
					found = append(found, m)
				}
			}
			require.Len(t, found, 1, name)
			assert.Equal(t, registry.Method{Name: name, Category: registry.CategoryChatAdmin,
				Notes: "needs a join request query (guard bot)"}, found[0])
		}
	})

	// B-50 R1: approveChatJoinRequest and declineChatJoinRequest are in the registry, once each, with no scenario
	// (a real decision needs a pending join request from a second account).
	t.Run("registry_join_request_decisions", func(t *testing.T) {
		for _, name := range []string{"approveChatJoinRequest", "declineChatJoinRequest"} {
			var found []registry.Method
			for _, m := range registry.AllMethods {
				if m.Name == name {
					found = append(found, m)
				}
			}
			require.Len(t, found, 1, name)
			assert.Equal(t, registry.Method{Name: name, Category: registry.CategoryChatAdmin,
				Notes: "needs a pending join request (second account)"}, found[0])
		}
	})
}

// fake101dGroup runs S67 on the real runner as in a group: send and edit return a rich message, the draft is refused
// with the group error, and deleteMessage is recorded. Any other SenderClient method panics (embedded nil interface).
type fake101dGroup struct {
	engine.SenderClient
	deleted [][2]int64
}

func (f *fake101dGroup) richReply() *tg.Message {
	return &tg.Message{MessageID: 501, Chat: &tg.Chat{ID: testutil.TestChatID},
		RichMessage: &tg.RichMessage{Blocks: tg.RichBlocks{tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Plain: "p"}}}}}
}

func (f *fake101dGroup) SendRichMessage(context.Context, int64, tg.InputRichMessage) (*tg.Message, error) {
	return f.richReply(), nil
}

func (f *fake101dGroup) EditMessageRich(context.Context, int64, int, tg.InputRichMessage) (*tg.Message, error) {
	return f.richReply(), nil
}

func (f *fake101dGroup) SendRichMessageDraft(context.Context, int64, int, tg.InputRichMessage) error {
	return &tg.APIError{Code: 400, Description: "Bad Request: TEXTDRAFT_PEER_INVALID"}
}

func (f *fake101dGroup) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	f.deleted = append(f.deleted, [2]int64{chatID, int64(messageID)})
	return nil
}

// B-25 10.1d T-tb-7 (C1-01): in a group the draft is skipped, and the sent message is still deleted.
func TestAPI101D_S67GroupCleanup(t *testing.T) {
	f := &fake101dGroup{}
	rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))

	res := runner.Run(context.Background(), S67_RichMessage())
	assert.True(t, res.Success)
	assert.True(t, res.Skipped)
	assert.Equal(t, [][2]int64{{testutil.TestChatID, 501}}, f.deleted)
	assert.Empty(t, rt.CreatedMessages)
}
