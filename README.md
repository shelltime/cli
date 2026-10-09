# ShellTime CLI

[![codecov](https://codecov.io/gh/shelltime/cli/graph/badge.svg?token=N09WIJHNI2)](https://codecov.io/gh/shelltime/cli)
[![shelltime](https://api.shelltime.xyz/badge/AnnatarHe/count)](https://shelltime.xyz/users/AnnatarHe)

ShellTime is a CLI and background daemon that tracks your shell activity, syncs your command history, and pipes your AI coding tools into one shared telemetry stream. The hosted service lives at [shelltime.xyz](https://shelltime.xyz).

## Install

### Homebrew (macOS)

```bash
brew install shelltime/tap/shelltime
```

### curl installer (macOS and Linux)

```bash
curl -sSL https://shelltime.xyz/i | bash
```

On macOS the script installs through Homebrew when `brew` is available. Otherwise it puts `shelltime` and `shelltime-daemon` in `~/.shelltime/bin`.

### Upgrading

If you installed with the curl script, upgrade in place. This also replaces the daemon binary and reinstalls the daemon service if you have one:

```bash
shelltime update           # --check only compares versions
```

If you installed with Homebrew, upgrade through brew instead (`shelltime update` detects this and tells you to):

```bash
brew upgrade shelltime/tap/shelltime
```

## Quick Start

The fastest way to get set up is a single command:

```bash
shelltime init
```

`shelltime init` authenticates the CLI (in the browser, or pass `--token`), installs the bash, zsh and fish hooks and the daemon, and tries to wire up Claude Code and Codex OTEL integration for you.

Prefer to do it step by step?

```bash
shelltime auth
shelltime hooks install
shelltime daemon install
shelltime cc install
shelltime codex install
```

## What ShellTime Does

- Tracks shell commands locally, with masking and exclusion rules to keep secrets out. Each command also records whether it ran over SSH, so the web timeline can group remote commands under the `ssh` that opened the session.
- Syncs your command history to ShellTime so you can search and analyze it.
- Runs a background daemon for low-latency, non-blocking sync.
- Forwards Claude Code and OpenAI Codex telemetry through OTEL, and backfills past sessions from their local transcripts.
- Syncs Claude Code and Codex quota windows every 10 minutes while those tools are running.
- Links pull requests to the AI coding sessions that opened them.
- Shows a live Claude Code statusline with cost, quota, time, and context usage.
- Syncs supported dotfiles to and from the ShellTime service.

## Command Overview

### Core setup and auth

| Command | Description |
|---------|-------------|
| `shelltime init` | Bootstrap auth, hooks, daemon, and AI-code integrations |
| `shelltime auth` | Authenticate with `shelltime.xyz` (`--token` skips the browser flow) |
| `shelltime update` | Download and install the latest release in place (`--check`, `--force`, `--skip-daemon-reinstall`) |
| `shelltime doctor` | Diagnose setup problems and show how to fix each one. `--fix` applies the safe fixes after asking (`--yes` skips the prompt), `--offline` skips network checks, `--format json` prints a machine-readable report. Exits non-zero when a check fails |
| `shelltime web` | Open the ShellTime dashboard in a browser |

### Tracking and sync

| Command | Description |
|---------|-------------|
| `shelltime track` | Record a shell command event (called by the shell hooks) |
| `shelltime sync` | Manually sync pending local data (`--dry-run`) |
| `shelltime ls` | List locally saved commands (`--format json`) |
| `shelltime gc` | Remove already-synced local records and oversized log files (`--withLog` clears the logs whatever their size) |
| `shelltime rg <text>` | Search synced command history. Alias `grep`. Filters: `--shell`, `--hostname`, `--username`, `--result`, `--main-command`, `--since`/`--until` (`2024`, `2024-01` or `2024-01-15`), `--limit`, `--format json` |

### AI helpers and integrations

| Command | Description |
|---------|-------------|
| `shelltime query "prompt"` | Ask AI for a suggested shell command, using context about your repo, project and machine (see [Query Context](docs/CONFIG.md#query-context)). `--show-context` prints the request without calling the AI |
| `shelltime q "prompt"` | Alias for `shelltime query` |
| `shelltime cc install` | Install Claude Code OTEL configuration into `~/.claude/settings.json` |
| `shelltime cc uninstall` | Remove Claude Code OTEL configuration from `~/.claude/settings.json` |
| `shelltime cc statusline` | Print the Claude Code statusline (reads the session JSON Claude Code sends on stdin) |
| `shelltime cc backfill` | Upload past Claude Code usage from local transcripts |
| `shelltime cc pr --session-id <id> <url>...` | Link pull requests to a Claude Code session (called by the ShellTime Claude Code mod) |
| `shelltime codex install` | Add ShellTime OTEL config to `~/.codex/config.toml` |
| `shelltime codex uninstall` | Remove ShellTime OTEL config from `~/.codex/config.toml` |
| `shelltime codex backfill` | Upload past Codex usage from local session files |

### Environment helpers

| Command | Description |
|---------|-------------|
| `shelltime hooks install` | Install shell hooks |
| `shelltime hooks uninstall` | Remove shell hooks |
| `shelltime daemon install` | Install the ShellTime daemon service |
| `shelltime daemon status` | Check daemon status |
| `shelltime daemon reinstall` | Reinstall the daemon service |
| `shelltime daemon uninstall` | Remove the daemon service |
| `shelltime alias import` | Import aliases from `~/.zshrc` and `~/.config/fish/config.fish` (`--full` re-imports everything) |
| `shelltime config view` | Show the merged current configuration (`--format json`) |
| `shelltime schema` | Print the JSON schema for config autocompletion (`-o <file>` writes it to a file) |
| `shelltime ios dl` | Open the ShellTime iOS App Store page |

### Dotfiles

| Command | Description |
|---------|-------------|
| `shelltime dotfiles push` | Push supported dotfiles to the server |
| `shelltime dotfiles pull` | Pull supported dotfiles to local config (`--dry-run` shows the changes first) |

Supported apps: `nvim`, `fish`, `git`, `zsh`, `bash`, `ghostty`, `claude`, `starship`, `npm`, `ssh`, `kitty` and `kubernetes`. Both commands take `--apps` (`-a`) to limit them to some apps; without it they cover all of them.

## Configuration

ShellTime stores data under `~/.shelltime/`.

- Main config: `~/.shelltime/config.yaml`
- Local overrides: `~/.shelltime/config.local.yaml`
- Also supported: `config.yml`, `config.toml`, `config.local.yml`, `config.local.toml`
- Generated schema: `~/.shelltime/config-schema.json` (written by `shelltime auth`, and referenced from the `config.yaml` it creates)
- Proxy: set `proxy.url` (`http`, `https`, `socks5`, `socks5h`) to route all outbound traffic through a proxy. See [Network Proxy](docs/CONFIG.md#network-proxy)

Minimal example:

```yaml
token: "your-api-token"
flushCount: 10
gcTime: 14
dataMasking: true

exclude:
  - ".*password.*"
  - "^export .*"
```

For every option, its default, and the OTEL and AI settings, see [docs/CONFIG.md](docs/CONFIG.md).

## Daemon Mode

The daemon keeps your shell fast by buffering commands and syncing them in the background, so a slow network never blocks your prompt.

| Mode | Latency | Blocks your shell? |
|------|---------|--------------------|
| Direct | ~100ms+ | Yes |
| Daemon | <8ms | No |

Run in daemon mode for lower shell latency, automatic sync retries, and background processing of sync and OTEL events. It is optional but recommended, and Claude Code and Codex telemetry, quota sync and encryption all need it.

`shelltime daemon install` registers `shelltime-daemon` as a launchd agent on macOS or a systemd user service on Linux. The CLI talks to it over `/tmp/shelltime.sock` (`socketPath`), and it receives Claude Code and Codex OTEL data over gRPC on `localhost:54027` (`aiCodeOtel.grpcPort`).

## Claude Code Statusline

ShellTime can provide a live statusline for [Claude Code](https://code.claude.com/docs/en/statusline).

Add this to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "shelltime cc statusline"
  }
}
```

Example output:

```text
🌿 main* | 🤖 Opus | 💰 $0.12 | 📊 $3.45 | 🚦 5h:23% 7d:12% | ⏱️ 5m30s | 📈 45%
```

For formatting details and platform notes, see [docs/CC_STATUSLINE.md](docs/CC_STATUSLINE.md).

## Claude Code and Codex Telemetry

`shelltime cc install` and `shelltime codex install` point each tool's OpenTelemetry exporter at the daemon, which forwards everything to ShellTime:

- **Claude Code**: the OTEL variables go into the `env` block of `~/.claude/settings.json`, so they reach every Claude Code session (terminal, desktop app, IDE). Blocks that older versions wrote to your shell rc files are removed. Restart running sessions afterwards.
- **Codex**: the exporter and flags are merged into the `[otel]` table of `~/.codex/config.toml`. Your other `[otel]` keys are kept.

Besides tokens and cost, both tools then send your prompts, tool details (shell commands, file paths, truncated tool input or output, errors) and the assistant's responses. To keep some of these out, set `OTEL_LOG_USER_PROMPTS`, `OTEL_LOG_TOOL_DETAILS` or `OTEL_LOG_ASSISTANT_RESPONSES` to `0` in the `env` of `~/.claude/settings.json`, or `otel.log_user_prompt` / `otel.log_agent_responses` to `false` in `~/.codex/config.toml`. Re-running the install command resets them. `shelltime doctor` warns when a config written by an older version is missing newer keys, and `shelltime doctor --fix` re-runs the install.

## Codex Usage Tracking

ShellTime receives Codex data through two independent paths:

- `shelltime codex install` configures Codex OTEL export for sessions, tokens, tool activity, and cost telemetry.
- The running `shelltime-daemon` reads your local Codex login, fetches the rate-limit windows and credit status currently returned by Codex, and syncs that summary every 10 minutes while a Codex process is running.

Quota sync requires both a ShellTime login (`shelltime auth`) and a ChatGPT-authenticated Codex installation. ShellTime reads the Codex access token from `~/.codex/auth.json` only for the direct request to Codex; the token stays on your machine, and only the returned plan, quota windows, percentages, reset times, and credit summary are sent to ShellTime.

Codex decides which windows are present. ShellTime displays the windows returned by Codex instead of assuming that every account has a fixed 5-hour window.

## Backfilling AI Usage

Live tracking only records sessions that run while the OTEL configuration is installed and the daemon is running. To upload earlier sessions from the transcripts Claude Code and Codex keep on disk:

```bash
shelltime cc backfill --dry-run   # show what would be uploaded
shelltime cc backfill             # upload Claude Code sessions
shelltime codex backfill          # upload Codex sessions
```

- Claude Code transcripts are read from `~/.claude/projects` and `~/.config/claude/projects`, or the directories in `CLAUDE_CONFIG_DIR`. Codex sessions are read from `~/.codex/sessions` and `~/.codex/archived_sessions`, or `CODEX_HOME`.
- Prompts, token usage, models and tool calls are uploaded as if they had been tracked live; the server adds costs. Lines of code, commits and active time are not in the transcripts.
- Sessions the server already has from live tracking are skipped, as are sessions still running. Running the command again only uploads what is missing.
- Flags: `--since` / `--until` (`YYYY-MM-DD`) limit the range, `--no-prompts` uploads prompt lengths without the text, and `--ai-summary` also generates AI session summaries, which use your monthly AI credits.
- Claude Code deletes transcripts after 30 days by default (`cleanupPeriodDays`), so only recent history may be available.

## Linking Pull Requests to AI Sessions

The [ShellTime Claude Code mod](https://github.com/shelltime/claude-code-mods) watches for `gh pr create` in Claude Code's Bash tool. When it sees one, it runs:

```bash
shelltime cc pr --session-id <claude-code-session-id> https://github.com/owner/repo/pull/123 [more URLs...]
```

The command hands the URLs to the daemon, and the daemon sends them to ShellTime, where they appear on the session. If no daemon is running, the CLI sends them itself. A session can link any number of PRs. Sending the same URL again does nothing. The command does nothing if you are not logged in.

For Codex, and for Claude Code sessions without the mod, the ShellTime server looks for opened PRs in the OTEL data the daemon forwards. It confirms them through the ShellTime GitHub App, so this needs the app connected in your ShellTime settings.

## Security and Privacy

- **Data masking** (`dataMasking`, on by default) redacts sensitive command content before it leaves your machine.
- **Exclusion patterns** skip matching commands entirely, so they are never recorded.
- **Encryption** (`encrypted: true`, set in the config `shelltime auth` creates) makes the daemon encrypt command uploads with your token's public key (RSA + AES-GCM). It only applies in daemon mode, and if the key can't be fetched the upload fails instead of going out unencrypted.
- **AI coding telemetry** includes prompts and tool details unless you turn them off (see [Claude Code and Codex Telemetry](#claude-code-and-codex-telemetry)). `--no-prompts` keeps prompt text out of a backfill.
- **Local config overrides** keep secrets like tokens out of your main config file.

## Development

Requires Go 1.27.1 (see `go.mod`). Common local commands:

```bash
go build -o shelltime ./cmd/cli/main.go
go build -o shelltime-daemon ./cmd/daemon/main.go
go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...
go fmt ./...
go vet ./...
mockery                                                   # regenerate mocks after interface changes
go -C perf test -run '^$' -bench . -benchtime 50x -count 5  # benchmark `shelltime track` latency
```

Contributor and agent guidance lives in [AGENTS.md](AGENTS.md); `perf/README.md` covers the latency benchmarks.

> **Note on naming:** the product is **ShellTime** (`shelltime.xyz`), but the Go module path is `github.com/malamtime/cli`. This mismatch is intentional — use `ShellTime` in product-facing docs and `github.com/malamtime/cli` for imports.

## Links

- [Configuration Guide](docs/CONFIG.md)
- [Claude Code Statusline Guide](docs/CC_STATUSLINE.md)
- [Dashboard](https://shelltime.xyz)
- [Issues](https://github.com/shelltime/cli/issues)

## License

Copyright (c) 2026 ShellTime Team. All rights reserved.
