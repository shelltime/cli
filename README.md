# ShellTime CLI [![codecov](https://codecov.io/gh/malamtime/cli/graph/badge.svg?token=N09WIJHNI2)](https://codecov.io/gh/malamtime/cli)

The foundation CLI tool for shelltime.xyz - a platform for tracking DevOps work.

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

Example configuration:
```toml
token = "your-token-here"
apiEndpoint = "https://api.shelltime.xyz"
webEndpoint = "https://shelltime.xyz"
flushCount = 10
gcTime = 14
dataMasking = true
enableMetrics = false
```

⚠️ Note: Setting `enableMetrics` to `true` will track detailed metrics for every command execution. Only enable this when requested by developers for debugging purposes, as it may impact shell performance.

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

## Version Information

Use `shelltime --version` or `shelltime -v` to display the current version of the CLI.

## Support

For support, please contact: annatar.he+shelltime.xyz@gmail.com

## License

Copyright (c) 2024 shelltime.xyz Team
