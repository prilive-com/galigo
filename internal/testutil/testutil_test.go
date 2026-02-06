package testutil_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
)

func TestMockServer_CapturesRequests(t *testing.T) {
	server := testutil.NewMockServer(t)

	// Make a request
	resp, err := http.Post(server.BaseURL()+"/test", "application/json", nil)
	require.NoError(t, err)
	resp.Body.Close()

	// Verify capture
	assert.Equal(t, 1, server.CaptureCount())

	cap := server.LastCapture()
	require.NotNil(t, cap)
	assert.Equal(t, "POST", cap.Method)
	assert.Equal(t, "/test", cap.Path)
}

func TestMockServer_CustomHandler(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 42)
	})

	resp, err := http.Post(server.BaseURL()+"/bot"+testutil.TestToken+"/sendMessage", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	var envelope testutil.TelegramEnvelope
	err = json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)

	assert.True(t, envelope.OK)
	result, ok := envelope.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(42), result["message_id"])
}

func TestMockServer_DefaultSuccess(t *testing.T) {
	server := testutil.NewMockServer(t)

	// No handler registered - should return default success
	resp, err := http.Get(server.BaseURL() + "/unknown")
	require.NoError(t, err)
	defer resp.Body.Close()

	var envelope testutil.TelegramEnvelope
	err = json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)

	assert.True(t, envelope.OK)
}

func TestMockServer_Reset(t *testing.T) {
	server := testutil.NewMockServer(t)

	// Make some requests
	resp1, _ := http.Get(server.BaseURL() + "/test1")
	if resp1 != nil {
		resp1.Body.Close()
	}
	resp2, _ := http.Get(server.BaseURL() + "/test2")
	if resp2 != nil {
		resp2.Body.Close()
	}

	assert.Equal(t, 2, server.CaptureCount())

	server.Reset()

	assert.Equal(t, 0, server.CaptureCount())
}

func TestMockServer_TimeBetweenCaptures(t *testing.T) {
	server := testutil.NewMockServer(t)

	resp1, _ := http.Get(server.BaseURL() + "/test1")
	if resp1 != nil {
		resp1.Body.Close()
	}
	time.Sleep(50 * time.Millisecond)
	resp2, _ := http.Get(server.BaseURL() + "/test2")
	if resp2 != nil {
		resp2.Body.Close()
	}

	duration := server.TimeBetweenCaptures(0, 1)
	assert.GreaterOrEqual(t, duration, 50*time.Millisecond)
}

func TestReplyRateLimit(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/test", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyRateLimit(w, 5)
	})

	resp, err := http.Post(server.BaseURL()+"/test", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "5", resp.Header.Get("Retry-After"))

	var envelope testutil.TelegramEnvelope
	err = json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)

	assert.False(t, envelope.OK)
	assert.Equal(t, 429, envelope.ErrorCode)
	assert.Equal(t, 5, envelope.Parameters.RetryAfter)
}

func TestReplyServerError(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/test", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyServerError(w, 502, "Bad Gateway")
	})

	resp, err := http.Post(server.BaseURL()+"/test", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	var envelope testutil.TelegramEnvelope
	err = json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)

	assert.False(t, envelope.OK)
	assert.Equal(t, 502, envelope.ErrorCode)
	assert.Equal(t, "Bad Gateway", envelope.Description)
}

func TestFakeSleeper_RecordsCalls(t *testing.T) {
	sleeper := &testutil.FakeSleeper{}
	ctx := context.Background()

	err := sleeper.Sleep(ctx, 100*time.Millisecond)
	require.NoError(t, err)

	err = sleeper.Sleep(ctx, 200*time.Millisecond)
	require.NoError(t, err)

	assert.Equal(t, 2, sleeper.CallCount())
	assert.Equal(t, 100*time.Millisecond, sleeper.CallAt(0))
	assert.Equal(t, 200*time.Millisecond, sleeper.CallAt(1))
	assert.Equal(t, 200*time.Millisecond, sleeper.LastCall())
	assert.Equal(t, 300*time.Millisecond, sleeper.TotalDuration())
}

func TestFakeSleeper_RespectsContextCancel(t *testing.T) {
	sleeper := &testutil.FakeSleeper{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := sleeper.Sleep(ctx, time.Hour)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, sleeper.CallCount()) // Should not record cancelled sleep
}

func TestFakeSleeper_Reset(t *testing.T) {
	sleeper := &testutil.FakeSleeper{}
	ctx := context.Background()

	sleeper.Sleep(ctx, time.Second)
	sleeper.Sleep(ctx, time.Second)

	assert.Equal(t, 2, sleeper.CallCount())

	sleeper.Reset()

	assert.Equal(t, 0, sleeper.CallCount())
}

func TestRealSleeper_ActuallySleeps(t *testing.T) {
	sleeper := testutil.RealSleeper{}
	ctx := context.Background()

	start := time.Now()
	err := sleeper.Sleep(ctx, 50*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
}

func TestRealSleeper_RespectsContextCancel(t *testing.T) {
	sleeper := testutil.RealSleeper{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := sleeper.Sleep(ctx, time.Hour)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, elapsed, 100*time.Millisecond) // Should exit quickly
}

func TestCapture_Assertions(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 123)
	})

	resp, err := http.Post(
		server.BaseURL()+"/bot"+testutil.TestToken+"/sendMessage",
		"application/json",
		nil,
	)
	require.NoError(t, err)
	resp.Body.Close()

	// Test with body
	server.ResetCaptures()

	req, _ := http.NewRequest("POST", server.BaseURL()+"/bot"+testutil.TestToken+"/sendMessage", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = http.DefaultClient.Do(req)
	if resp != nil {
		resp.Body.Close()
	}

	cap := server.LastCapture()
	require.NotNil(t, cap)

	cap.AssertMethod(t, "POST")
	cap.AssertPath(t, "/bot"+testutil.TestToken+"/sendMessage")
	cap.AssertContentType(t, "application/json")

	// Test body assertions with actual body
	server.ResetCaptures()

	resp, _ = http.Post(
		server.BaseURL()+"/test",
		"application/json",
		jsonReader(map[string]any{"chat_id": float64(123), "text": "Hello"}),
	)
	resp.Body.Close()

	cap = server.LastCapture()
	cap.AssertJSONField(t, "chat_id", float64(123))
	cap.AssertJSONField(t, "text", "Hello")
	cap.AssertJSONFieldExists(t, "chat_id")
	cap.AssertJSONFieldAbsent(t, "parse_mode")
}

func TestFixtures(t *testing.T) {
	user := testutil.TestUser()
	assert.Equal(t, testutil.TestUserID, user.ID)
	assert.False(t, user.IsBot)

	bot := testutil.TestBot()
	assert.Equal(t, testutil.TestBotID, bot.ID)
	assert.True(t, bot.IsBot)

	chat := testutil.TestChat()
	assert.Equal(t, testutil.TestChatID, chat.ID)
	assert.Equal(t, "private", chat.Type)

	msg := testutil.TestMessage(42, "Hello World")
	assert.Equal(t, 42, msg.MessageID)
	assert.Equal(t, "Hello World", msg.Text)

	update := testutil.TestUpdate(100, "Test")
	assert.Equal(t, 100, update.UpdateID)
	assert.NotNil(t, update.Message)

	cb := testutil.TestCallbackQuery("cb_123", "button_data")
	assert.Equal(t, "cb_123", cb.ID)
	assert.Equal(t, "button_data", cb.Data)
}

// Helper to create JSON reader
func jsonReader(v any) *bytes.Buffer {
	data, _ := json.Marshal(v)
	return bytes.NewBuffer(data)
}
