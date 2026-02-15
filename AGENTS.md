# Repository Guidelines

## Project Structure & Module Organization
This is a Go monorepo for the ShellTime CLI and daemon.
- `cmd/cli/main.go`: CLI entrypoint (`shelltime`)
- `cmd/daemon/main.go`: daemon entrypoint (`shelltime-daemon`)
- `commands/`: CLI command implementations (for example `sync.go`, `doctor.go`, `daemon.install.go`)
- `daemon/`: background services, socket handling, sync processors, OTEL handlers
- `model/`: core domain logic (config, API clients, crypto, shell integrations)
- `docs/`: user-facing docs (`CONFIG.md`, `CC_STATUSLINE.md`)
- `fixtures/`: test fixtures

Keep new code in the existing package boundary; avoid mixing CLI wiring, daemon internals, and model logic.

## Build, Test, and Development Commands
- `go build -o shelltime ./cmd/cli/main.go`: build the CLI binary
- `go build -o shelltime-daemon ./cmd/daemon/main.go`: build the daemon binary
- `go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...`: run full test suite with coverage (matches CI)
- `go test ./commands/...` (or `./daemon/...`, `./model/...`): package-level tests
- `go test -run TestName ./daemon/`: run a single test
- `go fmt ./... && go vet ./...`: format and static checks
- `mockery`: regenerate mocks (configured by `.mockery.yml`)
- `pp g`: regenerate PromptPal-generated types before tests/releases

## Coding Style & Naming Conventions
Use standard Go conventions and keep code `gofmt`-clean (tabs, canonical spacing/import grouping).
- File naming: lowercase with underscores or dotted qualifiers (for example `daemon.install.go`, `api.base.go`)
- Tests: `*_test.go` files with clear `TestXxx` names
- Commits and scopes should reflect touched package areas (`commands`, `daemon`, `model`, `docs`)

## Testing Guidelines
Testing uses Go `testing` plus `testify`.
- Prefer table-driven tests for pure logic
- Use suite-based tests (`suite.Suite`, `SetupTest`, `TearDownTest`) for stateful daemon flows
- Keep fixtures in `fixtures/` when payloads are reused
- Ensure `go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...` passes before opening PRs

## Commit & Pull Request Guidelines
History follows Conventional Commits with scope, e.g. `fix(daemon): ...`, `feat(commands): ...`, `refactor(model): ...`.
- Write focused commits with one behavioral change each
- PRs should include: concise summary, why the change is needed, and test evidence
- Link related issues when applicable
- If behavior/output changes, include CLI examples or screenshots
- Regenerate artifacts (`pp g`, `mockery`) when relevant so CI stays green
