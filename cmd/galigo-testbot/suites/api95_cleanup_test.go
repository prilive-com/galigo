package suites

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// B-55 (with B-30): when S45 stops after writing the member tag, the runner clears it. Rows T1-T8 of the B-30
// TEST-PLAN §4, and the S67/S68 rows of §2 (their own cleanup step did the work: no fallback cleanup).

// fake95 records every SetChatMemberTag call (chat, user, tag) and answers GetChatMember from a script.
type fake95 struct {
	engine.SenderClient
	tags      []string
	tagErrs   map[int]error // error of the n-th SetChatMemberTag call (0-based)
	tagAlways map[string]error
	members   []tg.ChatMember // n-th GetChatMember answer
	memberErr map[int]error
	gets      int
}

func (f *fake95) SetChatMemberTag(_ context.Context, chatID, userID int64, tag string) error {
	key := fmt.Sprintf("%d/%d/%s", chatID, userID, tag)
	n := len(f.tags)
	f.tags = append(f.tags, key)
	if err, ok := f.tagAlways[key]; ok {
		return err
	}
	return f.tagErrs[n]
}

func (f *fake95) GetChatMember(_ context.Context, _, _ int64) (tg.ChatMember, error) {
	n := f.gets
	f.gets++
	if err := f.memberErr[n]; err != nil {
		return nil, err
	}
	return f.members[n], nil
}

const admin95 = int64(555)

func run45(t *testing.T, f *fake95, testUser int64) (*engine.ScenarioResult, *engine.Runtime) {
	t.Helper()
	rt := engine.NewRuntime(f, testutil.TestChatID, admin95)
	rt.TestUserID = testUser
	r := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
	return r.Run(context.Background(), S45_MemberTags()), rt
}

func tagKey(user int64, tag string) string {
	return fmt.Sprintf("%d/%d/%s", testutil.TestChatID, user, tag)
}

func hasAfterStop(res *engine.ScenarioResult) bool {
	for _, s := range res.Steps {
		if s.StepName == "cleanup after stop" {
			return true
		}
	}
	return false
}

func TestB30_T1_ReadbackSkipClearsTag(t *testing.T) {
	f := &fake95{members: []tg.ChatMember{tg.ChatMemberAdministrator{}}}
	res, _ := run45(t, f, 0)
	assert.True(t, res.Skipped)
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, []string{tagKey(admin95, "galigo-test"), tagKey(admin95, "")}, f.tags)
	require.Len(t, res.Steps, 3)
	last := res.Steps[2]
	assert.Equal(t, "cleanup after stop", last.StepName)
	assert.True(t, last.Success)
	assert.Equal(t, map[string]any{"deleted_count": 0, "tags_cleared": 1}, last.Evidence)
}

func TestB30_T2_MismatchClearsTag(t *testing.T) {
	f := &fake95{members: []tg.ChatMember{tg.ChatMemberMember{Tag: "other"}}}
	res, _ := run45(t, f, 0)
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "tag mismatch")
	assert.Equal(t, []string{tagKey(admin95, "galigo-test"), tagKey(admin95, "")}, f.tags)
}

// Guard: a full S45 run makes exactly its own two writes and no fallback cleanup.
func TestB30_T3_FullRunNoExtraCall(t *testing.T) {
	f := &fake95{members: []tg.ChatMember{tg.ChatMemberMember{Tag: "galigo-test"}, tg.ChatMemberMember{}}}
	res, rt := run45(t, f, 0)
	assert.True(t, res.Success, res.Error)
	assert.False(t, res.Skipped)
	assert.Len(t, res.Steps, 4)
	assert.False(t, hasAfterStop(res))
	assert.Equal(t, []string{tagKey(admin95, "galigo-test"), tagKey(admin95, "")}, f.tags)
	_, err := (&engine.CleanupStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, []string{tagKey(admin95, "galigo-test"), tagKey(admin95, "")}, f.tags, "no tag record left after a full run")
}

func TestB30_T4_ClearFails(t *testing.T) {
	f := &fake95{members: []tg.ChatMember{tg.ChatMemberAdministrator{}},
		tagAlways: map[string]error{tagKey(admin95, ""): errors.New("clear-x")}}
	res, _ := run45(t, f, 0)
	assert.False(t, res.Success)
	assert.True(t, res.Skipped)
	assert.Contains(t, res.Error, fmt.Sprintf("member tag %d/%d: clear-x", testutil.TestChatID, admin95))
}

// [t3] A-T2-01: S45 stops at step 3 (its own clear fails once): the runner clears the tag.
func TestB30_T9_OwnClearFails(t *testing.T) {
	f := &fake95{members: []tg.ChatMember{tg.ChatMemberMember{Tag: "galigo-test"}}, tagErrs: map[int]error{1: errors.New("clear-net")}}
	res, _ := run45(t, f, 0)
	assert.False(t, res.Success)
	assert.Equal(t, []string{tagKey(admin95, "galigo-test"), tagKey(admin95, ""), tagKey(admin95, "")}, f.tags)
	require.Len(t, res.Steps, 4)
	last := res.Steps[3]
	assert.Equal(t, "cleanup after stop", last.StepName)
	assert.True(t, last.Success)
	assert.Equal(t, map[string]any{"deleted_count": 0, "tags_cleared": 1}, last.Evidence)
}

// Guard: a failed first write records nothing, so nothing is cleared.
func TestB30_T5_FirstWriteFails(t *testing.T) {
	f := &fake95{tagErrs: map[int]error{0: &tg.APIError{Code: 400, Description: "Bad Request: USER_NOT_PARTICIPANT"}}}
	res, _ := run45(t, f, 0)
	assert.False(t, res.Success)
	assert.Equal(t, []string{tagKey(admin95, "galigo-test")}, f.tags)
	assert.False(t, hasAfterStop(res))
}

func TestB30_T6_TestUserCleared(t *testing.T) {
	const testUser = int64(777)
	f := &fake95{members: []tg.ChatMember{tg.ChatMemberAdministrator{}}}
	res, _ := run45(t, f, testUser)
	assert.True(t, res.Skipped)
	assert.Equal(t, []string{tagKey(testUser, "galigo-test"), tagKey(testUser, "")}, f.tags)
}

// Guard: once step 3 cleared the tag, a failure in step 4 causes no extra clear.
func TestB30_T7_ClearedThenReadbackFails(t *testing.T) {
	f := &fake95{members: []tg.ChatMember{tg.ChatMemberMember{Tag: "galigo-test"}},
		memberErr: map[int]error{1: errors.New("get-x")}}
	res, _ := run45(t, f, 0)
	assert.False(t, res.Success)
	assert.Equal(t, []string{tagKey(admin95, "galigo-test"), tagKey(admin95, "")}, f.tags)
	assert.False(t, hasAfterStop(res))
}

// Guard: the CHAT_CREATOR_REQUIRED skip writes nothing, so nothing is cleared.
func TestB30_T8_CreatorRequiredSkip(t *testing.T) {
	f := &fake95{tagErrs: map[int]error{0: &tg.APIError{Code: 400, Description: "Bad Request: CHAT_CREATOR_REQUIRED"}}}
	res, _ := run45(t, f, 0)
	assert.True(t, res.Skipped)
	assert.Equal(t, []string{tagKey(admin95, "galigo-test")}, f.tags)
	assert.False(t, hasAfterStop(res))
}

// S67 and S68 in a group: the draft skips after the scenario's own CleanupStep, so nothing is left and the runner adds
// no fallback cleanup (guards; REGISTER-plan O-note-2).
func TestB30_S67S68_NoFallbackCleanup(t *testing.T) {
	t.Run("S67", func(t *testing.T) {
		f := &fake101dGroup{}
		rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		r := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
		sc := S67_RichMessage()
		res := r.Run(context.Background(), sc)
		assert.False(t, hasAfterStop(res))
		assert.Len(t, res.Steps, len(sc.Steps()))
		assert.Equal(t, [][2]int64{{testutil.TestChatID, 501}}, f.deleted)
	})
	t.Run("S68", func(t *testing.T) {
		f := &fake102dGroup{}
		rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		r := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
		sc := S68_RichBlocks()
		res := r.Run(context.Background(), sc)
		assert.False(t, hasAfterStop(res))
		assert.Len(t, res.Steps, len(sc.Steps()))
		assert.Equal(t, [][2]int64{{testutil.TestChatID, 502}}, f.deleted)
	})
}
