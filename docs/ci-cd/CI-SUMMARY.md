# galigo CI/CD Configuration Summary

**Updated:** February 2026  
**Status:** Production-ready, all consultants' feedback incorporated

---

## Files Included

```
.github/
├── dependabot.yml          # Automated dependency updates
├── workflows/
│   ├── ci.yml              # Main CI pipeline (lint, test, coverage)
│   ├── fuzz.yml            # Weekly fuzz testing
│   ├── integration.yml     # Live Telegram API tests
│   ├── release.yml         # Automated releases with SBOM
│   ├── scorecard.yml       # OpenSSF security scorecard
│   ├── security.yml        # govulncheck, gosec, CodeQL
│   └── stale.yml           # Stale issue management
.golangci.yml               # Linter configuration (v2 format)
```

---

## Key Features

### ci.yml — Main CI Pipeline
| Feature | Status | Notes |
|---------|--------|-------|
| Multi-OS matrix | ✅ | Ubuntu, macOS, Windows |
| Multi-Go version | ✅ | `stable` + `oldstable` |
| Race detector | ✅ | Linux only (speed optimization) |
| Coverage threshold | ✅ | 80% minimum |
| golangci-lint | ✅ | v2 format, blocking (no continue-on-error) |
| go-version-file | ✅ | Uses `go.mod` as single source of truth |
| Concurrency groups | ✅ | Prevents duplicate runs |
| Dependency Review | ✅ | Blocks high-severity CVEs in PRs |

### security.yml — Security Scanning
| Tool | Purpose |
|------|---------|
| govulncheck | Official Go vulnerability scanner |
| gosec | Security linter with SARIF upload |
| CodeQL | GitHub's semantic code analysis |
| Dependency Review | Blocks vulnerable deps in PRs |

### integration.yml — Live Telegram Tests
| Security Layer | Implementation |
|----------------|----------------|
| GitHub Environment | `telegram-live` with required reviewers |
| Repository check | Only runs for repository owner |
| harden-runner | Monitors network egress for token exfiltration |
| Secret management | Environment-scoped secrets only |

### release.yml — Automated Releases
| Feature | Status |
|---------|--------|
| Validation | Full test + lint + vuln scan before release |
| Changelog | Auto-generated from commit messages |
| Multi-platform binaries | Linux, macOS, Windows (amd64, arm64) |
| SBOM | CycloneDX format via Anchore |
| Checksums | SHA256 for all artifacts |

### fuzz.yml — Fuzz Testing
| Feature | Status |
|---------|--------|
| Schedule | Weekly (Sunday 2 AM UTC) |
| Targets | 5 fuzz targets from `tg/fuzz_test.go` |
| Go version | Uses `go.mod` (fixed from hardcoded) |
| Corpus caching | Persists between runs |
| Auto-issue creation | Creates GitHub issue on crash |

---

## Changes Made

### fuzz.yml Fix
```yaml
# BEFORE (hardcoded):
env:
  GO_VERSION: '1.25'

- uses: actions/setup-go@v5
  with:
    go-version: ${{ env.GO_VERSION }}

# AFTER (consistent):
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod  # Single source of truth
```

---

## golangci-lint Configuration

The `.golangci.yml` uses **v2 format** (stable since August 2025):

```yaml
version: "2"

linters:
  default: standard
  enable:
    # Bug Detection
    - bodyclose
    - contextcheck
    - errcheck
    - errorlint
    - nilerr
    - staticcheck
    
    # Security
    - gosec
    
    # Performance
    - prealloc
    - unconvert
    
    # Code Quality
    - gocritic
    - gocyclo
    - gocognit
    
    # Style
    - gofmt
    - goimports
    - revive
    
    # Go 1.25+
    - sloglint
    - intrange
```

**Note:** golangci-lint v2.x requires v2 config format. The v1 format is NOT supported in v2 binaries.

---

## Verification Results

| Concern | Status | Evidence |
|---------|--------|----------|
| `continue-on-error` in linting | ✅ NOT PRESENT | Only in fuzz.yml (intentional for crash capture) |
| golangci-lint v2 format | ✅ CORRECT | v2.8.0 is current stable (Jan 2026) |
| Multi-OS testing | ✅ PRESENT | Ubuntu, macOS, Windows matrix |
| Race detector Linux-only | ✅ PRESENT | Conditional in test job |
| harden-runner | ✅ PRESENT | In integration.yml only |
| SBOM generation | ✅ PRESENT | In release.yml |
| Scorecard | ✅ PRESENT | Weekly schedule |

---

## Environment Setup Required

### GitHub Environments

Create environment `telegram-live` in Settings → Environments:

**Secrets:**
- `TESTBOT_TOKEN` — Bot token from @BotFather
- `TESTBOT_CHAT_ID` — Test chat ID
- `TESTBOT_ADMINS` — Comma-separated admin user IDs

**Protection Rules:**
- Required reviewers: Add maintainers
- Wait timer: 0 (optional)

### Repository Settings

1. **Branch Protection** (main):
   - Require status checks: `Lint`, `Test`, `Coverage`, `Build`
   - Require branches to be up to date
   - Require review from CODEOWNERS

2. **Secrets:**
   - `CODECOV_TOKEN` (optional, for coverage upload)

---

## CI Pipeline Flow

```
┌─────────────────────────────────────────────────────────────┐
│                      Push / PR to main                       │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
         ┌────────┐     ┌──────────┐    ┌──────────────┐
         │  Lint  │     │   Test   │    │   Security   │
         │        │     │ (matrix) │    │              │
         └────────┘     └──────────┘    └──────────────┘
              │               │               │
              └───────────────┼───────────────┘
                              ▼
                       ┌──────────┐
                       │ Coverage │
                       │  Build   │
                       └──────────┘
                              │
                              ▼
                    ┌────────────────┐
                    │ Testbot Status │
                    │  (--status)    │
                    └────────────────┘
```

---

## Maintenance Notes

1. **Go Version:** Managed in `go.mod`, all workflows use `go-version-file`
2. **Action Versions:** Dependabot auto-updates weekly
3. **golangci-lint:** Use migration command for config updates: `golangci-lint migrate`
4. **Scorecard:** Results appear in Security tab automatically
