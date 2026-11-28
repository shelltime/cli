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

`shelltime init` authenticates the CLI (in the browser, with `--email` to approve on your iPhone in the ShellTime iOS app, or with `--token`), installs the shell hooks and the daemon, and tries to wire up Claude Code and Codex OTEL integration for you. Cursor and Antigravity take their hooks in each project's repository, so it only says how to add them when those are installed. It prints one line per step; `shelltime init --verbose` shows each step's details.

Shell hooks go to the shells you use: zsh, bash or fish when its config file (`~/.zshrc`, `~/.bashrc`, `~/.config/fish/config.fish`) exists, and your login shell (`$SHELL`), whose config file is created if missing. To add a shell later, create its config file and run `shelltime hooks install`.

Prefer to do it step by step?

```bash
shelltime auth
shelltime hooks install
shelltime daemon install
shelltime cc install
shelltime codex install
shelltime cursor install   # in each repository you use Cursor in
shelltime agy install      # in each repository you use Google Antigravity in
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
| `shelltime auth` | Authenticate with `shelltime.xyz` (`--token` skips the browser flow; `--email you@example.com` asks the ShellTime iOS app to approve instead, with no browser, so agents can run it) |
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
| `shelltime cc projects` | Group this machine's coding agent projects by git repository instead of folder (`--dry-run` lists what would move). The daemon does this once on its own; see [Projects](#projects) |
| `shelltime codex install` | Add ShellTime OTEL config to `~/.codex/config.toml` |
| `shelltime codex uninstall` | Remove ShellTime OTEL config from `~/.codex/config.toml` |
| `shelltime codex backfill` | Upload past Codex usage from local session files |
| `shelltime codex projects` | Same as `shelltime cc projects`: the projects of both agents are grouped together |
| `shelltime cursor install` | Add the ShellTime hooks to the current repository's `.cursor/hooks.json` (Cursor IDE, `cursor-agent` and Cloud Agents) |
| `shelltime cursor uninstall` | Remove the ShellTime hooks from the current repository's `.cursor/hooks.json` and `~/.cursor/hooks.json` |
| `shelltime agy install` | Add the ShellTime hook group to the current repository's `.agents/hooks.json` (Antigravity app, `agy` CLI and the Antigravity agent's Gemini API sandbox); `shelltime antigravity` works too |
| `shelltime agy uninstall` | Remove the ShellTime hook group from the current repository's `.agents/hooks.json` and `~/.gemini/config/hooks.json` |

### Environment helpers

| Command | Description |
|---------|-------------|
| `shelltime hooks install` | Install shell hooks |
| `shelltime hooks uninstall` | Remove shell hooks |
| `shelltime daemon install` | Install the ShellTime daemon service |
| `shelltime daemon status` | Check daemon status |
| `shelltime daemon reinstall` | Reinstall and restart the daemon service (alias: `shelltime daemon restart`) |
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

## Cursor and Antigravity Hooks

Cursor and Google Antigravity have no OpenTelemetry export, so ShellTime reads their native hooks instead: the agent runs `shelltime hook-ingest` for each event with the event's JSON on stdin, and ShellTime answers with the reply that lets the agent carry on. It never blocks, denies or changes anything the agent does.

The hooks go into each project's repository, the only place the agents' cloud runs read them from (Cursor Cloud Agents never read `~/.cursor/hooks.json`, and Antigravity runs a workspace's hooks ahead of its global file). Run the install in the repository, then commit the file:

- **Cursor**: `shelltime cursor install` registers `beforeSubmitPrompt`, `postToolUse`, `postToolUseFailure`, `afterAgentResponse`, `stop`, `sessionStart` and `sessionEnd` in `.cursor/hooks.json` at the repository's root. Your other hooks stay as they are.
- **Antigravity**: `shelltime agy install` adds a `shelltime` hook group (`SessionStart`, `PreInvocation`, `PostToolUse`, `PostInvocation`, `Stop`) to `.agents/hooks.json` at the repository's root. Antigravity ignores the whole file when it isn't strict JSON, so a file with comments, trailing commas or a byte order mark is left alone with an error, never rewritten.

Outside a repository the current folder counts as the project, but never your home folder. Hooks older versions put in `~/.cursor/hooks.json` or `~/.gemini/config/hooks.json` stay and keep tracking your other folders; a repository's own hooks leave events to them, so nothing is counted twice. `uninstall` takes ShellTime out of the repository's file and those. Restart the agent after installing. The hooks need macOS or Linux: `shelltime-daemon` doesn't run on Windows, and the install refuses there (a committed hook file fails on a Windows collaborator's machine, which Cursor logs and carries on from).

Each hook is one self-contained shell command, the same on every machine that checks the repository out: it records the event only where `shelltime` and `shelltime-daemon` are installed (on `PATH`, or in `~/.shelltime/bin`, `/opt/homebrew/bin` or `/usr/local/bin`, which an app started from the Dock may not have on its `PATH`), each to that machine's ShellTime account, and anywhere else just lets the agent carry on, so collaborators without ShellTime see no failing hooks. Events go to the daemon, which sends them in batches; while it isn't running they wait in `~/.shelltime/agent-hooks.pending.jsonl` until it sends them. The hooks send your prompts, the agent's responses, tool names and inputs (shell commands, file paths), per-turn token counts and the workspace folder; tool output is never read. Data masking and your exclude patterns apply (an event whose shell command matches one is sent without the command, prompt, response, tool input or error), and `shelltime config disable agentHooks.sendContent` keeps the text of prompts, responses, tool input and errors, and the paths of files attached to prompts, out, so only the activity and token usage are sent. `shelltime doctor`, run in a repository, checks its hook file and that the hooks can find both binaries.

Token usage (used to calculate estimated USD cost on the server) is captured per turn: Cursor reports token counts directly on its `stop` hook payload, while Google Antigravity's `PostInvocation` hook reads completed model steps from the local `.jsonl` transcript file referenced by `transcriptPath` (when the file is missing, unreadable, or has no matching completed steps, activity is still recorded without token counts).

### Cloud agents

In a Cursor Cloud Agent's environment, install ShellTime, sign it in (`shelltime auth --token …`), set `SHELLTIME_CLOUD=true` in the environment's variables or secrets, and keep `shelltime-daemon` running, started with that variable set: the daemon reads it once, when it starts (or put `aiCloudOnly: true` in its config). The committed hooks do the rest. Cursor marks a cloud run only in `sessionStart` (`is_background_agent`), which the daemon passes on to the session's later events, so the variable is what marks every event. The Antigravity agent's Gemini API sandbox needs the same setup (its environment takes variables too); its hooks are marked as cloud by the sandbox's OS user, `antigravity`, even without the variable. With the variable the machine collects AI coding agent usage only (see `aiCloudOnly` in [CONFIG.md](docs/CONFIG.md)). Claude Code cloud environments (`CLAUDE_CODE_REMOTE=true`) count as cloud too.

## Projects

ShellTime groups your coding agent sessions into projects by the git repository they ran in, so a renamed or moved clone, a worktree or a subfolder stays one project. The repository comes from the folder's `origin` remote (else `upstream`, else the first remote), sent as `https://<host>/<path>` with any credentials in the remote URL removed. Claude Code reports it itself (`shelltime cc install` turns this on); for Codex and older Claude Code setups the daemon asks git about the session's folder. Sessions outside a repository, or in one without a remote, are grouped by machine and folder.

Sessions recorded before this were grouped by folder. A couple of minutes after it starts, the daemon moves this machine's folder projects whose folders still exist and are clones into their repository's project, once, and leaves `~/.shelltime/project-repositories.v1` behind. `shelltime cc projects` does the same by hand (`--dry-run` shows the folders and repositories without changing anything). Folders that no longer exist (renamed or deleted) can't be checked, and neither can a folder on a drive that isn't mounted, so run `shelltime cc projects` again once it is, and host names many machines share, like `localhost`, are skipped. A folder moves whole, into the repository it is a clone of today, so only folders where no session ever named a repository are moved this way: the server keys the others from the sessions that did, each session from the closest one before it.

To keep repository names to yourself, set `sendRepository: false` under `aiCodeOtel` in `~/.shelltime/config.yaml`. The daemon then never asks git, removes the repository Claude Code reports, and skips the migration; sessions are grouped by machine and folder. Restart the daemon (`shelltime daemon reinstall`) for a change to take effect.

## Codex Usage Tracking

ShellTime receives Codex data through two independent paths:

- `shelltime codex install` configures Codex OTEL export for sessions, tokens, tool activity, and cost telemetry.
- The running `shelltime-daemon` reads your local Codex login, fetches the rate-limit windows and credit status currently returned by Codex, and syncs that summary every 10 minutes while a Codex process is running.

Quota sync requires both a ShellTime login (`shelltime auth`) and a ChatGPT-authenticated Codex installation. ShellTime reads the Codex access token from `~/.codex/auth.json` only for the direct request to Codex; the token stays on your machine, and only the returned plan, quota windows, percentages, reset times, and credit summary are sent to ShellTime.

Codex decides which windows are present. ShellTime displays the windows returned by Codex instead of assuming that every account has a fixed 5-hour window.

## Backfilling AI Usage

Live tracking records sessions while OTEL or daemon sync is running. To upload historical usage for Claude Code, Codex, Cursor, or Google Antigravity:

```bash
shelltime cc backfill --dry-run      # show what would be uploaded
shelltime cc backfill                # upload Claude Code sessions
shelltime codex backfill             # upload Codex sessions
shelltime cursor backfill            # upload Cursor token & cost usage
shelltime agy backfill               # upload Antigravity token usage
```

- Claude Code transcripts are read from `~/.claude/projects` and `~/.config/claude/projects`, or `CLAUDE_CONFIG_DIR`. Codex sessions are read from `~/.codex/sessions` and `~/.codex/archived_sessions`, or `CODEX_HOME`.
- Cursor usage is fetched from Cursor's `GetFilteredUsageEvents` API using `CURSOR_AUTH_TOKEN`, `CURSOR_API_KEY`, `cursor.apiKey` in config, `~/.cursor/auth.json`, or Cursor IDE's local `state.vscdb`. The daemon also syncs Cursor subscription quota (`GetCurrentPeriodUsage`) and recent token usage every 10 minutes while Cursor is running.
- Antigravity usage is parsed from local conversation SQLite databases under `~/.gemini/antigravity*` and `~/.config/antigravity` (or `ANTIGRAVITY_DATA_DIR`). The daemon also scans modified conversation databases every 10 minutes while Antigravity (`agy`) is running.
- Prompts, token usage, models and tool calls are uploaded as if they had been tracked live; the server fills in costs when the agent did not report one.
- Sessions the server already has from live tracking are skipped, as are sessions still running. Running the command again only uploads what is missing.
- Flags: `--since` / `--until` (`YYYY-MM-DD`) limit the range, `--no-prompts` uploads prompt lengths without the text, and `--ai-summary` also generates AI session summaries, which use your monthly AI credits.
- Claude Code deletes transcripts after 30 days by default (`cleanupPeriodDays`), so only recent history may be available.

## Linking Pull Requests to AI Sessions

The [ShellTime Claude Code mod](https://github.com/shelltime/claude-code-mods) watches for `gh pr create` in Claude Code's Bash tool. When it sees one, it runs:

```bash
shelltime cc pr --session-id <claude-code-session-id> https://github.com/owner/repo/pull/123 [more URLs...]
```

The command hands the URLs to the daemon, and the daemon sends them to ShellTime, where they appear on the session. If no daemon is running, the CLI sends them itself. A session can link any number of PRs. Sending the same URL again does nothing. The command does nothing if you are not logged in.

For **Cursor** and **Google Antigravity**, `shelltime hook-ingest` watches successful `gh pr create` and GitHub MCP `create_pull_request` calls (`postToolUse` / `PostToolUse`, reading the Antigravity `transcript.jsonl` step when present) and extracts only the created pull request URLs — raw tool output is never stored or uploaded — so the daemon links them via `/api/v1/cc/session-pull-requests`.

For Codex, Claude Code sessions without the mod, and Cursor or Antigravity calls whose output was not visible locally, the ShellTime server also looks for opened PRs in the telemetry the daemon forwards. It confirms them through the ShellTime GitHub App, so this needs the app connected in your ShellTime settings.

## Security and Privacy

- **Data masking** (`dataMasking`, on by default) redacts sensitive command content before it leaves your machine.
- **Exclusion patterns** skip matching commands entirely, so they are never recorded.
- **Encryption** (`encrypted: true`, set in the config `shelltime auth` creates) makes the daemon encrypt command uploads with your token's public key (RSA + AES-GCM). It only applies in daemon mode. If the key can't be fetched, is empty, or encryption fails, the upload fails instead of going out unencrypted or with a key the server cannot unwrap.
- **`shelltime auth`** writes `~/.shelltime` as mode `0700` and `config.yaml` as mode `0600`.
- **AI coding telemetry** includes prompts and tool details unless you turn them off (see [Claude Code and Codex Telemetry](#claude-code-and-codex-telemetry) and [Cursor and Antigravity Hooks](#cursor-and-antigravity-hooks)). `--no-prompts` keeps prompt text out of a backfill.
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

[MIT](LICENSE). Copyright (c) 2026 ShellTime Team.
