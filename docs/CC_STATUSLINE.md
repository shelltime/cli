# Claude Code Statusline Integration

Display real-time cost and context usage in Claude Code's status bar using ShellTime.

## Overview

The `shelltime cc statusline` command provides a custom status line for Claude Code that shows:

- Git branch name and dirty status
- Current model name
- Session cost (current conversation)
- Today's total cost (from ShellTime API)
- AI agent time (session duration)
- Context window usage percentage
- Anthropic API quota utilization (5-hour and 7-day windows)

## Quick Start

### 1. Configure Claude Code

Add to your Claude Code settings (`~/.claude/settings.json`):

```json
{
  "statusLine": {
    "type": "command",
    "command": "shelltime cc statusline"
  }
}
```

### 2. That's It!

The status line will appear at the bottom of Claude Code:

```
🌿 main* | 🤖 Opus | 💰 $0.12 | 📊 $3.45 | 🚦 5h:23% 7d:12% | ⏱️ 5m30s | 📈 45%
```

---

## Output Format

| Section | Emoji | Description | Color |
|---------|-------|-------------|-------|
| Git | 🌿 | Current branch name (`*` if dirty) | Green |
| Model | 🤖 | Current model display name | Default |
| Session | 💰 | Current session cost in USD | Cyan |
| Today | 📊 | Today's total cost from API | Yellow |
| Quota | 🚦 | Anthropic API quota utilization | Green/Yellow/Red |
| Time | ⏱️ | AI agent session duration | Magenta |
| Context | 📈 | Context window usage % | Green/Yellow/Red |

### Git Status Indicator

The git section shows the current branch name. If there are uncommitted changes (dirty working tree), an asterisk (`*`) is appended:

- `🌿 main` - On main branch, no uncommitted changes
- `🌿 main*` - On main branch, with uncommitted changes
- `🌿 feature/auth` - On feature branch
- `🌿 -` - Not in a git repository

### Quota Utilization

The quota section displays your Anthropic API rate limit utilization across two time windows:

- `🚦 5h:23% 7d:12%` - 23% of 5-hour quota used, 12% of 7-day quota used
- `🚦 -` - Quota data unavailable (macOS only, requires Claude Code OAuth token)

The percentage is clickable and links to your [Claude usage settings](https://claude.ai/settings/usage) page.

### Quota Color Coding

Color is based on the **maximum** utilization across both windows:

| Usage | Color | Meaning |
|-------|-------|---------|
| < 50% | Green | Normal usage |
| 50-80% | Yellow | Approaching quota limit |
| > 80% | Red | Near quota exhaustion |

### Context Color Coding

| Usage | Color | Meaning |
|-------|-------|---------|
| < 50% | Green | Plenty of context remaining |
| 50-80% | Yellow | Context getting full |
| > 80% | Red | Context nearly exhausted |

---

## How It Works

1. **Claude Code** passes session data as JSON via stdin
2. **shelltime cc statusline** parses the JSON and extracts:
   - Model name from `model.display_name`
   - Session cost from `cost.total_cost_usd`
   - Context usage from `context_window`
   - Working directory from `working_directory`
3. **Git info** is fetched from the daemon (which caches it for performance)
4. **Daily cost** is fetched from ShellTime GraphQL API (cached for 5 minutes)
5. **Quota utilization** is fetched from Anthropic OAuth API by the daemon (cached for 10 minutes)
6. **Output** is a single formatted line with ANSI colors

### JSON Input (from Claude Code)

```json
{
  "model": {
    "id": "claude-opus-4-1",
    "display_name": "Opus"
  },
  "cost": {
    "total_cost_usd": 0.12,
    "total_duration_ms": 45000
  },
  "context_window": {
    "total_input_tokens": 15234,
    "total_output_tokens": 4521,
    "context_window_size": 200000,
    "current_usage": {
      "input_tokens": 8500,
      "output_tokens": 1200,
      "cache_creation_input_tokens": 5000,
      "cache_read_input_tokens": 2000
    }
  },
  "working_directory": "/home/user/projects/my-app"
}
```

---

## Requirements

### For Session Cost & Context

No additional setup required - data comes directly from Claude Code.

### For Git Branch Info

Requires the ShellTime daemon to be running:

```bash
# Start the daemon
shelltime daemon start

# Verify it's running
shelltime daemon status
```

The daemon caches git info and refreshes it periodically for optimal performance.

### For Today's Cost

Requires ShellTime configuration:

```yaml
# ~/.shelltime/config.yaml
token: "your-api-token"
apiEndpoint: "https://api.shelltime.xyz"

# Enable OTEL collection to track costs
aiCodeOtel:
  enabled: true
```

If no token is configured, the daily cost will show as `-`.

### For Quota Utilization

Requires **macOS** and the ShellTime daemon running. The daemon reads Claude Code's OAuth token from the macOS Keychain (service name: `Claude Code-credentials`) and queries the Anthropic usage API.

- **macOS only** - Keychain access is required to retrieve the OAuth token
- **Daemon required** - quota data is fetched and cached by the daemon's background timer
- **No manual setup** - if you're logged into Claude Code on macOS, it works automatically

If quota data is unavailable, the section will show as `🚦 -`.

---

## Performance

- **Hard timeout:** 100ms for entire operation
- **API caching:** 5-minute TTL for daily cost, 10-minute TTL for quota utilization
- **Git info caching:** Daemon fetches git info in background timer loop, not on-demand
- **Quota caching:** Daemon fetches quota data asynchronously with rate-limit protection
- **Non-blocking:** Background fetches don't delay output
- **Graceful degradation:** Shows available data even if API or daemon fails

---

## Troubleshooting

### Status line not appearing

1. Check Claude Code settings path: `~/.claude/settings.json`
2. Verify shelltime is in your PATH: `which shelltime`
3. Test manually: `echo '{}' | shelltime cc statusline`

### Daily cost shows `-`

1. Verify your token is configured: `shelltime doctor`
2. Check AICodeOtel is enabled in your config
3. Ensure the daemon is running: `shelltime daemon status`

### Git branch shows `-`

1. Ensure you're in a git repository
2. Verify the daemon is running: `shelltime daemon status`
3. The daemon must be running for git info to appear (direct API fallback doesn't include git info)

### Quota shows `-`

1. Ensure you're on **macOS** - quota display is only available on macOS
2. Verify you're logged into Claude Code (the OAuth token is stored in macOS Keychain)
3. Ensure the daemon is running: `shelltime daemon status`
4. Quota data is cached for 10 minutes - it may take a moment after daemon start

### Colors not displaying

Your terminal may not support ANSI colors. Check terminal settings or try a different terminal emulator.

---

## Related

- [Configuration Guide](./CONFIG.md) - Full configuration reference
- [Claude Code Integration](./CONFIG.md#claude-code-integration) - AICodeOtel setup
