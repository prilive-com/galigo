# Contributing to galigo

First off, thank you for considering contributing to galigo! 🎉

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Style Guidelines](#style-guidelines)
- [Getting Help](#getting-help)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Getting Started

### Good First Issues

Looking for a place to start? Check out issues labeled:
- [`good first issue`](https://github.com/prilive-com/galigo/labels/good%20first%20issue) - Simple issues for newcomers
- [`help wanted`](https://github.com/prilive-com/galigo/labels/help%20wanted) - Issues where we need help

### Types of Contributions

We welcome:
- 🐛 Bug fixes
- ✨ New features (especially new Telegram Bot API methods)
- 📚 Documentation improvements
- 🧪 Test improvements
- ⚡ Performance optimizations

## Development Setup

### Prerequisites

- Go 1.23 or later (1.25 recommended)
- Git
- A Telegram bot token (for integration tests)

### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/prilive-com/galigo.git
cd galigo

# Verify the setup
go mod download
go build ./...
go test ./...
```

### Install Development Tools

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install govulncheck
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Making Changes

### Branch Naming

Use descriptive branch names:
- `feat/add-send-poll-method`
- `fix/rate-limiter-race-condition`
- `docs/improve-webhook-examples`
- `refactor/simplify-error-handling`

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance tasks

Examples:
```
feat(sender): add SendPoll method

Implements sendPoll Telegram Bot API method with support for
regular and quiz polls.

Closes #123
```

```
fix(receiver): prevent race condition in offset update

Use atomic operations instead of mutex to fix potential
race condition when updating polling offset.
```

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests with race detector
make test-race

# Run tests with coverage
make test-coverage

# Run linter
make lint

# Run full CI suite
make ci
```

### Writing Tests

- Place tests in `*_test.go` files alongside the code
- Use table-driven tests where appropriate
- Use the testutil package for mocking
- Aim for meaningful coverage, not just high percentages

Example test structure:
```go
func TestSendMessage_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot.../sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    msg, err := client.SendMessage(ctx, req)

    require.NoError(t, err)
    assert.Equal(t, 123, msg.MessageID)
}
```

### Integration Tests

For testing against the real Telegram API:

```bash
# Set up environment
export TESTBOT_TOKEN="your-bot-token"
export TESTBOT_CHAT_ID="your-chat-id"
export TESTBOT_ADMINS="your-user-id"

# Run smoke test
go run ./cmd/galigo-testbot --run smoke

# Run full test suite
go run ./cmd/galigo-testbot --run all
```

## Submitting Changes

### Before Submitting

1. **Run the full test suite**
   ```bash
   make ci
   ```

2. **Format your code**
   ```bash
   go fmt ./...
   goimports -w .
   ```

3. **Run the linter**
   ```bash
   golangci-lint run
   ```

4. **Update documentation** if you changed public APIs

### Pull Request Process

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Push to your fork
5. Open a Pull Request

### PR Requirements

- [ ] All tests pass
- [ ] No linter errors
- [ ] Code is formatted (`gofmt`)
- [ ] New code has tests
- [ ] Public APIs have godoc comments
- [ ] PR description explains the changes

## Style Guidelines

### Go Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` and `goimports`
- Keep functions focused and small
- Prefer returning errors over panicking
- Use meaningful variable names

### Documentation

- All exported functions need godoc comments
- Start comments with the function name
- Include examples for complex APIs

```go
// SendMessage sends a text message to the specified chat.
// It supports various options like parse mode and reply markup.
//
// Example:
//
//	msg, err := client.SendMessage(ctx, sender.SendMessageRequest{
//	    ChatID: 12345,
//	    Text:   "Hello, World!",
//	})
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*tg.Message, error) {
```

### Error Handling

- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Use sentinel errors for known conditions
- Never ignore errors silently (use `//nolint:errcheck` if intentional)

## Getting Help

- 💬 [GitHub Discussions](https://github.com/prilive-com/galigo/discussions) - Questions and ideas
- 🐛 [Issues](https://github.com/prilive-com/galigo/issues) - Bug reports and feature requests
- 📚 [Documentation](https://pkg.go.dev/github.com/prilive-com/galigo) - API reference

## Recognition

Contributors are recognized in:
- Release notes
- README contributors section
- GitHub contributors page

Thank you for contributing to galigo! 🚀
