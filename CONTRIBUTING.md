# Contributing to galigo

Thank you for considering contributing to galigo! This guide will help you get started.

## 🛡️ Security First

**CRITICAL:** Never commit real API tokens, keys, or secrets.

- Use mock helpers in `internal/testutil` for test cases
- If you find a security vulnerability, **do NOT open a public issue**
- Use GitHub's [Private Vulnerability Reporting](https://github.com/prilive-com/galigo/security) instead
- See [SECURITY.md](.github/SECURITY.md) for our full security policy

## Getting Started

### Prerequisites

- Go 1.25 or later
- golangci-lint (for linting)

### Setup

```bash
# Fork and clone your fork
git clone https://github.com/YOUR_USERNAME/galigo.git
cd galigo

# Add upstream remote
git remote add upstream https://github.com/prilive-com/galigo.git

# Install dependencies
go mod download
```

## Development Workflow

### Running Tests

```bash
# Unit tests
go test ./...

# With race detector (required before PR)
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Running Linters

All PRs must pass linting:

```bash
golangci-lint run
```

### Integration Testing

For changes to `sender/` or `receiver/`, verify against the real Telegram API:

```bash
export TESTBOT_TOKEN="your-test-bot-token"
export TESTBOT_CHAT_ID="your-chat-id"
export TESTBOT_ADMINS="your-user-id"

go run ./cmd/galigo-testbot --run all
```

See [docs/testing.md](docs/testing.md) for detailed instructions.

## Pull Request Guidelines

### Before Submitting

1. Ensure all tests pass: `go test -race ./...`
2. Ensure linting passes: `golangci-lint run`
3. Update documentation if needed
4. Squash or rebase commits for clean history

### Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add support for Telegram Stars payments
fix: resolve panic in long polling on context cancel
docs: update SendPhoto example in README
test: add coverage for circuit breaker edge cases
refactor: extract rate limiter to separate file
chore: update golangci-lint to v2.8
```

### PR Description

Include:
- What the PR does
- Why it's needed
- How to test it
- Related issues (e.g., "Fixes #42")

## Coding Style

### Go Conventions

- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for formatting
- Keep functions focused and reasonably sized

### Error Handling

```go
// Always wrap errors with context
if err != nil {
    return fmt.Errorf("failed to send message: %w", err)
}
```

### Context

All blocking operations must accept `context.Context` as the first parameter:

```go
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*tg.Message, error)
```

### Documentation

- Add GoDoc comments for all exported types and functions
- Include usage examples where helpful

## Project Structure

```
galigo/
├── bot.go              # Unified Bot type
├── tg/                 # Telegram types and errors
├── sender/             # Message sending client
├── receiver/           # Update receiving (polling/webhook)
├── internal/           # Internal packages (not for external use)
├── cmd/galigo-testbot/ # Integration test bot
├── examples/           # Usage examples
└── docs/               # Documentation
```

## What to Contribute

### Good First Issues

Look for issues labeled [`good first issue`](https://github.com/prilive-com/galigo/labels/good%20first%20issue).

### Feature Requests

Before implementing a feature:
1. Check existing [issues](https://github.com/prilive-com/galigo/issues) and [discussions](https://github.com/prilive-com/galigo/discussions)
2. Open an issue to discuss the approach
3. Wait for maintainer feedback before starting work

### API Stability

galigo aims for API stability. Breaking changes require:
1. Discussion in an issue first
2. Maintainer approval
3. Migration guide in the PR

## Getting Help

- **Questions**: [GitHub Discussions](https://github.com/prilive-com/galigo/discussions)
- **Bugs**: [GitHub Issues](https://github.com/prilive-com/galigo/issues)
- **Security**: [Private Vulnerability Reporting](https://github.com/prilive-com/galigo/security)

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).