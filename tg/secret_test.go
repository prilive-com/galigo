package tg_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/tg"
)

func TestSecretToken_Value(t *testing.T) {
	token := tg.SecretToken("123456:ABC-DEF")
	assert.Equal(t, "123456:ABC-DEF", token.Value())
}

func TestSecretToken_String(t *testing.T) {
	token := tg.SecretToken("123456:ABC-DEF")
	assert.Equal(t, "[REDACTED]", token.String())
}

func TestSecretToken_GoString(t *testing.T) {
	token := tg.SecretToken("123456:ABC-DEF")
	assert.Equal(t, `tg.SecretToken("[REDACTED]")`, token.GoString())
}

func TestSecretToken_LogValue(t *testing.T) {
	token := tg.SecretToken("123456:ABC-DEF")
	logValue := token.LogValue()
	assert.Equal(t, slog.KindString, logValue.Kind())
	assert.Equal(t, "[REDACTED]", logValue.String())
}

func TestSecretToken_MarshalText(t *testing.T) {
	token := tg.SecretToken("123456:ABC-DEF")
	text, err := token.MarshalText()
	assert.NoError(t, err)
	assert.Equal(t, []byte("[REDACTED]"), text)
}

func TestSecretToken_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		token   tg.SecretToken
		isEmpty bool
	}{
		{"empty token", tg.SecretToken(""), true},
		{"non-empty token", tg.SecretToken("123456:ABC"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isEmpty, tt.token.IsEmpty())
		})
	}
}

func TestSecretToken_NotLeakedInFmt(t *testing.T) {
	token := tg.SecretToken("123456:ABC-DEF-SECRET")

	// Test various fmt formats don't leak the token
	formats := []string{
		token.String(),
		fmt.Sprintf("%v", token),
		fmt.Sprintf("%#v", token),
		fmt.Sprintf("%+v", token),
	}

	for _, formatted := range formats {
		assert.NotContains(t, formatted, "123456")
		assert.NotContains(t, formatted, "SECRET")
		assert.Contains(t, formatted, "REDACTED")
	}
}

func TestSecretToken_NotLeakedInJSON(t *testing.T) {
	type container struct {
		Token tg.SecretToken `json:"token"`
	}

	c := container{Token: tg.SecretToken("123456:SECRET")}
	data, err := json.Marshal(c)
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "123456")
	assert.NotContains(t, string(data), "SECRET")
}

func TestSecretToken_NotLeakedInSlog(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	token := tg.SecretToken("123456:SECRET")
	logger.Info("test", "token", token)

	output := buf.String()
	assert.NotContains(t, output, "123456")
	assert.NotContains(t, output, "SECRET")
	assert.Contains(t, output, "REDACTED")
}
