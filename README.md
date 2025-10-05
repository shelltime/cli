# ShellTime CLI

[![codecov](https://codecov.io/gh/shelltime/cli/graph/badge.svg?token=N09WIJHNI2)](https://codecov.io/gh/shelltime/cli)
[![shelltime](https://api.shelltime.xyz/badge/AnnatarHe/count)](https://shelltime.xyz/users/AnnatarHe)

> A professional command-line interface for [ShellTime](https://shelltime.xyz) - the comprehensive platform for tracking and analyzing DevOps workflows.

**Documentation**: [https://deepwiki.com/shelltime/cli](https://deepwiki.com/shelltime/cli)

## Quick Start

### Installation

Install ShellTime CLI using our automated installation script:

```bash
curl -sSL https://shelltime.xyz/i | bash
```

### Initial Setup

1. **Initialize authentication**:
   ```bash
   shelltime init
   ```

2. **Install shell hooks** (for automatic command tracking):
   ```bash
   shelltime hooks install
   ```

3. **Optional: Enable daemon mode** (recommended for optimal performance):
   ```bash
   shelltime daemon install
   ```

## Configuration

ShellTime CLI configuration is stored in `$HOME/.shelltime/config.toml`. The configuration file is automatically created during initialization.

### Configuration Reference

#### Core Settings

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
#### AI Agent Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ai.agent.view` | boolean | `false` | Allow AI to auto-execute read-only commands |
| `ai.agent.edit` | boolean | `false` | Allow AI to auto-execute file editing commands |
| `ai.agent.delete` | boolean | `false` | Allow AI to auto-execute deletion commands |

#### Analytics Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ccusage.enabled` | boolean | `false` | Enable Claude Code usage tracking and analytics |

### Configuration Example

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

### Important Configuration Considerations

#### Performance Optimization
- **Metrics Collection**: The `enableMetrics` option impacts performance. Enable only for debugging purposes.
- **Flush Count**: Adjust `flushCount` based on your workflow. Higher values reduce sync frequency but increase local storage.
- **Daemon Mode**: Essential for users experiencing latency issues, especially in regions distant from servers.

#### Security & Privacy
- **End-to-End Encryption**: Requires daemon mode and a special token from shelltime.xyz.
- **Data Masking**: Automatically masks sensitive information in tracked commands.
- **Exclusion Patterns**: Use regex patterns to prevent tracking of sensitive commands.

#### AI Assistant Permissions
The AI agent configuration provides granular control over command execution:
- **View Operations**: Read-only commands (ls, cat, grep)
- **Edit Operations**: File modification commands (vim, sed)
- **Delete Operations**: Removal commands (rm, rmdir)

All permissions default to `false` for maximum security.

## Command Reference

### Authentication & Setup

#### `shelltime init`

Initialize ShellTime CLI with your authentication token.

```bash
shelltime init [--token <your-token>]
```

**Options:**
- `--token, -t`: Personal access token (optional; uses web auth if omitted)

**Usage:**
```bash
# Initialize with token
shelltime init --token abc123xyz

# Initialize via web authentication
shelltime init
```

### Command Tracking

#### `shelltime track`

Track shell commands and activities (typically invoked automatically via shell hooks).

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

**Note:** Automatically invoked by shell hooks after installation.

#### `shelltime sync`

Synchronize local command history with ShellTime servers.

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

Perform garbage collection on old tracking data based on configured retention period.

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

Display pending commands awaiting synchronization.

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

### AI Assistant

#### `shelltime query` / `shelltime q`

Query the AI assistant for command suggestions and solutions.

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

### Service Management

#### `shelltime daemon`

Manage the background daemon service for enhanced performance.

```bash
shelltime daemon <subcommand>
```

**Subcommands:**
- `install`: Install the daemon service
- `uninstall`: Remove the daemon service
- `reinstall`: Reinstall the daemon service

**Examples:**
```bash
# Install daemon for better performance
shelltime daemon install

# Remove daemon service
shelltime daemon uninstall

# Reinstall (useful for updates)
shelltime daemon reinstall
```

#### `shelltime hooks`

Manage shell integration hooks for automatic command tracking.

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

### Utilities

#### `shelltime web`

Launch ShellTime web dashboard in your default browser.

```bash
shelltime web
```

#### `shelltime doctor`

Diagnose and validate ShellTime installation and configuration.

```bash
shelltime doctor
```

**Verification scope:**
- Configuration file validity
- Token authentication
- Database connectivity
- Shell hook installation
- Daemon service status
- Network connectivity to shelltime.xyz

#### `shelltime alias`

Manage and synchronize shell aliases.

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

Manage dotfiles backup and synchronization.

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

### Help & Version

#### Version Information
```bash
shelltime --version  # Display version information
shelltime -v         # Short form
```

#### Help Documentation
```bash
shelltime --help              # General help
shelltime <command> --help    # Command-specific help
```
## Performance & Optimization

### Platform Support
- **Linux**: Utilizes `systemd` for service management
- **macOS**: Utilizes `launchctl` for service management

### Performance Metrics

Default synchronization behavior and expected latencies:

| Operation | Latency | Description |
|-----------|---------|-------------|
| Local Save | <8ms | File I/O operations only |
| Network Sync (Singapore) | ~100ms | Southeast Asia region |
| Network Sync (Other) | Variable | Depends on geographic location |

### Daemon Mode (Recommended)

For optimal performance and minimal shell latency, enable daemon mode:

```bash
~/.shelltime/bin/shelltime daemon install
```

**Key Benefits:**
- **Asynchronous Processing**: Shell blocking reduced to <8ms
- **Background Synchronization**: Network operations handled independently
- **Zero Shell Impact**: Complete isolation from interactive sessions
- **Resilient Delivery**: Automatic retry and buffering during network issues

**Technical Implementation:**
- Operates as a user-level service
- Manages all network synchronization operations
- Implements intelligent command buffering
- Provides automatic retry mechanisms for failed synchronizations

**Optimization Tips:**
```toml
# Adjust buffer size for reduced sync frequency
FlushCount = 100
```

> **Note**: Commands are always persisted locally first, ensuring zero data loss regardless of network conditions.

## Security Features

### End-to-End Encryption

> **Requirements**: Version 0.1.12+ with daemon mode enabled

ShellTime provides enterprise-grade end-to-end encryption for command data, ensuring complete privacy in sensitive environments.

#### Activation Process

1. **Obtain encryption-enabled token** from shelltime.xyz
2. **Configure encryption** in settings:

```toml
# ~/.shelltime/config.toml
encrypted = true
```

3. **Verify daemon mode** is active

#### Technical Architecture

ShellTime implements a hybrid RSA/AES-GCM encryption scheme:

**Client-Side Operations:**
1. Retrieve public key associated with authentication token
2. Generate unique AES-GCM key per request
3. Encrypt AES key using RSA public key encryption
4. Encrypt payload using AES-GCM symmetric encryption
5. Transmit encrypted key and payload to server

**Server-Side Operations:**
1. Decrypt AES-GCM key using private key
2. Decrypt payload using recovered AES-GCM key
3. Process decrypted command data

**Security Advantages:**
- **Asymmetric Security**: RSA provides robust key exchange
- **Symmetric Efficiency**: AES-GCM ensures fast payload encryption
- **Perfect Forward Secrecy**: Unique keys per request prevent retrospective decryption

#### Request Structure

```json
{
    "encrypted": "<aes-gcm encrypted payload>",
    "aes_key": "<rsa encrypted aes-gcm key>",
    "nonce": "<aes-gcm nonce>"
}
```

**Performance Impact:**
- Additional latency: ~5-10ms per request
- Automatic encryption/decryption when enabled
- Local storage remains unencrypted for performance

## Maintenance

### Daemon Service Management

#### Uninstallation
Remove the daemon service when no longer needed:

```bash
~/.shelltime/bin/shelltime daemon uninstall
```

**Uninstallation Process:**
1. Terminate running daemon process
2. Remove service configuration from system
3. Clean up temporary files and sockets

**Post-Uninstallation:**
- CLI reverts to direct synchronization mode
- Reinstallation available at any time via `daemon install`

## Troubleshooting

### Common Issues

1. **High Latency**: Enable daemon mode for improved performance
2. **Authentication Failures**: Re-run `shelltime init` with a fresh token
3. **Missing Commands**: Verify shell hooks are installed with `shelltime hooks install`
4. **Sync Issues**: Check network connectivity and run `shelltime doctor`

### Diagnostic Tools

- `shelltime doctor`: Comprehensive system check
- `shelltime sync --dry-run`: Preview sync operations
- `shelltime gc --dry-run`: Preview cleanup operations

## Resources

### Documentation
- **Official Documentation**: [https://deepwiki.com/shelltime/cli](https://deepwiki.com/shelltime/cli)
- **Web Dashboard**: [https://shelltime.xyz](https://shelltime.xyz)

### Support
- **Email**: annatar.he+shelltime.xyz@gmail.com
- **Issue Tracker**: [GitHub Issues](https://github.com/shelltime/cli/issues)
- **Community**: [ShellTime Community Forum](https://community.shelltime.xyz)

## License

Copyright © 2024 ShellTime Team. All rights reserved.

Licensed under the proprietary ShellTime license. See LICENSE file for details.
