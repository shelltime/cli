# ShellTime CLI Configuration Guide

This guide explains all configuration options available in ShellTime CLI. Configuration is straightforward yet powerful, allowing you to customize everything from data syncing to AI features.

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration Files](#configuration-files)
- [Core Settings](#core-settings)
- [Privacy & Security](#privacy--security)
- [Command Filtering](#command-filtering)
- [AI Features](#ai-features)
- [Claude Code Integration](#claude-code-integration)
- [Advanced Settings](#advanced-settings)
- [Complete Example](#complete-example)
- [FAQ](#faq)

---

## Quick Start

Create a configuration file at `~/.shelltime/config.yaml`:

```yaml
# Minimal configuration - just your API token
token: "your-api-token-from-shelltime.xyz"
```

That's it! ShellTime works with sensible defaults. Read on to customize your experience.

---

## Configuration Files

ShellTime uses two configuration files:

| File | Location | Purpose |
|------|----------|---------|
| `config.yaml` | `~/.shelltime/config.yaml` | Main configuration |
| `config.local.yaml` | `~/.shelltime/config.local.yaml` | Local overrides (for sensitive data) |

> **Note:** TOML format (`config.toml`, `config.local.toml`) is also supported. YAML files take priority when both exist.

### How Merging Works

Local config values **override** base config values. This lets you:
- Keep `config.yaml` in version control (without secrets)
- Store tokens and sensitive settings in `config.local.yaml` (add to `.gitignore`)

**Example:**

```yaml
# config.yaml
token: ""
flushCount: 5
dataMasking: true

# config.local.yaml
token: "my-secret-token"
flushCount: 10

# Result after merge:
# token: "my-secret-token"  (from local)
# flushCount: 10            (from local)
# dataMasking: true         (from base)
```

---

## Core Settings

### Authentication

| Option | Type | Required | Default |
|--------|------|----------|---------|
| `token` | string | Yes | - |
| `apiEndpoint` | string | No | `https://api.shelltime.xyz` |
| `webEndpoint` | string | No | `https://shelltime.xyz` |

```yaml
# Your API token from shelltime.xyz
token: "your-api-token"

# Custom API endpoint (for self-hosted instances)
apiEndpoint: "https://api.shelltime.xyz"

# Web dashboard URL
webEndpoint: "https://shelltime.xyz"
```

### Sync Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `flushCount` | integer | `10` | Commands buffered before syncing |
| `gcTime` | integer | `14` | Days to retain data on server |

```yaml
# Sync after every 10 commands (minimum: 3)
flushCount: 10

# Keep 2 weeks of history on server
gcTime: 14
```

**How syncing works:**
1. Commands are stored locally as you work
2. When `flushCount` commands accumulate, they're synced to the server
3. Daemon mode syncs instantly with <8ms latency
4. Direct mode syncs with ~100ms+ latency

### Daemon Socket

| Option | Type | Default |
|--------|------|---------|
| `socketPath` | string | `/tmp/shelltime.sock` |

```yaml
# Custom socket path for CLI-daemon communication
socketPath: "/tmp/shelltime.sock"
```

---

## Privacy & Security

### Data Masking

| Option | Type | Default |
|--------|------|---------|
| `dataMasking` | boolean | `true` |

Data masking automatically redacts sensitive information before syncing:

```yaml
# Enable automatic masking of sensitive data (recommended)
dataMasking: true
```

**What gets masked:**
- Environment variables (AWS keys, tokens, passwords)
- API keys and secrets
- Database connection strings
- SSH credentials
- Private keys

**Example:**
```bash
# Before masking:
export AWS_SECRET_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE

# After masking:
export AWS_SECRET_ACCESS_KEY=***MASKED***
```

### End-to-End Encryption

| Option | Type | Default | Requirements |
|--------|------|---------|--------------|
| `encrypted` | boolean | `false` | Daemon mode + token capability |

```yaml
# Enable E2E encryption (requires daemon mode)
encrypted: true
```

**Important:**
- Encryption only works when the daemon is running
- Your token must have encryption capability enabled
- Uses hybrid RSA/AES-GCM encryption

---

## Command Filtering

### Exclude Patterns

| Option | Type | Default |
|--------|------|---------|
| `exclude` | array of strings | `[]` |

Filter out commands you don't want tracked using regex patterns:

```yaml
exclude:
  - ".*password.*"        # Commands containing "password"
  - "^export AWS_"        # AWS credential exports
  - "^ssh.*root"          # SSH to root
  - "^gpg.*--decrypt"     # GPG decryption
  - "^history"            # History commands
  - "(?i)secret"          # Case-insensitive "secret"
  - "^mysql.*-p"          # MySQL with password flag
```

**Pattern syntax:** Uses Go's standard regex syntax (not PCRE). [Reference](https://pkg.go.dev/regexp/syntax)

**Tips:**
- Use `^` for start-of-command matching
- Use `(?i)` for case-insensitive matching
- Use `.*` for wildcards
- Invalid patterns are logged as warnings and skipped

---

## AI Features

ShellTime includes AI-powered command suggestions via `shelltime q`.

### AI Configuration

```yaml
ai:
  # Show helpful tips when using AI features
  showTips: true

  agent:
    # Auto-execute read-only commands (ls, cat, etc.)
    view: false

    # Auto-execute file modification commands
    edit: false

    # Auto-execute delete commands (DANGEROUS - not recommended)
    delete: false
```

### Auto-Execution Levels

| Level | Setting | Examples | Risk |
|-------|---------|----------|------|
| View | `ai.agent.view` | `ls`, `cat`, `grep` | Low |
| Edit | `ai.agent.edit` | `echo >>`, `sed -i` | Medium |
| Delete | `ai.agent.delete` | `rm`, `rmdir` | High |

**Recommended settings:**
```yaml
ai:
  agent:
    view: true    # Safe - only reads
    edit: false   # Requires confirmation
    delete: false # Always requires confirmation
```

---

## Claude Code Integration

ShellTime can track and forward Claude Code metrics for analysis.

### CCOtel (Recommended)

The modern approach using OpenTelemetry gRPC passthrough:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ccotel.enabled` | boolean | `false` | Enable OTEL collection |
| `ccotel.grpcPort` | integer | `54027` | gRPC server port |
| `ccotel.debug` | boolean | `false` | Write debug files |

```yaml
ccotel:
  enabled: true
  grpcPort: 54027   # Default ShellTime OTEL port
  debug: false      # Set true to debug issues
```

**How it works:**
1. Daemon starts gRPC server on configured port
2. Claude Code sends OTEL metrics/logs to this port
3. ShellTime forwards data to shelltime.xyz for analysis

### CCUsage (Legacy)

CLI-based collection (older method):

```yaml
ccusage:
  enabled: false
```

### Code Tracking

Track coding activity heartbeats:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `codeTracking.enabled` | boolean | `false` | Enable heartbeat tracking |
| `codeTracking.apiEndpoint` | string | - | Custom API endpoint for heartbeats |
| `codeTracking.token` | string | - | Custom token for heartbeats |

```yaml
codeTracking:
  enabled: true
  # Optional: use a custom API endpoint for heartbeats (defaults to global apiEndpoint)
  apiEndpoint: "https://api.custom-heartbeat.com"
  # Optional: use a custom token for heartbeats (defaults to global token)
  token: "custom-heartbeat-token"
```

When `apiEndpoint` or `token` is set under `codeTracking`, heartbeat data will use these values instead of the global configuration. This allows you to send coding activity to a different server or authenticate with a separate token.

---

## Advanced Settings

### Multiple Endpoints

Sync to multiple servers simultaneously:

```yaml
# Primary endpoint
token: "primary-token"
apiEndpoint: "https://api.shelltime.xyz"

# Additional endpoints (synced in parallel)
endpoints:
  - apiEndpoint: "https://backup-api.example.com"
    token: "backup-token"

  - apiEndpoint: "https://enterprise.internal.com"
    token: "enterprise-token"
```

### Log Cleanup

Automatic cleanup of log files:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `logCleanup.enabled` | boolean | `true` | Enable auto-cleanup |
| `logCleanup.thresholdMB` | integer | `100` | File size limit in MB |

```yaml
logCleanup:
  enabled: true
  thresholdMB: 100   # Clean files larger than 100MB
```

**Files cleaned:**
- `~/.shelltime/log.log`
- `~/.shelltime/heartbeat.log`
- `~/.shelltime/sync-pending.txt`
- `~/.shelltime/logs/shelltime-daemon.log` (macOS)
- `~/.shelltime/logs/shelltime-daemon.err` (macOS)

Cleanup runs every 24 hours when daemon is active.

### Metrics Collection

| Option | Type | Default |
|--------|------|---------|
| `enableMetrics` | boolean | `false` |

```yaml
# Enable OTEL metrics (has performance impact)
enableMetrics: false
```

**Warning:** Enabling metrics adds overhead to every command. Only use for debugging.

---

## Complete Example

Here's a full configuration with all options:

```yaml
# ============================================
# ShellTime CLI Configuration
# ============================================

# --- Authentication ---
token: "your-api-token"
apiEndpoint: "https://api.shelltime.xyz"
webEndpoint: "https://shelltime.xyz"

# --- Sync Settings ---
flushCount: 10        # Sync every 10 commands
gcTime: 14            # Keep 14 days of history

# --- Privacy ---
dataMasking: true     # Mask sensitive data
encrypted: false      # E2E encryption (requires daemon)

# --- Command Filtering ---
exclude:
  - ".*password.*"
  - "^export AWS_"
  - "^export.*SECRET"
  - "^ssh.*root"
  - "^gpg.*--decrypt"
  - "^history"

# --- AI Configuration ---
ai:
  showTips: true
  agent:
    view: true
    edit: false
    delete: false

# --- Claude Code Integration ---
ccotel:
  enabled: false
  grpcPort: 54027
  debug: false

ccusage:
  enabled: false

codeTracking:
  enabled: false
  # apiEndpoint: "https://api.custom-heartbeat.com"  # Optional: custom endpoint
  # token: "custom-heartbeat-token"                   # Optional: custom token

# --- Log Management ---
logCleanup:
  enabled: true
  thresholdMB: 100

# --- Advanced ---
socketPath: "/tmp/shelltime.sock"
enableMetrics: false

# --- Additional Sync Targets ---
# endpoints:
#   - apiEndpoint: "https://backup.example.com"
#     token: "backup-token"
```

---

## FAQ

### Where are configuration files stored?

All ShellTime data lives in `~/.shelltime/`:
```
~/.shelltime/
├── config.yaml          # Main configuration
├── config.local.yaml    # Local overrides (add to .gitignore)
├── log.log              # CLI logs
├── sync-pending.txt     # Pending sync data
└── logs/                # Daemon logs (macOS)
```

### How do I check my current configuration?

```bash
shelltime doctor
```

### Why isn't my local config being applied?

1. Ensure file is named exactly `config.local.yaml` (or `config.local.toml`)
2. Check YAML syntax (use a YAML validator)
3. Only non-empty values override base config

### How do I disable tracking temporarily?

Add a catch-all exclude pattern:
```yaml
exclude:
  - ".*"
```

Or unset your token:
```yaml
token: ""
```

### What's the difference between CCOtel and CCUsage?

| Feature | CCOtel | CCUsage |
|---------|--------|---------|
| Method | gRPC passthrough | CLI parsing |
| Performance | Better | More overhead |
| Data richness | Full OTEL data | Basic metrics |
| Recommended | Yes | Legacy |

### How do I test my exclude patterns?

Use this Go regex tester with your patterns:
```go
import "regexp"
pattern := regexp.MustCompile("your-pattern")
matched := pattern.MatchString("your-command")
```

Or test online at [regex101.com](https://regex101.com/) (select "Golang" flavor).

### Is daemon mode required?

No, but it's recommended:
- **With daemon:** <8ms latency, encryption support
- **Without daemon:** ~100ms+ latency, no encryption

Start the daemon:
```bash
shelltime-daemon
```

---

## Need Help?

- **Issues:** [github.com/shelltime/cli/issues](https://github.com/shelltime/cli/issues)
- **Documentation:** [shelltime.xyz/docs](https://shelltime.xyz/docs)
- **Status check:** `shelltime doctor`
