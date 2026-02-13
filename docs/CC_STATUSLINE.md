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
- Anthropic API quota utilization (5-hour and 7-day windows, macOS only)

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

| Section | Emoji | Description | Color | Clickable Link |
|---------|-------|-------------|-------|----------------|
| Git | 🌿 | Current branch name (`*` if dirty) | Green (Gray if unavailable) | — |
| Model | 🤖 | Current model display name | Default | — |
| Session | 💰 | Current session cost in USD | Cyan | Session detail page |
| Daily | 📊 | Today's total cost from API | Yellow when > 0, Gray when 0 | Coding agent page |
| Quota | 🚦 | Anthropic API quota utilization | Green/Yellow/Red (Gray if unavailable) | Claude usage settings (always linked) |
| Time | ⏱️ | AI agent session duration | Magenta when > 0, Gray when 0 | User profile page |
| Context | 📈 | Context window usage % | Green/Yellow/Red | — |

### Clickable Links

All cost and usage sections support terminal hyperlinks using the [OSC 8 protocol](https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda). Click on the section text in a supported terminal to open the corresponding page:

| Section | Link Target |
|---------|-------------|
| 💰 Session Cost | `{webEndpoint}/users/{login}/coding-agent/session/{sessionID}` |
| 📊 Daily Cost | `{webEndpoint}/users/{login}/coding-agent/claude-code` |
| 🚦 Quota | `https://claude.ai/settings/usage` (always linked, even when showing `-`) |
| ⏱️ Time | `{webEndpoint}/users/{login}` (user profile) |

Session and Daily links require a configured ShellTime account (`userLogin` and `webEndpoint` from daemon). The Quota link is always active regardless of data availability.

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

### Platform Note

On **Linux**, the quota section (`🚦`) is omitted entirely from the output — the statusline skips from Daily Cost to Time. On **macOS**, it is always shown (either with data or as `🚦 -`).

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
   - Working directory from `cwd`
   - Session ID from `session_id` (used for session cost link)
   - Workspace info from `workspace` (used for session-project mapping)
3. **Session-project mapping** is sent to the daemon as a fire-and-forget message (~1ms), associating the session ID with its project directory
4. **Git info** is fetched from the daemon (which caches it for performance)
5. **Daily cost** is fetched from ShellTime GraphQL API (cached for 5 minutes)
6. **Quota utilization** is fetched from Anthropic OAuth API by the daemon (cached for 10 minutes)
7. **Clickable links** are added to Session, Daily, Quota, and Time sections using OSC 8 terminal hyperlinks
8. **Output** is a single formatted line with ANSI colors

### JSON Input (from Claude Code)

```json
{
  "hook_event_name": "StatusLine",
  "session_id": "abc123-def456",
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
  "cwd": "/home/user/projects/my-app",
  "version": "1.0.0",
  "workspace": {
    "current_dir": "/home/user/projects/my-app",
    "project_dir": "/home/user/projects/my-app"
  }
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

- **macOS only** - Keychain access is required to retrieve the OAuth token; on Linux the quota section is omitted entirely
- **Daemon required** - quota data is fetched and cached by the daemon's background timer
- **No manual setup** - if you're logged into Claude Code on macOS, it works automatically

If quota data is unavailable, the section will show as `🚦 -`.

---

## Performance

- **Hard timeout:** 100ms for entire operation
- **Daemon request timeout:** 50ms for the daemon socket request (fast path)
- **Session mapping:** ~1ms fire-and-forget to daemon
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

1. Ensure you're on **macOS** - quota display is only available on macOS (omitted entirely on Linux)
2. Verify you're logged into Claude Code (the OAuth token is stored in macOS Keychain)
3. Ensure the daemon is running: `shelltime daemon status`
4. Quota data is cached for 10 minutes - it may take a moment after daemon start

### Colors not displaying

Your terminal may not support ANSI colors. Check terminal settings or try a different terminal emulator.

### Links not clickable

Your terminal must support the [OSC 8 hyperlink protocol](https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda). Most modern terminals (iTerm2, WezTerm, Windows Terminal, GNOME Terminal 3.26+) support it. Older terminals or multiplexers (tmux, screen) may not.

---

## Related

- [Configuration Guide](./CONFIG.md) - Full configuration reference
- [Claude Code Integration](./CONFIG.md#claude-code-integration) - AICodeOtel setup
