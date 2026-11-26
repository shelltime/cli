# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ShellTime CLI is a Go-based command-line tool for tracking DevOps work. It consists of two main binaries:
- `shelltime`: The main CLI tool for command tracking and management
- `shelltime-daemon`: A background service for asynchronous command tracking, synchronization, and OTEL data collection

## Development Commands

**Requires Go 1.27.1+**

### Building
```bash
# Build the CLI binary
go build -o shelltime ./cmd/cli/main.go

# Build the daemon binary
go build -o shelltime-daemon ./cmd/daemon/main.go

# Build with all ldflags (version, commit, date, Uptrace DSN)
go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%d) -X main.uptraceDsn=<dsn>" -o shelltime ./cmd/cli/main.go
```

### Testing
```bash
# Run all tests with coverage
go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...

# Run tests for a specific package
go test ./commands/...
go test ./daemon/...
go test ./model/...

# Run a single test
go test -run TestHandlerName ./daemon/
```

Tests use **testify** (assertions + suites). Suite-based tests use `suite.Suite` with `SetupTest`/`TearDownTest` lifecycle hooks (see `daemon/cc_info_handler_test.go` for example). Simple functions use table-driven tests.

Tests must never reach the daemon running on the machine (yours, or a Claude Code cloud session's): it uploads to a real account. `commands/main_test.go` points `track`'s fast-path socket (`trackSocketPath`) at a path that doesn't exist, and every other test uses a temp `socketPath`. The `commands` and `daemon` `TestMain`s clear `CLAUDE_CODE_REMOTE*`, so tests behave the same inside a cloud session as on CI. The `daemon` and `model` `TestMain`s also give their suites a temporary `HOME`: what the daemon couldn't send is queued under `~/.shelltime` (`sync-pending.jsonl`, `coding-heartbeat.data.log`), where the machine's daemon would pick it up and resend it, and a test that swaps a storage path without setting `HOME` would otherwise write into the real one.

### Performance (`shelltime track` latency)
`perf/` is a separate Go module (root `./...` skips it). It benchmarks the real binary the way the shell hooks run it: daemon and direct paths, pre and post, plus a sync. See `perf/README.md`.
```bash
# Benchmark the current tree
go -C perf test -run '^$' -bench . -benchtime 50x -count 5

# Compare a base commit with the working tree, as CI does
perf/compare.sh origin/main
```
The `Track Perf` workflow (`.github/workflows/perf.yaml`) runs this on every PR and push to `main`. It posts the report as a PR comment and in the job summary. A scenario that gets more than 10% slower (and more than 0.25 ms, with p < 0.05) raises a warning, never a failure. `Track/*` scenarios skip while a real daemon owns `/tmp/shelltime.sock`.

### Code Generation
```bash
# Generate mocks (uses .mockery.yml configuration, Mockery v3)
mockery
```

CI runs `mockery` before tests. Generated files: `model/mock_*.go` (testify mocks for service interfaces).

### Linting
```bash
go vet ./...
go fmt ./...
```

## Architecture

### Package Structure
- **cmd/cli/**: CLI entry point - registers all commands, initializes services via dependency injection
- **cmd/daemon/**: Daemon entry point - sets up pub/sub, socket handler, and optional AICodeOtel gRPC server
- **commands/**: CLI command implementations - each command in its own file, `base.go` holds injected services
- **daemon/**: Daemon internals - socket handler, Watermill pub/sub channel, AICodeOtel gRPC server/processor
- **model/**: Business logic - API clients, config, crypto, shell hooks, service installers, dotfile handlers

### Service Interfaces (model package)
Three key interfaces with dependency injection:
- `ConfigService`: Reads and merges config from `config.yaml`/`config.toml` (YAML preferred) plus the matching `config.local.*` overrides
- `AIService`: streams `shelltime q` command suggestions from the ShellTime API (`POST /api/v1/ai/command-suggest`, server-sent events)
- `CommandService`: Executable lookup with fallback paths (handles daemon's limited PATH)

Injection happens in `cmd/*/main.go` via `commands.InjectVar()` and `commands.InjectAIService()`.

### Daemon Architecture
1. **SocketHandler**: Unix domain socket server accepting JSON messages from CLI
2. **GoChannel**: Watermill pub/sub for decoupled message processing
3. **SocketTopicProcessor**: Consumes messages and routes to appropriate handlers

Editor heartbeats (the VS Code, JetBrains and Neovim plugins) arrive as `heartbeat` messages. The socket answers
`{"status":"disabled"}` while code tracking is off in the config on disk (read per message, so turning it on needs no
restart) and `ok` once the message is queued; the plugins keep a batch until it is acknowledged, then drop it.
`handlePubSubHeartbeat` posts it to `/api/v1/heartbeats` and on any failure (also a 200 whose `errors` exceed the rows
without an id or entity) appends it to `~/.shelltime/coding-heartbeat.data.log`. `HeartbeatResyncService` resends each
saved batch in chunks of at most 500 by the same rule and writes back only the chunks the server didn't store.
On SIGTERM `SocketHandler.Shutdown` stops accepting, saves heartbeats still queued to that file instead of sending them
(a send in flight is cut short and saved too) and drains the channel for up to `ShutdownDrainTimeout`. If the drain
times out (a sync to a slow server holds the single processor), it saves the heartbeats still in the channel
(`GoChannel.PendingPayloads`) before closing it; one may then be both sent and saved, which the heartbeat id dedupes.

Optional daemon services (feature-gated via config):
- **CCInfoTimerService**: Lazy-fetch background timer for Claude Code statusline data (cost, quota, git info), plus an always-on Anthropic usage sync (10-min interval, only while a `claude` process is running) that pushes quota to `/api/v1/anthropic-usage` without needing statusline activity
- **CodexUsageSyncService**: Syncs Codex usage to `/api/v1/codex-usage` every 10 min, only while a `codex`/`chatgpt` process is running (both intervals share the `usageSyncInterval` const in `daemon/usage_sync.go`)
- **SyncCircuitBreakerService**: Retry failed syncs with file-based persistence (`~/.shelltime/sync-pending.jsonl`, one sync message per line, of any length) and hourly recovery timer. `SaveForRetry` and the timer's read-republish-rewrite share a lock, and the rewrite keeps lines saved while it was republishing
- **AICodeOtelServer**: gRPC OTEL collector for AI coding CLI metrics, logs and traces (Claude Code, Codex). Spans (`claude_code.interaction`, `llm_request`, `tool`, ...) go to the same `/api/v1/cc/otel` endpoint in `spans`; a span without `session.id` takes it from another span of its trace in the same export. `shelltime cc install` turns traces on (`CLAUDE_CODE_ENHANCED_TELEMETRY_BETA`, `OTEL_TRACES_EXPORTER=otlp`). In a Claude Code cloud session (`CLAUDE_CODE_REMOTE=true`, where the SessionStart hook starts the daemon) it adds the VM's `CLAUDE_CODE_REMOTE_SESSION_ID` as the `ccr.session.id` attribute to Claude Code records that lack it; the server marks those sessions as cloud sessions. `daemon/main_test.go` clears both variables so tests behave the same inside a cloud session
- **Git repositories** (`model/git_repository.go`): the server groups coding agent sessions into projects by git repository, not folder, so a renamed or moved clone, a worktree or a subfolder stays one project. `GitRepositoryResolver` asks the git CLI (found through `CommandService`, so the daemon's limited PATH reaches Homebrew) for a directory's top level and its `origin` remote (else `upstream`, else the first by name; `remote get-url`, so `insteadOf` applies) without optional locks, within one 2 s budget per lookup, since the daemon resolves on the OTLP export path. Concurrent lookups of one directory share one run, which goes on its own goroutine: a stat or git blocked in the kernel (a hung network mount) ignores the context, so the lookup is given up on after the budget (plus `gitRepositoryGrace`), or when its caller's context ends, and the directory gets no other until it returns (its answer is kept then). git runs with `LC_ALL=C`, since its messages are read; a remote's path segments are decoded and escaped again (`url.PathEscape`), like the server's. A migration run stops asking after `projectMigrationMaxUnanswered` folders' stats timed out (a hung mount; git refusing a folder doesn't count), and a folder that doesn't exist counts as missing, one it can't stat otherwise as unchecked. git refusing a repository it doesn't trust (`safe.directory`) or killed by a signal is no answer about the directory; a git binary not found is looked for again every 30 s. It keeps definitive answers, negative ones too (10 min in the daemon), and a timeout or git failing to start for 30 s only, reported as unknown by `Lookup` (`Resolve` drops the flag). `CanonicalRepositoryURL` turns any remote into `https://<host>/<path>` without credentials, port or `.git`, the form Claude Code reports; a local path is no repository. The repository travels as `vcs.repository.url.full` (`model.AICodeRepositoryAttribute`): the OTEL processor adds it to metrics and log events whose resource has a `pwd` and that don't name one (not to spans: the server keys a session from its events), and never when the resource names one (Claude Code's own value wins; git isn't asked then), the socket's `session_project` handler sends it with the statusline's directory, and backfill adds it to uploaded events (Codex's `session_meta.git.repository_url` first, since it was true when the session ran, else the transcript directory's). `aiCodeOtel.sendRepository: false` (`ShellTimeConfig.SendsRepository`) turns all of it off: no resolver in the processor or the socket, the processor removes the attribute from every record (the agent's own too), backfill attaches none, and neither the daemon nor `cc projects` migrates
- **Project migration** (`daemon/project_migration.go`, `model/project_migration.go`): sessions recorded before the repository was sent are folder projects on the server. A while after start the daemon runs `MigrateProjectRepositories` once, never in AI cloud only mode (marker `~/.shelltime/project-repositories.v1`, written only when the run is `Complete`: a 404 from an older server, a failure, more pages than one run goes through (50), a folder git couldn't answer for, or a machine without a host name of its own leaves none, so the next start tries again): it lists the folder projects of this machine's names (`ProjectMigrationHosts`: the host name, without its domain, and the `machine.name` in `~/.claude/settings.json`, leaving out names many machines share, like `localhost`, `genericHostNames`) from `POST /api/v1/cc/projects/unresolved`, a page at a time by the last project of the page (`after`), and posts the folders that still exist and are clones to `POST /api/v1/cc/projects/repositories`. `shelltime cc projects` / `shelltime codex projects` run it by hand (`--dry-run` lists what would move; skipped in AI cloud only mode)
- **HeartbeatResyncService**: Periodic resync of failed heartbeats (30-min interval)
- **CleanupTimerService**: Periodic log file cleanup (24-hour interval)

Services initialize in `cmd/daemon/main.go`: check enabled flag → create → start → defer stop. All run concurrently with graceful shutdown on SIGINT/SIGTERM.

### Data Flow
1. Shell hooks capture commands → CLI stores locally (file-based buffer)
2. CLI sends sync message to daemon via Unix socket
3. Daemon's pub/sub routes to sync handler
4. Batch sync to shelltime.xyz API with optional encryption

### Txt command store
`~/.shelltime/commands/{pre,post,cursor}.txt` is the file store (`model/store_file.go`), shared by processes that don't know of each other: a `track` per command (when no daemon takes it), a `gc` per new shell, `sync`, and the daemon unless it uses bolt. Lines are only appended, except by a compaction, which drops synced post commands and the pre commands they complete: the daemon's `Prune` after a sync and `gc` (`model.CompactCommandFiles`, with `.bak` copies) are one implementation, `fileStore.compact`. `cursor.txt` gets a line from every sync, and the store's cursor is the latest of them, not the last line (`parseLastCursor`): two processes can sync the same store, and the one that covered less may write last. A compaction goes by the later of that cursor and the one its caller names, and writes it back as the only line, so nothing moves the cursor backwards. A pre command without a post is kept, however old. It is a command still running or one whose post will never come (the terminal was closed under it, or an older gc dropped the synced post and kept the pre), and the store can't tell which. Don't expire them by age: `BuildTrackingData` leaves out a post that has no pre under its key, so a command that outlived its pre would not be recorded, and that skip can't go either, because it is what keeps the stray posts of hook scripts from before 0.1.94 (a post at every zsh prompt, a repeated post on an empty enter in bash; the scripts stay on disk until `shelltime hooks install` runs again) from being uploaded as commands.

`commands/store.lock` is an advisory `flock` (`model/store_lock*.go`): every append holds it shared, a compaction holds it exclusively from reading the files to replacing them. Readers don't take it; a compaction replaces each file with a rename, so they see the old file or the new one. Nothing waits for the lock longer than `commandStoreLockWait` (200 ms): an append then goes ahead unlocked, a compaction returns `ErrCommandStoreBusy` and changes nothing. Where there is no lock (Windows, Solaris, a filesystem without `flock`) and for appends by binaries from before it, a compaction still re-reads each file right before replacing it and keeps what was appended since its first read, which leaves only the moment between that read and the rename. A file that no longer starts with what was first read was rewritten by such a process: the compaction stops with `ErrCommandStoreChanged` (gc ignores it like `ErrCommandStoreBusy`, the daemon logs it), and the next one does the work. Append to these files through the store or `DoSavePre`/`DoUpdate`, and rewrite them only through `compact`, never directly.

### Configuration
- Main config: `$HOME/.shelltime/config.yaml` (YAML preferred; `config.yml` and `config.toml` are also accepted)
- Local overrides: `$HOME/.shelltime/config.local.yaml` (merged over the base, gitignored)
- Daemon socket: `/tmp/shelltime.sock` (configurable via `socketPath`)
- AICodeOtel gRPC port: configurable via `aiCodeOtel.grpcPort` (default: 54027)
- Outbound proxy: `proxy.url` / `proxy.noProxy` (http, https, socks5, socks5h). Applied once at startup via `model.ConfigureProxy`; every outbound HTTP client must use `model.NewHTTPClient` or `model.HTTPTransport()` so the proxy applies
- Token `proxy-injected` (`model.ProxyInjectedToken`): a Claude Code cloud environment keeps the real token as a network secret, and its agent proxy adds `Authorization: CLI <token>` to requests for `api.shelltime.xyz`. Requests to the API then carry no `Authorization` header of their own (`setCLIAuthorization`); set it through that helper, never by hand. `cloud.bash` in the installation repo writes it
- AI cloud only (`aiCloudOnly`, `model.IsAICloudOnly`): collect only AI coding agent telemetry and nothing about the machine, so it never becomes a host. Unset, it is on where `CLAUDE_CODE_REMOTE=true` (a Claude Code cloud VM); an explicit value wins. The daemon's socket drops `track_pre`/`track_post`/`sync` and heartbeats, and `cmd/daemon/main.go` skips the bolt store, sync circuit breaker and heartbeat resync; OTEL, `cc pr` and session-project pass through. In the CLI, `track` without a daemon stores nothing, and `sync`, `alias import`, `dotfiles push` and `hooks install` print why and return nil (`skippedInAICloudOnly`). `doctor` reports it under Configuration and skips Shell Hooks

## Commit Rules

Follow Conventional Commits with scope: `fix(daemon): ...`, `feat(cli): ...`, `refactor(model): ...`

## Key Dependencies

- **CLI Framework**: `urfave/cli/v2`
- **Message Queue**: `ThreeDotsLabs/watermill` (in-process pub/sub for daemon)
- **Telemetry**: `uptrace-go` for OTEL
- **Config**: `pelletier/go-toml/v2`
- **Git**: `go-git/v5` for branch/dirty detection
- **gRPC**: OTEL collector protos for AICodeOtel server

## Release

Releases use Release-Please (always-bump-patch) + Goreleaser. macOS builds include code signing/notarization via Quill when credentials are present. CI config in `.github/workflows/`.

## Important Notes

- Daemon is optional but recommended (<8ms latency vs ~100ms+ direct)
- Encryption requires daemon mode and a token with encryption capability
- Shell hooks are platform-specific (bash, zsh, fish) - test on target shells
- CC statusline quota display needs Claude Code's OAuth token: from the Keychain on macOS, from `~/.claude/.credentials.json` on Linux; other platforms omit it
- AICodeOtel feature enables AI coding CLI metrics/logs/traces passthrough via gRPC (port 54027) - supports Claude Code, Codex, and other OTEL-compatible CLIs