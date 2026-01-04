# Claude Code Statusline Integration

Display real-time cost and context usage in Claude Code's status bar using ShellTime.

## Overview

The `shelltime cc statusline` command provides a custom status line for Claude Code that shows:

- Current model name
- Session cost (current conversation)
- Today's total cost (from ShellTime API)
- Context window usage percentage

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
🤖 Opus | 💰 $0.12 | 📊 $3.45 | 📈 45%
```

---

## Output Format

| Section | Emoji | Description | Color |
|---------|-------|-------------|-------|
| Model | 🤖 | Current model display name | Default |
| Session | 💰 | Current session cost in USD | Cyan |
| Today | 📊 | Today's total cost from API | Yellow |
| Context | 📈 | Context window usage % | Green/Yellow/Red |

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
3. **Daily cost** is fetched from ShellTime GraphQL API (cached for 5 minutes)
4. **Output** is a single formatted line with ANSI colors

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
  }
}
```

---

## Requirements

### For Session Cost & Context

No additional setup required - data comes directly from Claude Code.

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

---

## Performance

- **Hard timeout:** 100ms for entire operation
- **API caching:** 5-minute TTL to minimize API calls
- **Non-blocking:** Background API fetches don't delay output
- **Graceful degradation:** Shows available data even if API fails

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

### Colors not displaying

Your terminal may not support ANSI colors. Check terminal settings or try a different terminal emulator.

---

## Related

- [Configuration Guide](./CONFIG.md) - Full configuration reference
- [Claude Code Integration](./CONFIG.md#claude-code-integration) - AICodeOtel setup
