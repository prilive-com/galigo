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
		assert.Equal(t, float64(1), req["correct_option_id"])
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
