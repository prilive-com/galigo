package testutil

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// MockTelegramServer provides a mock Telegram Bot API server for testing.
type MockTelegramServer struct {
	*httptest.Server
	t        *testing.T
	mu       sync.Mutex
	handlers map[string]http.HandlerFunc
	captures []Capture
}

// NewMockServer creates a mock Telegram API server.
// The server is automatically closed when the test completes.
func NewMockServer(t *testing.T) *MockTelegramServer {
	t.Helper()

	m := &MockTelegramServer{
		t:        t,
		handlers: make(map[string]http.HandlerFunc),
		captures: make([]Capture, 0),
	}

	m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.Server.Close)
	return m
}

func (m *MockTelegramServer) handle(w http.ResponseWriter, r *http.Request) {
	// Read body once for capture
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	// Restore body for downstream handler
	r.Body = io.NopCloser(bytes.NewReader(body))

	m.mu.Lock()
	m.captures = append(m.captures, Capture{
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.Query(),
		Headers:     r.Header.Clone(),
		Body:        body,
		ContentType: r.Header.Get("Content-Type"),
		Timestamp:   time.Now(),
	})

	// Find handler
	key := r.Method + ":" + r.URL.Path
	handler, exists := m.handlers[key]
	m.mu.Unlock()

	if exists {
		handler(w, r)
		return
	}

	// Default success response
	ReplyOK(w, map[string]any{})
}

// OnMethod registers a handler for a specific HTTP method and path.
//
// Example:
//
//	server.OnMethod("POST", "/bot123:ABC/sendMessage", func(w http.ResponseWriter, r *http.Request) {
//	    testutil.ReplyMessage(w, 123)
//	})
func (m *MockTelegramServer) OnMethod(method, path string, handler http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[method+":"+path] = handler
}

// On registers a handler for a POST request (most common case).
func (m *MockTelegramServer) On(path string, handler http.HandlerFunc) {
	m.OnMethod("POST", path, handler)
}

// Captures returns all captured requests.
func (m *MockTelegramServer) Captures() []Capture {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Capture{}, m.captures...)
}

// LastCapture returns the most recent captured request.
func (m *MockTelegramServer) LastCapture() *Capture {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.captures) == 0 {
		return nil
	}
	return &m.captures[len(m.captures)-1]
}

// CaptureAt returns the capture at the given index.
func (m *MockTelegramServer) CaptureAt(index int) *Capture {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.captures) {
		return nil
	}
	return &m.captures[index]
}

// CaptureCount returns the total number of captured requests.
func (m *MockTelegramServer) CaptureCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.captures)
}

// Reset clears all captures and handlers.
func (m *MockTelegramServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.captures = m.captures[:0]
	m.handlers = make(map[string]http.HandlerFunc)
}

// ResetCaptures clears only captures, keeping handlers.
func (m *MockTelegramServer) ResetCaptures() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.captures = m.captures[:0]
}

// TimeBetweenCaptures returns the duration between two captures.
// Useful for rate-limit testing.
func (m *MockTelegramServer) TimeBetweenCaptures(i, j int) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || j < 0 || i >= len(m.captures) || j >= len(m.captures) {
		return 0
	}
	return m.captures[j].Timestamp.Sub(m.captures[i].Timestamp)
}

// BaseURL returns the server's base URL.
// Use this as the API base URL when creating clients.
func (m *MockTelegramServer) BaseURL() string {
	return m.Server.URL
}

// BotURL returns the full bot API URL for a given token.
// Example: server.BotURL(testutil.TestToken) returns "http://127.0.0.1:port/bot123:ABC"
func (m *MockTelegramServer) BotURL(token string) string {
	return m.Server.URL + "/bot" + token
}
