# ShellTime CLI [![codecov](https://codecov.io/gh/malamtime/cli/graph/badge.svg?token=N09WIJHNI2)](https://codecov.io/gh/malamtime/cli)

deepwiki: [https://deepwiki.com/shelltime/cli]

The CLI tool for shelltime.xyz - a platform for tracking DevOps work.

AnnatarHe: [![shelltime](https://api.shelltime.xyz/badge/AnnatarHe/count)](https://shelltime.xyz/users/AnnatarHe)

## Installation

```bash
curl -sSL https://raw.githubusercontent.com/malamtime/installation/master/install.bash | bash
```

## Configuration

The CLI stores its configuration in `$HOME/.shelltime/config.toml`.

### Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token` | string | `""` | Your authentication token for shelltime.xyz |
| `apiEndpoint` | string | `"https://api.shelltime.xyz"` | The API endpoint URL for shelltime.xyz |
| `webEndpoint` | string | `"https://shelltime.xyz"` | The web interface URL for shelltime.xyz |
| `flushCount` | integer | `10` | Number of records to accumulate before syncing to server |
| `gcTime` | integer | `14` | Number of days to keep tracked data before garbage collection |
| `dataMasking` | boolean | `true` | Enable/disable masking of sensitive data in tracked commands |
| `enableMetrics` | boolean | `false` | Enable detailed command metrics tracking (WARNING: May impact performance) |
| `encrypted` | boolean | `false` | Enable end-to-end encryption for command data (requires daemon mode) |
| `exclude` | array | `[]` | List of regular expressions to exclude commands from tracking |
| `endpoints` | array | `[]` | Additional API endpoints for development or testing |
| `ai.agent.view` | boolean | `false` | Allow AI to auto-execute read-only commands (e.g., ls, cat, less, head, tail) |
| `ai.agent.edit` | boolean | `false` | Allow AI to auto-execute file editing commands (e.g., vim, nano, code, sed) |
| `ai.agent.delete` | boolean | `false` | Allow AI to auto-execute deletion commands (e.g., rm, rmdir, unlink) |
| `ccusage.enabled` | boolean | `false` | Enable Claude Code usage tracking and analytics |

### Complete Configuration Example

```toml
# Authentication token from shelltime.xyz
token = "your-token-here"

# API and web endpoints
apiEndpoint = "https://api.shelltime.xyz"
webEndpoint = "https://shelltime.xyz"

# Sync settings
flushCount = 10        # Sync after 10 commands (increase for less frequent syncs)
gcTime = 14           # Keep local data for 14 days

# Privacy and security
dataMasking = true    # Mask sensitive data in commands
encrypted = false     # Enable E2E encryption (requires daemon mode and special token)

# Performance monitoring
enableMetrics = false # WARNING: Impacts performance, only for debugging

# Command exclusion patterns (regular expressions)
# Commands matching these patterns won't be tracked
exclude = [
    "^ls .*",         # Exclude all ls commands
    ".*password.*",   # Exclude commands containing "password"
    "^git push",      # Exclude git push commands
]

# AI configuration (optional)
# Controls which command types the AI assistant can automatically execute
[ai.agent]
view = false    # Allow AI to execute read-only commands (ls, cat, less, head, tail, etc.)
edit = false    # Allow AI to execute file editing commands (vim, nano, code, sed, etc.)
delete = false  # Allow AI to execute deletion commands (rm, rmdir, unlink, etc.)

# Claude Code usage tracking (optional)
[ccusage]
enabled = false # Track and report Claude Code usage statistics

# Development endpoints (optional, for testing)
# [[endpoints]]
# apiEndpoint = "http://localhost:8080"
# token = "dev-token"
```

### Configuration Notes

⚠️ **Performance Warning**: Setting `enableMetrics` to `true` will track detailed metrics for every command execution. Only enable this when requested by developers for debugging purposes, as it may significantly impact shell performance.

🔒 **Encryption**: End-to-end encryption requires:
- Daemon mode to be installed and running
- A special encryption-enabled token (request from shelltime.xyz)
- Setting `encrypted = true` in the configuration

🤖 **AI Command Execution**: The AI agent configuration controls which types of commands the AI assistant is allowed to automatically execute:
- `view = true`: AI can execute read-only commands that don't modify the system (e.g., ls, cat, grep)
- `edit = true`: AI can execute commands that modify files (e.g., vim, sed, echo > file)
- `delete = true`: AI can execute commands that delete files or directories (e.g., rm, rmdir, unlink)

Set these to `false` (default) to prevent AI from automatically executing those command types. This provides granular control over AI permissions for safety.

🚫 **Exclusion Patterns**: Use regular expressions to exclude sensitive or high-frequency commands from being tracked. This helps maintain privacy and reduce unnecessary data transmission.

📊 **Claude Code Usage**: When enabled, tracks your Claude Code usage patterns to help improve your development workflow analytics.

## Commands

### Core Commands

#### `shelltime init` (Authentication)

Initializes the CLI with your shelltime.xyz authentication token. This must be run before using other features.

```bash
shelltime init [--token <your-token>]
```

**Options:**
- `--token, -t`: Your personal access token from shelltime.xyz (optional - will redirect to web auth if omitted)

**Examples:**
```bash
# Initialize with token
shelltime init --token abc123xyz

# Initialize via web authentication
shelltime init
```

#### `shelltime track`

Tracks shell commands and activities. This is typically called automatically by shell hooks.

```bash
shelltime track [options]
```

**Options:**
- `--shell, -s`: Specify the shell type (bash, zsh, fish, etc.)
- `--command, -c`: The command to track
- `--exit-code, -ec`: Exit code of the command
- `--start-time, -st`: Command start time (Unix timestamp)
- `--duration, -d`: Command duration in seconds
- `--shell-pid`: Shell process ID
- `--path, -p`: Current working directory

**Note:** This command is usually invoked automatically by shell hooks, not manually.

#### `shelltime sync`

Manually synchronizes local commands to the shelltime.xyz server.

```bash
shelltime sync [--dry-run]
```

**Options:**
- `--dry-run, -dr`: Preview what would be synced without actually syncing

**Examples:**
```bash
# Force sync all pending commands
shelltime sync

# Preview sync without uploading
shelltime sync --dry-run
```

#### `shelltime gc`

Cleans up old tracking data and temporary files based on your `gcTime` configuration.

```bash
shelltime gc [options]
```

**Options:**
- `--withLog, -wl`: Also clean log files
- `--skipLogCreation, -sl`: Don't create a new log file after cleaning
- `--dry-run, -dr`: Preview what would be cleaned without actually deleting

**Examples:**
```bash
# Clean old data (older than gcTime days)
shelltime gc

# Clean old data and logs
shelltime gc --withLog

# Preview cleanup
shelltime gc --dry-run
```

#### `shelltime ls`

Lists locally saved commands that haven't been synced yet.

```bash
shelltime ls [--format <format>]
```

**Options:**
- `--format, -f`: Output format (table/json) - default: table

**Examples:**
```bash
# List commands in table format
shelltime ls

# Export as JSON
shelltime ls --format json
```

#### `shelltime query` (AI Assistant)

Query the AI assistant for command suggestions based on your prompt.

```bash
shelltime query "<your prompt>"
shelltime q "<your prompt>"  # Short alias
```

**Examples:**
```bash
shelltime query "get the top 5 memory-using processes"
shelltime q "find all files modified in the last 24 hours"
shelltime q "show disk usage for current directory"
shelltime q "compress all png files in current folder"
```

### Service Management Commands

#### `shelltime daemon`

Manages the background daemon service for improved performance.

```bash
shelltime daemon <subcommand>
```

**Subcommands:**
- `install`: Install the daemon service (requires sudo)
- `uninstall`: Remove the daemon service (requires sudo)
- `reinstall`: Reinstall the daemon service (requires sudo)

**Examples:**
```bash
# Install daemon for better performance
sudo shelltime daemon install

# Remove daemon service
sudo shelltime daemon uninstall

# Reinstall (useful for updates)
sudo shelltime daemon reinstall
```

#### `shelltime hooks`

Manages shell integration hooks for automatic command tracking.

```bash
shelltime hooks <subcommand>
```

**Subcommands:**
- `install`: Install shell hooks for your current shell
- `uninstall`: Remove shell hooks

**Examples:**
```bash
# Install hooks for automatic tracking
shelltime hooks install

# Remove hooks
shelltime hooks uninstall
```

### Utility Commands

#### `shelltime web`

Opens the ShellTime web dashboard in your default browser.

```bash
shelltime web
```

#### `shelltime doctor`

Checks your ShellTime setup and environment for issues.

```bash
shelltime doctor
```

This command will verify:
- Configuration file validity
- Token authentication
- Database connectivity
- Shell hook installation
- Daemon service status
- Network connectivity to shelltime.xyz

#### `shelltime alias`

Manages shell aliases synchronization.

```bash
shelltime alias <subcommand>
```

**Subcommands:**
- `import`: Import aliases from shell configuration files
  - `--fully-refresh, -f`: Replace all existing aliases

**Examples:**
```bash
# Import aliases from your shell config
shelltime alias import

# Full refresh (replace all)
shelltime alias import --fully-refresh
```

#### `shelltime dotfiles`

Manages dotfiles configuration backup and synchronization.

```bash
shelltime dotfiles <subcommand>
```

**Subcommands:**
- `push`: Push dotfiles to server
  - `--apps`: Specify which app configs to push

**Examples:**
```bash
# Push all dotfiles
shelltime dotfiles push

# Push specific app configs
shelltime dotfiles push --apps vim --apps tmux
```

### Version Information

Display the current version of ShellTime CLI:

```bash
shelltime --version
shelltime -v
```

### Getting Help

Get help for any command:

```bash
# General help
shelltime --help
shelltime -h

# Command-specific help
shelltime <command> --help
shelltime daemon --help
shelltime query --help
```
## Performance

> [!NOTE]
> - **Linux**: Uses `systemd` for service management
> - **macOS**: Uses `launchctl` for service management

### Command Execution Performance

By default, the CLI performs synchronization directly which may impact shell responsiveness in certain scenarios:

- Standard command saving: <8ms (local file I/O only)
- Network synchronization:
  - Southeast Asia (Singapore servers): ~100ms
  - Other regions: Can vary significantly based on location

### Recommended: Daemon Mode

If you experience latency issues, we strongly recommend using daemon mode for better performance:

```bash
sudo ~/.shelltime/bin/shelltime daemon install
```

Benefits of daemon mode:
- Asynchronous command tracking (shell blocking time <8ms)
- Background synchronization handling
- No impact on shell responsiveness
- Reliable data delivery even during network issues

The daemon service:
1. Runs in the background as a system service
2. Handles all network synchronization operations
3. Buffers commands during connectivity issues
4. Automatically retries failed synchronizations

For users experiencing high latency, daemon mode is the recommended configuration. You can also adjust `FlushCount` in the config for additional optimization:

```toml
FlushCount = 100  # Increased buffer size for less frequent syncs
```

Note: Even without the daemon, all commands are still preserved locally first, ensuring no data loss during network issues.

## Encryption

> [!IMPORTANT]
> This feature is only available from version 0.1.12 and requires daemon mode operation.

ShellTime supports end-to-end encryption for command tracking data, providing an additional layer of security for sensitive environments.

### Enabling Encryption

1. Request a new open token that supports encryption (existing tokens need to be replaced)
2. Enable encryption in your config file:

```toml
# ~/.shelltime/config.toml
encrypted = true
```

3. Ensure daemon mode is active (encryption only works with daemon mode)

### How It Works

The encryption process uses a hybrid RSA/AES-GCM approach for optimal security and performance:

1. Client retrieves the public key associated with your open token
2. For each request:
   - Generates a new AES-GCM key
   - Encrypts the AES-GCM key using RSA public key
   - Encrypts the actual payload using AES-GCM
   - Sends both encrypted key and payload to server

Server-side:
1. Decrypts the AES-GCM key using the open token's private key
2. Uses the decrypted AES-GCM key to decrypt the payload
3. Processes the decrypted command data

This hybrid approach provides:
- Strong security through asymmetric encryption (RSA)
- Efficient payload encryption through symmetric encryption (AES-GCM)
- Perfect forward secrecy with unique keys per request

### Encrypted Request Structure

```json
{
    "encrypted": "<aes-gcm encrypted payload>",
    "aes_key": "<rsa encrypted aes-gcm key>",
    "nonce": "<aes-gcm nonce>"
}
```

> [!NOTE]
> - Encryption adds minimal overhead (~5-10ms per request)
> - All encryption/decryption happens automatically when enabled
> - Local data remains unencrypted for performance

### Uninstalling Daemon Service

To stop and remove the daemon service from your system:

```bash
sudo ~/.shelltime/bin/shelltime daemon uninstall
```

This command will:
1. Stop the currently running daemon
2. Remove the service configuration from systemd/launchctl
3. Clean up any daemon-specific temporary files

After uninstallation, the CLI will revert to direct synchronization mode. You can reinstall the daemon at any time using the install command if needed.

## Version Information

Use `shelltime --version` or `shelltime -v` to display the current version of the CLI.

## Support

For support, please contact: annatar.he+shelltime.xyz@gmail.com

## License

Copyright (c) 2024 shelltime.xyz Team
