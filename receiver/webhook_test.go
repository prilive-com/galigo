package receiver_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() receiver.Config {
	cfg := receiver.DefaultConfig()
	cfg.WebhookSecret = "test-secret-token"
	cfg.RateLimitRequests = 1000 // High limit for tests
	cfg.RateLimitBurst = 100
	cfg.MaxBodySize = 1 << 20 // 1MB
	return cfg
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

// ==================== Method Validation ====================

func TestWebhook_NonPOST_Returns405(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/webhook", nil)
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

func TestWebhook_POST_Accepted(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ==================== Secret Token Validation ====================

func TestWebhook_MissingSecretToken_Rejects(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No secret token header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWebhook_WrongSecretToken_Rejects(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWebhook_CorrectSecretToken_Accepts(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	update := tg.Update{UpdateID: 456}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify update was forwarded
	select {
	case received := <-updates:
		assert.Equal(t, 456, received.UpdateID)
	default:
		t.Fatal("expected update to be forwarded to channel")
	}
}

func TestWebhook_NoSecretConfigured_AcceptsAll(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := testConfig()
	cfg.WebhookSecret = "" // No secret configured
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	update := tg.Update{UpdateID: 789}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No secret token header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ==================== JSON Validation ====================

func TestWebhook_InvalidJSON_Returns400(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("not valid json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWebhook_EmptyBody_Returns400(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== Update Forwarding ====================

func TestWebhook_ValidUpdate_ForwardsToChannel(t *testing.T) {
	updates := make(chan tg.Update, 10)
	handler := receiver.NewWebhookHandler(testLogger(), updates, testConfig())

	update := tg.Update{
		UpdateID: 100,
		Message: &tg.Message{
			MessageID: 1,
			Text:      "Hello",
			Chat: &tg.Chat{
				ID:   123456,
				Type: "private",
			},
		},
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	received := <-updates
	assert.Equal(t, 100, received.UpdateID)
	require.NotNil(t, received.Message)
	assert.Equal(t, "Hello", received.Message.Text)
}

func TestWebhook_ChannelFull_DropNewest_Returns200(t *testing.T) {
	updates := make(chan tg.Update) // Unbuffered channel - will block
	cfg := testConfig()
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyDropNewest
	var droppedID int
	cfg.OnUpdateDropped = func(id int, reason string) {
		droppedID = id
	}
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	update := tg.Update{UpdateID: 200}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Webhook should always return 200 OK to Telegram, even when dropping
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 200, droppedID)
}

func TestWebhook_ChannelFull_BlockWithTimeout_Returns200(t *testing.T) {
	updates := make(chan tg.Update) // Unbuffered channel - will block
	cfg := testConfig()
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyBlock
	cfg.UpdateDeliveryTimeout = 10 * time.Millisecond
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	update := tg.Update{UpdateID: 201}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should return 200 even on timeout to prevent Telegram retry storms
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ==================== Body Size Limit ====================

func TestWebhook_OversizedBody_Returns413(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := testConfig()
	cfg.MaxBodySize = 100 // Very small limit
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	// Create a body larger than the limit
	largeBody := make([]byte, 200)
	for i := range largeBody {
		largeBody[i] = 'a'
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

// ==================== Domain Validation ====================

func TestWebhook_WrongDomain_Returns403(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := testConfig()
	cfg.AllowedDomain = "allowed.example.com"
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Host = "wrong.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestWebhook_CorrectDomain_Accepts(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := testConfig()
	cfg.AllowedDomain = "allowed.example.com"
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Host = "allowed.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWebhook_CorrectDomainWithPort_Accepts(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := testConfig()
	cfg.AllowedDomain = "allowed.example.com"
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Host = "allowed.example.com:8443"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ==================== Rate Limiting ====================

func TestWebhook_RateLimitExceeded_Returns429(t *testing.T) {
	updates := make(chan tg.Update, 100)
	cfg := testConfig()
	cfg.RateLimitRequests = 1
	cfg.RateLimitBurst = 1
	handler := receiver.NewWebhookHandler(testLogger(), updates, cfg)

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	// First request should succeed
	req1 := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Second request should be rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

// ==================== Health Handlers ====================

func TestHealthHandler_LivenessHandler_AlwaysOK(t *testing.T) {
	health := receiver.NewHealthHandler()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	health.LivenessHandler()(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "OK", rec.Body.String())
}

func TestHealthHandler_ReadinessHandler_NotReady(t *testing.T) {
	health := receiver.NewHealthHandler()
	// Not ready by default

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.ReadinessHandler()(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "Not Ready", rec.Body.String())
}

func TestHealthHandler_ReadinessHandler_Ready(t *testing.T) {
	health := receiver.NewHealthHandler()
	health.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.ReadinessHandler()(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Ready", rec.Body.String())
}

func TestHealthHandler_SetReady_Toggle(t *testing.T) {
	health := receiver.NewHealthHandler()

	// Initially not ready
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	health.ReadinessHandler()(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	// Set ready
	health.SetReady(true)
	rec = httptest.NewRecorder()
	health.ReadinessHandler()(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Set not ready again
	health.SetReady(false)
	rec = httptest.NewRecorder()
	health.ReadinessHandler()(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// ==================== Options ====================

func TestWebhookOption_WithWebhookRateLimit(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := testConfig()

	// Use option to set custom rate limit
	handler := receiver.NewWebhookHandler(
		testLogger(),
		updates,
		cfg,
		receiver.WithWebhookRateLimit(1, 1),
	)

	update := tg.Update{UpdateID: 123}
	body, _ := json.Marshal(update)

	// First request should succeed
	req1 := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Second should be rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestWebhookOption_WithWebhookMaxBodySize(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := testConfig()

	// Use option to set small body size
	handler := receiver.NewWebhookHandler(
		testLogger(),
		updates,
		cfg,
		receiver.WithWebhookMaxBodySize(50),
	)

	// Create body larger than 50 bytes
	largeBody := []byte(`{"update_id":123,"message":{"message_id":1,"text":"This is a long message that exceeds the body size limit"}}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
