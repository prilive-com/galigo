package testutil

import (
	"net/http"
	"strings"
)

// WebhookSecretValue is a named fake webhook secret (B-99).
type WebhookSecretValue struct{ Name, Secret string }

// WebhookSecretValues returns secrets that every copy of the webhook secret must keep unchanged: normal, one
// character, 256 characters (Telegram's maximum, so a copy that cuts the value fails), hyphens at both ends, and
// three values outside Telegram's alphabet that galigo still stores as they are (it does not check the value):
// spaces at both ends, only a space, and non-ASCII text. An empty secret is not in the list: it turns the check off,
// and the auth rows cover it.
func WebhookSecretValues(normal string) []WebhookSecretValue {
	return []WebhookSecretValue{
		{"normal", normal},
		{"one_char", "A"},
		{"max_256", strings.Repeat("Az9_", 64)},
		{"edge_hyphen", "-Az9_-"},
		{"spaces_kept", " B99 FAKE spaced secret "},
		{"blank_kept", " "},
		{"non_ascii", "B99-é-秘密"},
	}
}

// WebhookAuthCase is one request to a webhook handler: the X-Telegram-Bot-Api-Secret-Token header (nil: no header)
// and the status the handler must answer.
type WebhookAuthCase struct {
	Name   string
	Header *string
	Want   int
}

// WebhookAuthCases returns the requests for a handler configured with secret (B-99). With a secret, only the secret
// itself is accepted (200); longer, a shorter prefix, the same length with the first, the middle (byte len/2) or the
// last byte changed, the other letter case, the text [REDACTED] and no header are rejected (401). The other-case and
// [REDACTED] cases are left out when they equal the secret. With no secret, every request is accepted.
func WebhookAuthCases(secret string) []WebhookAuthCase {
	h := func(s string) *string { return &s }
	if secret == "" {
		return []WebhookAuthCase{
			{"no_header", nil, http.StatusOK},
			{"any", h("any"), http.StatusOK},
			{"redacted_text", h("[REDACTED]"), http.StatusOK},
		}
	}
	other := func(b byte) string {
		if b == 'x' {
			return "y"
		}
		return "x"
	}
	cases := []WebhookAuthCase{
		{"correct", h(secret), http.StatusOK},
		{"longer", h(secret + "x"), http.StatusUnauthorized},
		{"shorter_prefix", h(secret[:len(secret)-1]), http.StatusUnauthorized},
		{"same_length_first_byte", h(other(secret[0]) + secret[1:]), http.StatusUnauthorized},
		{"same_length_middle_byte", h(secret[:len(secret)/2] + other(secret[len(secret)/2]) + secret[len(secret)/2+1:]), http.StatusUnauthorized},
		{"same_length_last_byte", h(secret[:len(secret)-1] + other(secret[len(secret)-1])), http.StatusUnauthorized},
		{"no_header", nil, http.StatusUnauthorized},
	}
	if secret != "[REDACTED]" {
		cases = append(cases, WebhookAuthCase{"redacted_text", h("[REDACTED]"), http.StatusUnauthorized})
	}
	if swapped := swapCase(secret); swapped != secret {
		cases = append(cases, WebhookAuthCase{"other_case", h(swapped), http.StatusUnauthorized})
	}
	return cases
}

// swapCase swaps the case of the ASCII letters in s.
func swapCase(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case 'a' <= r && r <= 'z':
			return r - 'a' + 'A'
		case 'A' <= r && r <= 'Z':
			return r - 'A' + 'a'
		}
		return r
	}, s)
}
