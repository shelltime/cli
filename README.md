# ShellTime CLI

[![codecov](https://codecov.io/gh/shelltime/cli/graph/badge.svg?token=N09WIJHNI2)](https://codecov.io/gh/shelltime/cli)
[![shelltime](https://api.shelltime.xyz/badge/AnnatarHe/count)](https://shelltime.xyz/users/AnnatarHe)

ShellTime is a CLI and background daemon for tracking shell activity, syncing command history, and wiring AI coding tools into a shared telemetry stream. The public product and hosted service are ShellTime at [shelltime.xyz](https://shelltime.xyz).

The Go module path is `github.com/malamtime/cli`. That naming mismatch is intentional in this repo today; use `ShellTime` for product-facing docs and `github.com/malamtime/cli` for imports and module references.

## Install

```bash
curl -sSL https://shelltime.xyz/i | bash
```

## Quick Start

The fastest setup path is:

```bash
shelltime init
```

`shelltime init` authenticates the CLI, installs shell hooks, installs the daemon, and attempts to configure Claude Code and Codex OTEL integration.

If you prefer the manual flow:

```bash
shelltime auth
shelltime hooks install
shelltime daemon install
shelltime cc install
shelltime codex install
```

## What ShellTime Does

- Tracks shell commands locally with masking and exclusion support.
- Syncs command history to ShellTime for search and analysis.
- Runs a background daemon for low-latency, non-blocking sync.
- Integrates with Claude Code and OpenAI Codex through OTEL forwarding.
- Shows live Claude Code statusline data for cost, quota, time, and context.
- Syncs supported dotfiles to and from the ShellTime service.

## Command Overview

### Core setup and auth

| Command | Description |
|---------|-------------|
| `shelltime init` | Bootstrap auth, hooks, daemon, and AI-code integrations |
| `shelltime auth` | Authenticate with `shelltime.xyz` |
| `shelltime doctor` | Check installation and environment health |
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
| `shelltime cc install` | Install Claude Code OTEL shell configuration |
| `shelltime cc uninstall` | Remove Claude Code OTEL shell configuration |
| `shelltime cc statusline` | Emit statusline JSON for Claude Code |
| `shelltime codex install` | Add ShellTime OTEL config to `~/.codex/config.toml` |
| `shelltime codex uninstall` | Remove ShellTime OTEL config from `~/.codex/config.toml` |

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

For the full configuration surface, defaults, OTEL settings, and AI options, see [docs/CONFIG.md](docs/CONFIG.md).

## Daemon Mode

The daemon keeps tracking fast by buffering and syncing in the background.

| Mode | Latency | Network Blocking |
|------|---------|------------------|
| Direct | ~100ms+ | Yes |
| Daemon | <8ms | No |

Use daemon mode if you want lower shell latency, retry handling, and background processing for sync and OTEL events.

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

## Security and Privacy

- Data masking can redact sensitive command content before upload.
- Exclusion patterns let you skip matching commands entirely.
- Optional end-to-end encryption is available for supported flows.
- Local config overrides can keep sensitive values out of the primary config file.

## Development

Common local commands:

```bash
go build -o shelltime ./cmd/cli/main.go
go build -o shelltime-daemon ./cmd/daemon/main.go
go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...
go fmt ./...
go vet ./...
```

## Links

- [Configuration Guide](docs/CONFIG.md)
- [Claude Code Statusline Guide](docs/CC_STATUSLINE.md)
- [Dashboard](https://shelltime.xyz)
- [Issues](https://github.com/shelltime/cli/issues)

## License

Copyright (c) 2026 ShellTime Team. All rights reserved.
