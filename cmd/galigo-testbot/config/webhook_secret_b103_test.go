package config

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
)

// TestB103_TestbotSecretUnchanged: the testbot's Load still keeps any TESTBOT_WEBHOOK_SECRET_TOKEN unchanged (B-103
// checks the library's LoadConfig, New and NewWebhookHandler, not the testbot). A NUL byte cannot be set in the
// environment, so those rows are skipped. The value is compared with what the environment returns, read before Load:
// on Windows an invalid UTF-8 byte comes back as U+FFFD (syscall's WTF-16 conversion), and a valid UTF-8 value must
// come back unchanged.
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
			want := os.Getenv("TESTBOT_WEBHOOK_SECRET_TOKEN") // read before Load, so Load cannot change what is compared
			if utf8.ValidString(c.Secret) {
				require.Equal(t, c.Secret, want, "environment round trip")
			}
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, want, cfg.WebhookSecretToken.Value())
		})
	}
}
