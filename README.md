# ShellTime CLI

[![codecov](https://codecov.io/gh/shelltime/cli/graph/badge.svg?token=N09WIJHNI2)](https://codecov.io/gh/shelltime/cli)
[![shelltime](https://api.shelltime.xyz/badge/AnnatarHe/count)](https://shelltime.xyz/users/AnnatarHe)

Track and analyze your DevOps workflow. [shelltime.xyz](https://shelltime.xyz)

## Install

```bash
curl -sSL https://shelltime.xyz/i | bash
```

## Setup

```bash
shelltime init              # Authenticate
shelltime hooks install     # Enable automatic tracking
shelltime daemon install    # Optional: background sync for <8ms latency
```

## Commands

| Command | Description |
|---------|-------------|
| `shelltime sync` | Sync pending commands to server |
| `shelltime q "prompt"` | AI-powered command suggestions |
| `shelltime doctor` | Diagnose installation issues |
| `shelltime web` | Open dashboard in browser |
| `shelltime gc` | Clean old tracking data |
| `shelltime ls` | List pending commands |

## Configuration

Config file: `~/.shelltime/config.toml`

```toml
token = "your-token"
flushCount = 10           # Commands before sync
gcTime = 14               # Days to retain data
dataMasking = true        # Mask sensitive data
encrypted = false         # E2E encryption (requires daemon)

# Exclude patterns (regex)
exclude = [".*password.*", "^export .*"]

# AI permissions
[ai.agent]
view = false              # Read-only commands
edit = false              # File modifications
delete = false            # Destructive commands
```

Local overrides: `~/.shelltime/config.local.toml`

## Why Daemon Mode?

| Mode | Latency | Network Blocking |
|------|---------|------------------|
| Direct | ~100ms+ | Yes |
| Daemon | <8ms | No |

The daemon handles network sync in the background with automatic retry and buffering.

## Security

- **Data Masking**: Sensitive info automatically redacted
- **E2E Encryption**: Hybrid RSA/AES-GCM encryption (v0.1.12+)
- **Exclusion Patterns**: Regex-based command filtering

## Links

- [Documentation](https://deepwiki.com/shelltime/cli)
- [Dashboard](https://shelltime.xyz)
- [Issues](https://github.com/shelltime/cli/issues)

## License

Copyright (c) 2025 ShellTime Team. All rights reserved.
