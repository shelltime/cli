# ShellTime CLI

[![codecov](https://codecov.io/gh/shelltime/cli/graph/badge.svg?token=N09WIJHNI2)](https://codecov.io/gh/shelltime/cli)
[![shelltime](https://api.shelltime.xyz/badge/AnnatarHe/count)](https://shelltime.xyz/users/AnnatarHe)

ShellTime is a CLI and background daemon that tracks your shell activity, syncs your command history, and pipes your AI coding tools into one shared telemetry stream. The hosted service lives at [shelltime.xyz](https://shelltime.xyz).

## Install

### Homebrew (macOS and Linux)

```bash
brew install shelltime/tap/shelltime
```

### curl installer

```bash
curl -sSL https://shelltime.xyz/i | bash
```

### Upgrading

If you installed with the curl script, upgrade in place:

```bash
shelltime update
```

If you installed with Homebrew, upgrade through brew instead:

```bash
brew upgrade shelltime/tap/shelltime
```

## Quick Start

The fastest way to get set up is a single command:

```bash
shelltime init
```

`shelltime init` authenticates the CLI, installs the shell hooks and daemon, and tries to wire up Claude Code and Codex OTEL integration for you.

Prefer to do it step by step?

```bash
shelltime auth
shelltime hooks install
shelltime daemon install
shelltime cc install
shelltime codex install
```

## What ShellTime Does

- Tracks shell commands locally, with masking and exclusion rules to keep secrets out.
- Syncs your command history to ShellTime so you can search and analyze it.
- Runs a background daemon for low-latency, non-blocking sync.
- Forwards Claude Code and OpenAI Codex telemetry through OTEL.
- Syncs the rate-limit windows and extra-credit status that Codex currently reports.
- Shows a live Claude Code statusline with cost, quota, time, and context usage.
- Syncs supported dotfiles to and from the ShellTime service.

## Command Overview

### Core setup and auth

| Command | Description |
|---------|-------------|
| `shelltime init` | Bootstrap auth, hooks, daemon, and AI-code integrations |
| `shelltime auth` | Authenticate with `shelltime.xyz` |
| `shelltime update` | Download and install the latest release in place |
| `shelltime doctor` | Diagnose setup problems and show how to fix each one (`--fix` applies the safe fixes, `--offline`, `--format json`) |
| `shelltime web` | Open the ShellTime dashboard in a browser |

### Tracking and sync

| Command | Description |
|---------|-------------|
| `shelltime track` | Record a shell command event |
| `shelltime sync` | Manually sync pending local data |
| `shelltime ls` | List locally saved commands |
| `shelltime gc` | Clean internal storage and logs |
| `shelltime rg "pattern"` | Search synced command history |

### AI helpers and integrations

| Command | Description |
|---------|-------------|
| `shelltime query "prompt"` | Ask AI for a suggested shell command |
| `shelltime q "prompt"` | Alias for `shelltime query` |
| `shelltime cc install` | Install Claude Code OTEL configuration into `~/.claude/settings.json` |
| `shelltime cc uninstall` | Remove Claude Code OTEL configuration from `~/.claude/settings.json` |
| `shelltime cc statusline` | Emit statusline JSON for Claude Code |
| `shelltime cc backfill` | Upload past Claude Code usage from local transcripts |
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
| `shelltime alias import` | Import aliases from shell config files |
| `shelltime config view` | Show the merged current configuration |
| `shelltime schema` | Generate JSON schema for config autocompletion |
| `shelltime ios dl` | Open the ShellTime iOS App Store page |

### Dotfiles

| Command | Description |
|---------|-------------|
| `shelltime dotfiles push` | Push supported dotfiles to the server |
| `shelltime dotfiles pull` | Pull supported dotfiles to local config |

## Configuration

ShellTime stores data under `~/.shelltime/`.

- Main config: `~/.shelltime/config.yaml`
- Local overrides: `~/.shelltime/config.local.yaml`
- Also supported: `config.yml`, `config.toml`, `config.local.yml`, `config.local.toml`
- Generated schema: `~/.shelltime/config-schema.json`
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

Run in daemon mode for lower shell latency, automatic sync retries, and background processing of sync and OTEL events. It is optional but recommended.

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

## Codex Usage Tracking

ShellTime receives Codex data through two independent paths:

- `shelltime codex install` configures Codex OTEL export for sessions, tokens, tool activity, and cost telemetry.
- The running `shelltime-daemon` reads your local Codex login, fetches the rate-limit windows and credit status currently returned by Codex, and syncs that summary when the daemon starts and every 10 minutes afterward.

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

## Security and Privacy

- **Data masking** redacts sensitive command content before it leaves your machine.
- **Exclusion patterns** skip matching commands entirely, so they are never recorded.
- **End-to-end encryption** is available for supported flows (opt-in).
- **Local config overrides** keep secrets like tokens out of your main config file.

## Development

Common local commands:

```bash
go build -o shelltime ./cmd/cli/main.go
go build -o shelltime-daemon ./cmd/daemon/main.go
go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...
go fmt ./...
go vet ./...
```

> **Note on naming:** the product is **ShellTime** (`shelltime.xyz`), but the Go module path is `github.com/malamtime/cli`. This mismatch is intentional — use `ShellTime` in product-facing docs and `github.com/malamtime/cli` for imports.

## Links

- [Configuration Guide](docs/CONFIG.md)
- [Claude Code Statusline Guide](docs/CC_STATUSLINE.md)
- [Dashboard](https://shelltime.xyz)
- [Issues](https://github.com/shelltime/cli/issues)

## License

Copyright (c) 2026 ShellTime Team. All rights reserved.
