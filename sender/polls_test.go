package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== SendPollSimple ====================

func TestSendPollSimple(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPoll", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "What color?", req["question"])
		opts, ok := req["options"].([]any)
		require.True(t, ok)
		assert.Len(t, opts, 3)
		testutil.ReplyOK(w, map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": -100123, "type": "supergroup"},
			"date":       1234567890,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendPollSimple(context.Background(), int64(-100123), "What color?",
		[]string{"Red", "Green", "Blue"},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, msg.MessageID)
}

func TestSendPollSimple_Validation_EmptyQuestion(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendPollSimple(context.Background(), int64(-100123), "", []string{"A", "B"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "question")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSendPollSimple_Validation_TooFewOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendPollSimple(context.Background(), int64(-100123), "Q?", []string{"A"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSendPollSimple_Validation_TooManyOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendPollSimple(context.Background(), int64(-100123), "Q?",
		[]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== SendQuiz ====================

func TestSendQuiz(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPoll", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "quiz", req["type"])
		// AC4 (a), Bot API 9.6: SendQuiz sends correct_option_ids, not the deprecated correct_option_id.
		assert.Equal(t, []any{float64(1)}, req["correct_option_ids"])
		assert.NotContains(t, req, "correct_option_id")
		testutil.ReplyOK(w, map[string]any{
			"message_id": 2,
			"chat":       map[string]any{"id": -100123, "type": "supergroup"},
			"date":       1234567890,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendQuiz(context.Background(), int64(-100123), "Capital of France?",
		[]string{"London", "Paris", "Berlin"}, 1,
	)
	require.NoError(t, err)
	assert.Equal(t, 2, msg.MessageID)
}

func TestSendQuiz_Validation_InvalidIndex(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendQuiz(context.Background(), int64(-100123), "Q?",
		[]string{"A", "B"}, 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "correct_option_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== StopPoll ====================

func TestStopPoll(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/stopPoll", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":        "poll123",
			"question":  "Q?",
			"options":   []any{},
			"is_closed": true,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	poll, err := client.StopPoll(context.Background(), int64(-100123), 42)
	require.NoError(t, err)
	assert.True(t, poll.IsClosed)
}

func TestStopPoll_Validation_InvalidMessageID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.StopPoll(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestStopPoll_Error_BadRequest(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/stopPoll", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBadRequest(w, "poll has already been closed")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.StopPoll(context.Background(), int64(-100123), 42)

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.Code)
}

func TestPollOptions(t *testing.T) {
	opts := []sender.PollOption{
		sender.WithPollAnonymous(false),
		sender.WithMultipleAnswers(),
		sender.WithQuizExplanation("Because!", "HTML"),
		sender.WithPollOpenPeriod(60),
	}
	assert.Len(t, opts, 4)
}

// ==================== Bot API 9.6 (B-25 slice 1, TEST-PLAN §3) ====================

func api96PollServer(t *testing.T) *testutil.MockTelegramServer {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPoll", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 4242)
	})
	return server
}

// T-s-1 and T-s-2: SendPoll sends every 9.6 field exactly, and none of the new keys when they are unset.
func TestSendPoll96_Body(t *testing.T) {
	t.Run("T-s-1 all new fields", testSendPoll96AllNewFields)
	t.Run("T-s-2 no new fields", testSendPoll96NoNewFields)
}

// T-s-1: SendPoll with every 9.6 field sends the exact body.
func testSendPoll96AllNewFields(t *testing.T) {
	server := api96PollServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())
	f := false
	_, err := client.SendPoll(context.Background(), sender.SendPollRequest{
		ChatID:                 int64(-100123),
		Question:               "Q?",
		Options:                []sender.InputPollOption{{Text: "A"}, {Text: "B"}, {Text: "C"}},
		Type:                   "quiz",
		AllowsMultipleAnswers:  true,
		CorrectOptionIDs:       []int{0, 2},
		AllowsRevoting:         &f,
		ShuffleOptions:         true,
		AllowAddingOptions:     true,
		HideResultsUntilCloses: true,
		Description:            "d *x*",
		DescriptionParseMode:   tg.ParseModeMarkdownV2,
		DescriptionEntities:    []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":-100123,"question":"Q?","options":[{"text":"A"},{"text":"B"},{"text":"C"}],`+
		`"type":"quiz","allows_multiple_answers":true,"correct_option_ids":[0,2],"allows_revoting":false,`+
		`"shuffle_options":true,"allow_adding_options":true,"hide_results_until_closes":true,"description":"d *x*",`+
		`"description_parse_mode":"MarkdownV2","description_entities":[{"type":"bold","offset":0,"length":1}]}`,
		server.LastCapture().BodyString())
}

// T-s-2: SendPoll with none of the 9.6 fields sends none of the new keys.
func testSendPoll96NoNewFields(t *testing.T) {
	server := api96PollServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.SendPoll(context.Background(), sender.SendPollRequest{
		ChatID: int64(-100123), Question: "Q?", Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":-100123,"question":"Q?","options":[{"text":"A"},{"text":"B"}]}`,
		server.LastCapture().BodyString())
}

// T-s-3: SendQuiz sends correct_option_ids (never correct_option_id), and option callbacks that read or change
// CorrectOptionID or CorrectOptionIDs keep working (plan §5 P3).
func TestSendQuiz96_CorrectOptionIDs(t *testing.T) {
	const quizBody = `{"chat_id":-100123,"question":"Q?","options":[{"text":"A"},{"text":"B"},{"text":"C"}],"type":"quiz"`
	opts := []string{"A", "B", "C"}
	var recorded int
	rows := []struct {
		name  string
		index int
		opts  []sender.PollOption
		body  string
		after func(t *testing.T)
	}{
		{"index 0", 0, nil, quizBody + `,"correct_option_ids":[0]}`, nil},
		{"index 2 with multiple answers", 2, []sender.PollOption{sender.WithMultipleAnswers()},
			quizBody + `,"allows_multiple_answers":true,"correct_option_ids":[2]}`, nil},
		{"T-s-3 (a) callback reads *CorrectOptionID", 0, []sender.PollOption{func(r *sender.SendPollRequest) { recorded = *r.CorrectOptionID }},
			quizBody + `,"correct_option_ids":[0]}`,
			func(t *testing.T) { assert.Equal(t, 0, recorded, "the callback saw the index") }},
		{"T-s-3 (b) callback sets CorrectOptionID to 2", 0, []sender.PollOption{func(r *sender.SendPollRequest) { two := 2; r.CorrectOptionID = &two }},
			quizBody + `,"correct_option_ids":[2]}`, nil},
		{"T-s-3 (c) callback sets CorrectOptionIDs", 0, []sender.PollOption{func(r *sender.SendPollRequest) { r.CorrectOptionIDs = []int{0, 2} }},
			quizBody + `,"correct_option_ids":[0,2]}`, nil},
		{"T-s-3 (d) callback sets CorrectOptionID to nil", 0, []sender.PollOption{func(r *sender.SendPollRequest) { r.CorrectOptionID = nil }},
			quizBody + `}`, nil},
		{"T-s-3 (e) callback sets an empty CorrectOptionIDs", 0, []sender.PollOption{func(r *sender.SendPollRequest) { r.CorrectOptionIDs = []int{} }},
			quizBody + `,"correct_option_ids":[0]}`, nil},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			recorded = -1
			server := api96PollServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NotPanics(t, func() {
				_, err := client.SendQuiz(context.Background(), int64(-100123), "Q?", opts, r.index, r.opts...)
				require.NoError(t, err)
			})
			require.Equal(t, 1, server.CaptureCount())
			body := server.LastCapture().BodyString()
			assert.JSONEq(t, r.body, body)
			assert.NotContains(t, body, `"correct_option_id"`)
			if r.after != nil {
				r.after(t)
			}
		})
	}
	t.Run("T-s-4 validation unchanged", testSendQuiz96ValidationUnchanged)
}

// T-s-4: SendQuiz validation is unchanged: a wrong index is a *tg.ValidationError on correct_option_id, no request.
// (A control at the red step; it runs inside TestSendQuiz96_CorrectOptionIDs, which has red rows.)
func testSendQuiz96ValidationUnchanged(t *testing.T) {
	for _, idx := range []int{-1, 3} {
		server := api96PollServer(t)
		client := testutil.NewTestClient(t, server.BaseURL())
		_, err := client.SendQuiz(context.Background(), int64(-100123), "Q?", []string{"A", "B", "C"}, idx)
		var ve *tg.ValidationError
		require.ErrorAs(t, err, &ve, "index %d", idx)
		assert.Equal(t, "correct_option_id", ve.Field)
		assert.Equal(t, 0, server.CaptureCount())
	}
}
