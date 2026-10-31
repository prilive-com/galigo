package engine

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
	adapterSecretPre  = "ABCdefGHI"
	adapterSecretPost = "jklMNOpqr"
)

var errAdapterBoom = errors.New("boom")

type adapterCountingTransport struct{ calls atomic.Int32 }

func (c *adapterCountingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errAdapterBoom
}

func requireAdapterScrubbed(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	assert.NotContains(t, msg, adapterSecretPre)
	assert.NotContains(t, msg, adapterSecretPost)
	assert.Contains(t, msg, "[REDACTED]")
	var urlErr *url.Error
	assert.True(t, errors.As(err, &urlErr), "error chain must keep *url.Error")
}

func TestGetUpdates_RequestCreationError_Scrubbed(t *testing.T) {
	for _, x := range []string{"%zz", "\x7f"} {
		t.Run(x, func(t *testing.T) {
			tok := tg.SecretToken("123456789:" + adapterSecretPre + x + adapterSecretPost)
			_, err := NewSenderAdapter(nil).WithToken(tok).GetUpdates(context.Background(), 0, 1, 0)
			requireAdapterScrubbed(t, err)
		})
	}
}

func TestGetUpdates_DoError_Scrubbed(t *testing.T) {
	for _, x := range []string{"", " "} {
		t.Run("suffix "+x, func(t *testing.T) {
			tok := tg.SecretToken("123456789:" + adapterSecretPre + "STUVWXYZabcdefgh" + adapterSecretPost + x)
			rt := &adapterCountingTransport{}
			_, err := NewSenderAdapter(nil).WithToken(tok).WithHTTPClient(&http.Client{Transport: rt}).
				GetUpdates(context.Background(), 0, 1, 0)
			requireAdapterScrubbed(t, err)
			assert.ErrorIs(t, err, errAdapterBoom)
			assert.Equal(t, int32(1), rt.calls.Load())
		})
	}
}
