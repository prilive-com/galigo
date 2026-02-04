package sender_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== AnswerInlineQuery ====================

func TestAnswerInlineQuery(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerInlineQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.AnswerInlineQuery(context.Background(), sender.AnswerInlineQueryRequest{
		InlineQueryID: "query_123",
		Results: []tg.InlineQueryResult{
			tg.InlineQueryResultArticle{
				ID:                  "1",
				Title:               "Test",
				InputMessageContent: tg.InputTextMessageContent{MessageText: "hi"},
			},
		},
	})
	require.NoError(t, err)
}

func TestAnswerInlineQuery_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.AnswerInlineQuery(context.Background(), sender.AnswerInlineQueryRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline_query_id")
}

// ==================== AnswerWebAppQuery ====================

func TestAnswerWebAppQuery(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerWebAppQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{"inline_message_id": "inline_456"})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	result, err := client.AnswerWebAppQuery(context.Background(), sender.AnswerWebAppQueryRequest{
		WebAppQueryID: "webapp_123",
		Result:        tg.InlineQueryResultArticle{ID: "1", Title: "Test", InputMessageContent: tg.InputTextMessageContent{MessageText: "hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "inline_456", result.InlineMessageID)
}

func TestAnswerWebAppQuery_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.AnswerWebAppQueryRequest
		want string
	}{
		{"missing query_id", sender.AnswerWebAppQueryRequest{Result: tg.InlineQueryResultArticle{}}, "web_app_query_id"},
		{"missing result", sender.AnswerWebAppQueryRequest{WebAppQueryID: "q"}, "result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.AnswerWebAppQuery(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== SendChecklist ====================

func TestSendChecklist(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendChecklist", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": int64(123), "type": "private"},
			"date":       1700000000,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SendChecklist(context.Background(), sender.SendChecklistRequest{
		ChatID: int64(123),
		Checklist: tg.InputChecklist{
			Title: "My List",
			Tasks: []tg.InputChecklistTask{{Text: "Task 1"}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, msg.MessageID)
}

func TestSendChecklist_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SendChecklistRequest
		want string
	}{
		{"missing chat_id", sender.SendChecklistRequest{Checklist: tg.InputChecklist{Title: "T", Tasks: []tg.InputChecklistTask{{Text: "t"}}}}, "chat_id"},
		{"missing title", sender.SendChecklistRequest{ChatID: int64(1), Checklist: tg.InputChecklist{Tasks: []tg.InputChecklistTask{{Text: "t"}}}}, "title"},
		{"missing tasks", sender.SendChecklistRequest{ChatID: int64(1), Checklist: tg.InputChecklist{Title: "T"}}, "tasks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.SendChecklist(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== EditMessageChecklist ====================

func TestEditMessageChecklist(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageChecklist", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": int64(123), "type": "private"},
			"date":       1700000000,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.EditMessageChecklist(context.Background(), sender.EditMessageChecklistRequest{
		ChatID:    int64(123),
		MessageID: 1,
		Checklist: tg.InputChecklist{
			Title: "Updated List",
			Tasks: []tg.InputChecklistTask{{Text: "Task 1"}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, msg.MessageID)
}

func TestEditMessageChecklist_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.EditMessageChecklistRequest
		want string
	}{
		{"missing chat_id", sender.EditMessageChecklistRequest{MessageID: 1}, "chat_id"},
		{"missing message_id", sender.EditMessageChecklistRequest{ChatID: int64(1)}, "message_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.EditMessageChecklist(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
