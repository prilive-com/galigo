// Package testutil provides testing utilities for galigo.
//
// This package is intended for internal testing only and should not be imported
// by external packages.
//
// # Mock Telegram Server
//
// MockTelegramServer provides a mock Telegram Bot API server for testing:
//
//	server := testutil.NewMockServer(t)
//	server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
//	    testutil.ReplyMessage(w, 123)
//	})
//	// Use server.BaseURL() as the API base URL
//
// # Request Capture
//
// All requests are automatically captured and can be inspected:
//
//	cap := server.LastCapture()
//	cap.AssertMethod(t, "POST")
//	cap.AssertJSONField(t, "chat_id", float64(123))
//
// # Fake Sleeper
//
// FakeSleeper records sleep calls without actually sleeping:
//
//	sleeper := &testutil.FakeSleeper{}
//	// Pass to client via WithSleeper option
//	assert.Equal(t, 2*time.Second, sleeper.LastCall())
//
// # Test Fixtures
//
// Common test data is available:
//
//	testutil.TestToken    // Valid bot token format
//	testutil.TestChatID   // Test chat ID
//	testutil.TestUser()   // Test user fixture
//	testutil.TestMessage(1, "Hello") // Test message fixture
package testutil
