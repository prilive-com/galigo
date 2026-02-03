package scrub_test

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/prilive-com/galigo/internal/scrub"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenFromError_NilError(t *testing.T) {
	result := scrub.TokenFromError(nil, tg.SecretToken("123:ABC"))
	assert.Nil(t, result)
}

func TestTokenFromError_EmptyToken(t *testing.T) {
	original := errors.New("some error")
	result := scrub.TokenFromError(original, tg.SecretToken(""))
	assert.Equal(t, original, result)
}

func TestTokenFromError_NoTokenInMessage(t *testing.T) {
	original := errors.New("connection refused")
	result := scrub.TokenFromError(original, tg.SecretToken("123:ABC"))
	assert.Equal(t, original, result)
}

func TestTokenFromError_ScrubsToken(t *testing.T) {
	token := tg.SecretToken("123456:ABCdef")
	original := fmt.Errorf("Post https://api.telegram.org/bot123456:ABCdef/sendMessage: dial tcp: no such host")
	result := scrub.TokenFromError(original, token)

	require.NotEqual(t, original, result)
	assert.Contains(t, result.Error(), "[REDACTED]")
	assert.NotContains(t, result.Error(), "123456:ABCdef")
}

func TestTokenFromError_PreservesErrorChain(t *testing.T) {
	token := tg.SecretToken("123456:ABCdef")
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	wrapped := fmt.Errorf("Post https://api.telegram.org/bot123456:ABCdef/sendMessage: %w", netErr)

	result := scrub.TokenFromError(wrapped, token)

	// Original error chain is preserved via Unwrap
	var opErr *net.OpError
	assert.True(t, errors.As(result, &opErr))
}
