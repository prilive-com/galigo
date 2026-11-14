package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

func TestB98_ConfigSecrets(t *testing.T) {
	const token = "123456789:B98-FAKE-token-config-AAAAAAAAAAAA"
	const secret = "B98-FAKE-webhook-secret-config"
	t.Setenv("TESTBOT_TOKEN", token)
	t.Setenv("TESTBOT_CHAT_ID", "-1001")
	t.Setenv("TESTBOT_ADMINS", "1")
	t.Setenv("TESTBOT_WEBHOOK_SECRET_TOKEN", secret)
	// Load reads only the environment and treats an empty value as unset, so the other settings take their
	// defaults whatever the machine's environment holds, and the whole output can be compared.
	for _, k := range []string{"TESTBOT_MODE", "TESTBOT_STORAGE_DIR", "TESTBOT_LOG_LEVEL", "TESTBOT_MAX_MESSAGES_PER_RUN",
		"TESTBOT_SEND_INTERVAL", "TESTBOT_JITTER_INTERVAL", "TESTBOT_RETRY_429", "TESTBOT_MAX_429_RETRIES",
		"TESTBOT_ALLOW_STRESS", "TESTBOT_LISTEN_ADDR", "TESTBOT_WEBHOOK_PUBLIC_URL"} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	require.NoError(t, err)
	t.Run("plus_v", func(t *testing.T) {
		assert.Equal(t, "&{Token:[REDACTED] ChatID:-1001 Admins:[1] Mode:polling StorageDir:./var LogLevel:info "+
			"MaxMessagesPerRun:40 SendInterval:350ms JitterInterval:150ms RetryOn429:true Max429Retries:2 "+
			"AllowStress:false WebhookPublicURL: WebhookSecretToken:[REDACTED] ListenAddr::8080 Timeout:30s}",
			fmt.Sprintf("%+v", cfg))
	})
	t.Run("token_type", func(t *testing.T) { assert.IsType(t, tg.SecretToken(""), cfg.Token) })
	t.Run("secret_type", func(t *testing.T) { assert.IsType(t, tg.SecretToken(""), cfg.WebhookSecretToken) })
	t.Run("token_value", func(t *testing.T) {
		assert.Equal(t, token, string(cfg.Token), "the loaded token is the real value")
	})
	t.Run("secret_value", func(t *testing.T) { assert.Equal(t, secret, string(cfg.WebhookSecretToken)) })
}
