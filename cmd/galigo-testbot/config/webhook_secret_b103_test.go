package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
)

// TestB103_TestbotSecretUnchanged: the testbot's Load still keeps any TESTBOT_WEBHOOK_SECRET_TOKEN unchanged (B-103
// checks the library's LoadConfig, New and NewWebhookHandler, not the testbot). A NUL byte cannot be set in the
// environment, so those rows are skipped. The value is compared with what the environment returns: on Windows an
// invalid UTF-8 byte comes back as U+FFFD (syscall's WTF-16 conversion).
func TestB103_TestbotSecretUnchanged(t *testing.T) {
	rows := append(testutil.WebhookSecretMatrix(), testutil.WebhookSecretCase{Name: "empty", Secret: "", OK: true})
	for _, c := range rows {
		t.Run(c.Name, func(t *testing.T) {
			if strings.IndexByte(c.Secret, 0) >= 0 {
				t.Skip("a NUL byte cannot be set in the environment")
			}
			t.Setenv("TESTBOT_TOKEN", "123456789:B103-FAKE-token-config-AAAAAAAAAAA")
			t.Setenv("TESTBOT_CHAT_ID", "-1001")
			t.Setenv("TESTBOT_ADMINS", "1")
			t.Setenv("TESTBOT_WEBHOOK_SECRET_TOKEN", c.Secret)
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, os.Getenv("TESTBOT_WEBHOOK_SECRET_TOKEN"), cfg.WebhookSecretToken.Value())
		})
	}
}
