.PHONY: test test-coverage test-race test-fuzz test-short lint ci bench vuln clean help

# Go parameters
GO := go
GO_PACKAGES := ./...
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
COVERAGE_THRESHOLD := 80

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
NC := \033[0m # No Color

## help: Show this help message
help:
	@echo "galigo - Telegram Bot API Library"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## test: Run all tests
test:
	$(GO) test -v $(GO_PACKAGES)

## test-short: Run tests without long-running tests
test-short:
	$(GO) test -v -short $(GO_PACKAGES)

## test-coverage: Run tests with coverage report
test-coverage:
	$(GO) test -v -coverpkg=$(GO_PACKAGES) -coverprofile=$(COVERAGE_FILE) $(GO_PACKAGES)
	$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report: $(COVERAGE_HTML)"

## test-race: Run tests with race detector
test-race:
	$(GO) test -v -race $(GO_PACKAGES)

## test-fuzz: Run fuzz tests (30s each)
test-fuzz:
	@echo "Running FuzzDecodeUpdate..."
	$(GO) test -fuzz=FuzzDecodeUpdate -fuzztime=30s ./tg/ || true
	@echo "Running FuzzChatID..."
	$(GO) test -fuzz=FuzzChatID -fuzztime=30s ./tg/ || true

## lint: Run linters
lint:
	@echo "Running go vet..."
	$(GO) vet $(GO_PACKAGES)
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint not installed, skipping..." && exit 0)
	@golangci-lint run $(GO_PACKAGES) 2>/dev/null || true

## ci: Run CI pipeline (lint + race + coverage with threshold)
ci: lint test-race test-coverage check-coverage

## check-coverage: Check if coverage meets threshold
check-coverage:
	@COVERAGE=$$($(GO) tool cover -func=$(COVERAGE_FILE) | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${COVERAGE}%"; \
	if [ $$(echo "$${COVERAGE} < $(COVERAGE_THRESHOLD)" | bc -l 2>/dev/null || echo "1") -eq 1 ]; then \
		if command -v bc > /dev/null 2>&1; then \
			echo "$(RED)FAIL: Coverage $${COVERAGE}% is below $(COVERAGE_THRESHOLD)% threshold$(NC)"; \
			exit 1; \
		else \
			echo "$(YELLOW)WARN: bc not available, skipping threshold check$(NC)"; \
		fi; \
	else \
		echo "$(GREEN)PASS: Coverage meets $(COVERAGE_THRESHOLD)% threshold$(NC)"; \
	fi

## bench: Run benchmarks
bench:
	$(GO) test -bench=. -benchmem $(GO_PACKAGES)

## vuln: Run vulnerability check
vuln:
	@which govulncheck > /dev/null 2>&1 || (echo "Installing govulncheck..." && $(GO) install golang.org/x/vuln/cmd/govulncheck@latest)
	govulncheck $(GO_PACKAGES)

## fmt: Format code
fmt:
	$(GO) fmt $(GO_PACKAGES)

## tidy: Tidy go.mod
tidy:
	$(GO) mod tidy

## build: Build all packages
build:
	$(GO) build $(GO_PACKAGES)

## clean: Clean build artifacts
clean:
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	$(GO) clean -testcache

## verify: Run all verification (fmt + tidy + build + lint + test)
verify: fmt tidy build lint test
