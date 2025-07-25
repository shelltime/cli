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

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token` | string | `""` | Your authentication token for shelltime.xyz |
| `apiEndpoint` | string | `"https://api.shelltime.xyz"` | The API endpoint URL for shelltime.xyz |
| `webEndpoint` | string | `"https://shelltime.xyz"` | The web interface URL for shelltime.xyz |
| `flushCount` | integer | `10` | Number of records to accumulate before syncing to server |
| `gcTime` | integer | `14` | Number of days to keep tracked data before garbage collection |
| `dataMasking` | boolean | `true` | Enable/disable masking of sensitive data in tracked commands |
| `enableMetrics` | boolean | `false` | Enable detailed command metrics tracking (WARNING: May impact performance) |
| `endpoints` | array | `[]` | Additional API endpoints for development or testing |
| `ai.agent.view` | boolean | `false` | Auto-run AI for view commands (e.g., ls, cat, show) |
| `ai.agent.edit` | boolean | `false` | Auto-run AI for edit commands (e.g., vim, nano, code) |
| `ai.agent.delete` | boolean | `false` | Auto-run AI for delete commands (e.g., rm, rmdir) |

Example configuration:
```toml
token = "your-token-here"
apiEndpoint = "https://api.shelltime.xyz"
webEndpoint = "https://shelltime.xyz"
flushCount = 10
gcTime = 14
dataMasking = true
enableMetrics = false

# AI configuration (optional)
[ai.agent]
view = false    # Auto-run AI for view commands
edit = false    # Auto-run AI for edit commands
delete = false  # Auto-run AI for delete commands
```

⚠️ Note: Setting `enableMetrics` to `true` will track detailed metrics for every command execution. Only enable this when requested by developers for debugging purposes, as it may impact shell performance.

📤 AI Auto-run: When AI agent auto-run is enabled for specific command types, the AI will automatically analyze and provide suggestions when you run matching commands. This feature requires the AI service to be properly configured.

## Commands

### Authentication

```bash
shelltime auth [--token <your-token>]
```

Initializes the CLI with your shelltime.xyz authentication token. This command needs to be run before using other features.

Options:
- `--token`: Your personal access token from shelltime.xyz. if omit, you can also redirect to website to auth

Example:
```bash
shelltime auth --token abc123xyz
```

### Track

```bash
shelltime track [options]
```

Tracks your shells activities and sends them to shelltime.xyz.

Options:
- TODO: List track command options

Example:
```bash
shelltime track # TODO: Add example
```

### Sync

```bash
shelltime sync
```

Manually triggers synchronization of locally tracked commands to the shelltime.xyz server. This command can be useful when:
- You want to force an immediate sync without waiting for the automatic sync threshold
- You're troubleshooting data synchronization issues
- You need to ensure all local data is uploaded before system maintenance

Example:
```bash
shelltime sync
```

There are no additional options for this command as it simply processes and uploads any pending tracked commands according to your configuration settings.

### GC (Garbage Collection)

```bash
shelltime gc [options]
```

Performs cleanup of old tracking data and temporary files.

Options:
- TODO: List GC command options

Example:
```bash
shelltime gc # TODO: Add example
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
