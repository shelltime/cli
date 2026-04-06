# Repository Guidelines

## Project Identity
This repo is the ShellTime CLI and daemon. Public-facing docs, command examples, and product references should use `ShellTime` and `shelltime.xyz`.

Code and imports currently use the module path `github.com/malamtime/cli`. Do not "fix" ShellTime branding in docs just because the module path says `malamtime`; the mismatch is intentional in the current repo state.

## Project Structure & Package Boundaries
This is a Go monorepo for the ShellTime CLI and daemon.

- `cmd/cli/main.go`: CLI entrypoint for `shelltime`
- `cmd/daemon/main.go`: daemon entrypoint for `shelltime-daemon`
- `commands/`: CLI command definitions and user-facing command behavior
- `daemon/`: background services, socket handling, processors, and OTEL handlers
- `model/`: config, API clients, shell integrations, crypto, and shared domain logic
- `docs/`: user-facing docs such as `CONFIG.md` and `CC_STATUSLINE.md`
- `fixtures/`: reusable test fixtures

Keep new code inside the existing package boundary. Do not mix CLI wiring, daemon internals, and model logic in the same package.

## Build, Test, and Development Commands

- `go build -o shelltime ./cmd/cli/main.go`: build the CLI binary
- `go build -o shelltime-daemon ./cmd/daemon/main.go`: build the daemon binary
- `go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...`: run the full suite with coverage
- `go test ./commands/...`: run command-package tests
- `go test ./daemon/...`: run daemon-package tests
- `go test ./model/...`: run model-package tests
- `go test -run TestName ./daemon/`: run a targeted daemon test
- `go fmt ./...`: format Go code
- `go vet ./...`: run static analysis
- `mockery`: regenerate mocks when interfaces change
- `pp g`: regenerate PromptPal-generated artifacts when relevant

Use Go 1.26, as declared in `go.mod`.

## Coding Style & Naming Conventions
Use standard Go conventions and keep code `gofmt`-clean.

- File naming: lowercase with underscores or dotted qualifiers such as `daemon.install.go` or `api.base.go`
- Tests: `*_test.go` with clear `TestXxx` names
- Prefer table-driven tests for pure logic
- Use `testify` suites for stateful daemon flows
- Keep comments brief and only where they reduce real ambiguity

## Testing Guidance

- Put reusable payloads and fixtures under `fixtures/`
- Prefer package-level test runs while iterating, then run the full suite before finishing
- Ensure `go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...` passes before opening a PR or cutting a release

## Documentation Maintenance
When command behavior, setup flow, config formats, or integrations change, update the docs in the same change.

- `README.md`: concise user-facing overview, install/setup flow, current command surface, and links
- `docs/CONFIG.md`: detailed config semantics, file locations, defaults, and OTEL settings
- `docs/CC_STATUSLINE.md`: Claude Code statusline behavior, formatting, and platform notes
- `AGENTS.md`: contributor and agent workflow guidance for this repo

Do not leave `README.md` advertising commands that no longer exist, and do not document new commands only in code.

## Commit & Pull Request Guidelines
History follows Conventional Commits with scope, for example:

- `fix(daemon): ...`
- `feat(commands): ...`
- `refactor(model): ...`
- `docs(readme): ...`

Keep commits focused on one behavioral change. PRs should include a short summary, why the change is needed, and test evidence. Link related issues when applicable. If behavior or output changes, include CLI examples or screenshots. Regenerate artifacts such as `pp g` and `mockery` when needed so CI remains green.
