# Security Policy

## Supported Versions

We actively support the following versions of galigo with security updates:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### How to Report

**DO NOT** open a public GitHub issue for security vulnerabilities.

Instead, please report security issues via one of these methods:

1. **GitHub Security Advisories** (Preferred)
   - Go to the [Security tab](https://github.com/prilive-com/galigo/security)
   - Click "Report a vulnerability"
   - Provide details about the vulnerability

2. **Email**
   - Send an email to: security@prilive.com (update with your actual email)
   - Use the subject line: `[SECURITY] galigo vulnerability report`

### What to Include

Please include the following information in your report:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)
- Your contact information for follow-up

### What to Expect

- **Acknowledgment**: We will acknowledge receipt within 48 hours
- **Initial Assessment**: We will provide an initial assessment within 7 days
- **Resolution Timeline**: We aim to resolve critical issues within 30 days
- **Credit**: We will credit reporters in our release notes (unless you prefer anonymity)

### Security Best Practices for Users

When using galigo:

1. **Protect Your Bot Token**
   - Never commit tokens to version control
   - Use environment variables or secret management
   - Rotate tokens if you suspect they've been compromised

2. **Use Webhook Secrets**
   - Always configure `X-Telegram-Bot-Api-Secret-Token` for webhooks
   - Telegram accepts only 1-256 characters, only `A-Z`, `a-z`, `0-9`, `_` and `-`. `galigo.New` and `receiver.LoadConfig` return a validation error for a non-empty secret outside this rule; `receiver.NewWebhookHandler` logs a warning and keeps it
   - galigo uses constant-time comparison to prevent timing attacks

3. **Enable TLS**
   - galigo enforces TLS 1.2+ for all connections
   - Ensure your webhook endpoints use HTTPS

4. **Keep Updated**
   - Regularly update to the latest version
   - Monitor our security advisories

## Security Features

galigo includes several security features:

- **Secret Redaction**: fmt and slog print `[REDACTED]` for the bot token and the webhook secret (`tg.SecretToken`, also the type of `receiver.Config.WebhookSecret`). A `*sender.Client`, `*galigo.Bot`, `*receiver.PollingClient` or `*receiver.WebhookHandler` holds the bot token or the webhook secret in unexported fields; fmt and slog's text handler print its type name and `{[REDACTED]}` (slog's JSON handler prints `{}`, or `null` for a nil pointer). A `*sender.Config`, `*receiver.Config` or `*tg.Config` prints its fields with the secrets redacted, also inside your own structs, slices and maps. Errors from `http.NewRequestWithContext` and `http.Client.Do` pass through a token scrubber; other errors, such as an error from your own JSON marshaler or from reading the response body, do not. Some paths still print a secret: `%w`, a copied client struct value, `{{.}}` in `text/template` and `html/template`, a holder pointer in an unexported field of your own type printed with a verb such as `%s` or `%q`, a config in an unexported field of your own type (a value, or a pointer printed with a verb such as `%s` or `%q`), a config value in a named field of your own type printed through a pointer below the top level with a verb such as `%s` or `%q` (for example `%s` of a `[]*T`), and others listed in the `tg.SecretToken` doc. A type of your own that embeds a holder pointer prints only the holder's text (`<nil>` when that pointer is nil): its own fields, `String`, `Error` and `GoString` are not used. This does not happen when the type has its own `Format` or another `Format` at the same depth (details in the `tg.SecretToken` doc). A type of your own that embeds a config pointer, or embeds a config value and is used through a pointer, prints only the config's fields: its own `String`, `Error` and `GoString` are not used. This does not happen when the type has its own `Format` or another `Format` at the same depth (details in the `tg.SecretToken` doc). fmt and slog's text handler print a non-empty `ProviderToken` of a `sender.SendInvoiceRequest`, `sender.CreateInvoiceLinkRequest` or `tg.InputInvoiceMessageContent` as `[REDACTED]`, value or pointer, also below the top level; the field stays a `string`, and `encoding/json`, slog's JSON handler and `encoding/xml` print it, because they write the request sent to Telegram. A type of your own that embeds one of these requests prints only the request's fields. Limits and other paths that print the token are in the `tg.SecretToken` doc
- **TLS Enforcement**: Minimum TLS 1.2 for all HTTPS connections
- **Response Size Limits**: Protection against memory exhaustion attacks
- **Constant-Time Comparison**: Webhook secret validation uses `crypto/subtle`
- **Input Validation**: Comprehensive validation of API inputs

## Vulnerability Disclosure Policy

We follow responsible disclosure:

1. Reporter notifies us of the vulnerability
2. We confirm and assess the issue
3. We develop and test a fix
4. We release the fix and publish an advisory
5. After 90 days (or upon fix release), details may be made public

Thank you for helping keep galigo and its users safe!
