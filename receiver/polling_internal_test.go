package receiver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

const (
	scrubSecretPre  = "ABCdefGHI"
	scrubSecretPost = "jklMNOpqr"
	scrubToken      = "123456789:" + scrubSecretPre + "STUVWXYZabcdefgh" + scrubSecretPost
)

var errScrubBoom = errors.New("boom")

type scrubCountingTransport struct{ calls atomic.Int32 }

func (c *scrubCountingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errScrubBoom
}

func requireScrubbedAPIError(t *testing.T, err error, description string) {
	t.Helper()
	require.Error(t, err)
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr), "want *APIError, got %T", err)
	assert.Equal(t, description, apiErr.Description)
	msg := err.Error()
	assert.NotContains(t, msg, scrubSecretPre)
	assert.NotContains(t, msg, scrubSecretPost)
	assert.Contains(t, msg, "[REDACTED]")
	var urlErr *url.Error
	assert.True(t, errors.As(err, &urlErr), "error chain must keep *url.Error")
}

func TestFetchUpdates_RequestCreationError_Scrubbed(t *testing.T) {
	for _, tc := range []struct{ name, token, baseURL string }{
		{"control byte in base", scrubToken, "https://api.telegram.org/bot\x7f"},
		{"token with trailing CR", scrubToken + "\r", "https://api.telegram.org/bot\x7f"},
		{"token with hash, no scheme", "123456789:" + scrubSecretPre + "#" + scrubSecretPost, "://bot"},
		{"base without path", scrubToken, "https://api.telegram.org"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.BaseURL = tc.baseURL
			c := NewPollingClient(tg.SecretToken(tc.token), make(chan tg.Update, 1), nil, cfg)
			_, err := c.fetchUpdates(context.Background())
			requireScrubbedAPIError(t, err, "failed to create request")
		})
	}
}

func TestFetchUpdates_DoError_Scrubbed(t *testing.T) {
	for _, tc := range []struct{ name, token string }{
		{"plain token", scrubToken},
		{"token with trailing space", scrubToken + " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &scrubCountingTransport{}
			c := NewPollingClient(tg.SecretToken(tc.token), make(chan tg.Update, 1), nil, DefaultConfig(),
				WithPollingHTTPClient(&http.Client{Transport: rt}))
			_, err := c.fetchUpdates(context.Background())
			requireScrubbedAPIError(t, err, "request failed")
			assert.ErrorIs(t, err, errScrubBoom)
			assert.Equal(t, int32(1), rt.calls.Load())
		})
	}
}
