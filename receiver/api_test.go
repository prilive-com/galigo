package receiver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SetWebhook ====================

func TestSetWebhook_Success(t *testing.T) {
	var capturedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.True(t, strings.HasSuffix(r.URL.Path, "/setWebhook"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		json.NewDecoder(r.Body).Decode(&capturedBody)

		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": true,
		})
	}))
	defer server.Close()

	// Create client that uses our test server
	client := server.Client()

	// We need to intercept the URL - use a custom transport
	originalTransport := client.Transport
	client.Transport = &urlRewriteTransport{
		base:      originalTransport,
		targetURL: server.URL,
	}

	err := receiver.SetWebhook(
		context.Background(),
		client,
		tg.SecretToken("test:token"),
		"https://example.com/webhook",
		"secret123",
	)

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/webhook", capturedBody["url"])
	assert.Equal(t, "secret123", capturedBody["secret_token"])
}

func TestSetWebhook_NoSecret(t *testing.T) {
	var capturedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": true,
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	err := receiver.SetWebhook(
		context.Background(),
		client,
		tg.SecretToken("test:token"),
		"https://example.com/webhook",
		"", // No secret
	)

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/webhook", capturedBody["url"])
	_, hasSecret := capturedBody["secret_token"]
	assert.False(t, hasSecret, "should not include secret_token when empty")
}

func TestSetWebhook_TelegramError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"error_code":  400,
			"description": "Bad Request: bad webhook",
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	err := receiver.SetWebhook(
		context.Background(),
		client,
		tg.SecretToken("test:token"),
		"invalid-url",
		"",
	)

	require.Error(t, err)
	var apiErr *receiver.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code)
	assert.Contains(t, apiErr.Description, "bad webhook")
}

func TestSetWebhook_NilClient_UsesDefault(t *testing.T) {
	// This test verifies nil client handling - the request will fail
	// because there's no actual Telegram API, but it shouldn't panic
	err := receiver.SetWebhook(
		context.Background(),
		nil, // nil client
		tg.SecretToken("test:token"),
		"https://example.com/webhook",
		"",
	)

	// Should return an error (connection refused or similar) but not panic
	assert.Error(t, err)
}

func TestSetWebhook_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		select {}
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := receiver.SetWebhook(ctx, client, tg.SecretToken("test:token"), "https://example.com", "")
	assert.Error(t, err)
}

// ==================== DeleteWebhook ====================

func TestDeleteWebhook_Success(t *testing.T) {
	var capturedDropPending string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.True(t, strings.HasSuffix(r.URL.Path, "/deleteWebhook"))

		capturedDropPending = r.URL.Query().Get("drop_pending_updates")

		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": true,
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	err := receiver.DeleteWebhook(
		context.Background(),
		client,
		tg.SecretToken("test:token"),
		true,
	)

	require.NoError(t, err)
	assert.Equal(t, "true", capturedDropPending)
}

func TestDeleteWebhook_NoDropPending(t *testing.T) {
	var capturedDropPending string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDropPending = r.URL.Query().Get("drop_pending_updates")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": true,
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	err := receiver.DeleteWebhook(
		context.Background(),
		client,
		tg.SecretToken("test:token"),
		false,
	)

	require.NoError(t, err)
	assert.Equal(t, "false", capturedDropPending)
}

func TestDeleteWebhook_TelegramError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"error_code":  401,
			"description": "Unauthorized",
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	err := receiver.DeleteWebhook(
		context.Background(),
		client,
		tg.SecretToken("bad:token"),
		false,
	)

	require.Error(t, err)
	var apiErr *receiver.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 401, apiErr.Code)
}

func TestDeleteWebhook_NilClient_UsesDefault(t *testing.T) {
	err := receiver.DeleteWebhook(
		context.Background(),
		nil,
		tg.SecretToken("test:token"),
		false,
	)
	assert.Error(t, err) // Will fail but shouldn't panic
}

// ==================== GetWebhookInfo ====================

func TestGetWebhookInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.True(t, strings.HasSuffix(r.URL.Path, "/getWebhookInfo"))

		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"url":                    "https://example.com/webhook",
				"has_custom_certificate": false,
				"pending_update_count":   5,
				"max_connections":        40,
				"allowed_updates":        []string{"message", "callback_query"},
			},
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	info, err := receiver.GetWebhookInfo(
		context.Background(),
		client,
		tg.SecretToken("test:token"),
	)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "https://example.com/webhook", info.URL)
	assert.False(t, info.HasCustomCertificate)
	assert.Equal(t, 5, info.PendingUpdateCount)
	assert.Equal(t, 40, info.MaxConnections)
	assert.Equal(t, []string{"message", "callback_query"}, info.AllowedUpdates)
}

func TestGetWebhookInfo_EmptyWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"url":                    "",
				"has_custom_certificate": false,
				"pending_update_count":   0,
			},
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	info, err := receiver.GetWebhookInfo(
		context.Background(),
		client,
		tg.SecretToken("test:token"),
	)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "", info.URL)
	assert.Equal(t, 0, info.PendingUpdateCount)
}

func TestGetWebhookInfo_TelegramError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"error_code":  401,
			"description": "Unauthorized",
		})
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &urlRewriteTransport{
		base:      client.Transport,
		targetURL: server.URL,
	}

	info, err := receiver.GetWebhookInfo(
		context.Background(),
		client,
		tg.SecretToken("bad:token"),
	)

	require.Error(t, err)
	assert.Nil(t, info)
	var apiErr *receiver.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 401, apiErr.Code)
}

func TestGetWebhookInfo_NilClient_UsesDefault(t *testing.T) {
	info, err := receiver.GetWebhookInfo(
		context.Background(),
		nil,
		tg.SecretToken("test:token"),
	)
	assert.Error(t, err)
	assert.Nil(t, info)
}

// urlRewriteTransport rewrites requests to target our test server
type urlRewriteTransport struct {
	base      http.RoundTripper
	targetURL string
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite URL to our test server while keeping the path
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.targetURL, "http://")

	if t.base != nil {
		return t.base.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
