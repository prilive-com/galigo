package sender_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SendGame ====================

func TestSendGame(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendGame", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": int64(123), "type": "private"},
			"date":       1700000000,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SendGame(context.Background(), sender.SendGameRequest{
		ChatID:        123,
		GameShortName: "testgame",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, msg.MessageID)
}

func TestSendGame_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SendGameRequest
		want string
	}{
		{"missing chat_id", sender.SendGameRequest{GameShortName: "g"}, "chat_id"},
		{"missing game_short_name", sender.SendGameRequest{ChatID: 1}, "game_short_name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.SendGame(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== SetGameScore ====================

func TestSetGameScore_ChatMessage(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setGameScore", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": int64(123), "type": "private"},
			"date":       1700000000,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SetGameScore(context.Background(), sender.SetGameScoreRequest{
		UserID:    456,
		Score:     100,
		ChatID:    123,
		MessageID: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, msg.MessageID)
}

func TestSetGameScore_Inline(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setGameScore", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SetGameScore(context.Background(), sender.SetGameScoreRequest{
		UserID:          456,
		Score:           100,
		InlineMessageID: "inline_123",
	})
	require.NoError(t, err)
	assert.Nil(t, msg)
}

func TestSetGameScore_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SetGameScoreRequest
		want string
	}{
		{"missing user_id", sender.SetGameScoreRequest{Score: 1, ChatID: 1}, "user_id"},
		{"negative score", sender.SetGameScoreRequest{UserID: 1, Score: -1, ChatID: 1}, "score"},
		{"missing target", sender.SetGameScoreRequest{UserID: 1, Score: 1}, "chat_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.SetGameScore(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== GetGameHighScores ====================

func TestGetGameHighScores(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getGameHighScores", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]any{
			{"position": 1, "user": map[string]any{"id": int64(456), "is_bot": false, "first_name": "Alice"}, "score": 100},
			{"position": 2, "user": map[string]any{"id": int64(789), "is_bot": false, "first_name": "Bob"}, "score": 50},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	scores, err := client.GetGameHighScores(context.Background(), sender.GetGameHighScoresRequest{
		UserID:    456,
		ChatID:    123,
		MessageID: 1,
	})
	require.NoError(t, err)
	require.Len(t, scores, 2)
	assert.Equal(t, 1, scores[0].Position)
	assert.Equal(t, 100, scores[0].Score)
	assert.Equal(t, "Alice", scores[0].User.FirstName)
}

func TestGetGameHighScores_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.GetGameHighScoresRequest
		want string
	}{
		{"missing user_id", sender.GetGameHighScoresRequest{ChatID: 1}, "user_id"},
		{"missing target", sender.GetGameHighScoresRequest{UserID: 1}, "chat_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.GetGameHighScores(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
