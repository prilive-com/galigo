package receiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/prilive-com/galigo/tg"
)

// WebhookInfo contains information about the current webhook.
type WebhookInfo struct {
	URL                          string   `json:"url"`
	HasCustomCertificate         bool     `json:"has_custom_certificate"`
	PendingUpdateCount           int      `json:"pending_update_count"`
	IPAddress                    string   `json:"ip_address,omitempty"`
	LastErrorDate                int64    `json:"last_error_date,omitempty"`
	LastErrorMessage             string   `json:"last_error_message,omitempty"`
	LastSynchronizationErrorDate int64    `json:"last_synchronization_error_date,omitempty"`
	MaxConnections               int      `json:"max_connections,omitempty"`
	AllowedUpdates               []string `json:"allowed_updates,omitempty"`
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

// SetWebhook registers a webhook URL with Telegram.
func SetWebhook(ctx context.Context, client *http.Client, token tg.SecretToken, url, secret string) error {
	if client == nil {
		client = http.DefaultClient
	}

	payload := map[string]interface{}{
		"url": url,
	}
	if secret != "" {
		payload["secret_token"] = secret
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s%s/setWebhook", telegramAPIBaseURL, token.Value())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.OK {
		return &APIError{
			Code:        result.ErrorCode,
			Description: result.Description,
		}
	}

	return nil
}

// DeleteWebhook removes the webhook from Telegram.
func DeleteWebhook(ctx context.Context, client *http.Client, token tg.SecretToken, dropPending bool) error {
	if client == nil {
		client = http.DefaultClient
	}

	apiURL := fmt.Sprintf("%s%s/deleteWebhook?drop_pending_updates=%t",
		telegramAPIBaseURL, token.Value(), dropPending)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.OK {
		return &APIError{
			Code:        result.ErrorCode,
			Description: result.Description,
		}
	}

	return nil
}

// GetWebhookInfo retrieves the current webhook configuration.
func GetWebhookInfo(ctx context.Context, client *http.Client, token tg.SecretToken) (*WebhookInfo, error) {
	if client == nil {
		client = http.DefaultClient
	}

	apiURL := fmt.Sprintf("%s%s/getWebhookInfo", telegramAPIBaseURL, token.Value())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.OK {
		return nil, &APIError{
			Code:        result.ErrorCode,
			Description: result.Description,
		}
	}

	var info WebhookInfo
	if err := json.Unmarshal(result.Result, &info); err != nil {
		return nil, fmt.Errorf("failed to parse webhook info: %w", err)
	}

	return &info, nil
}
