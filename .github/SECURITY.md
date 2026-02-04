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
   - galigo uses constant-time comparison to prevent timing attacks

3. **Enable TLS**
   - galigo enforces TLS 1.2+ for all connections
   - Ensure your webhook endpoints use HTTPS

4. **Keep Updated**
   - Regularly update to the latest version
   - Monitor our security advisories

## Security Features

galigo includes several security features:

- **Secret Redaction**: Bot tokens are automatically redacted from logs and errors
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
