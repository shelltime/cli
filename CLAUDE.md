# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ShellTime CLI is a Go-based command-line tool for tracking DevOps work. It consists of two main binaries:
- `shelltime`: The main CLI tool for command tracking and management
- `shelltime-daemon`: A background service for asynchronous command tracking and synchronization

## Development Commands

### Building
```bash
# Build the CLI binary
go build -o shelltime ./cmd/cli/main.go

# Build the daemon binary
go build -o shelltime-daemon ./cmd/daemon/main.go

# Build with version information
go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%d)" -o shelltime ./cmd/cli/main.go
```

### Testing
```bash
# Run all tests with coverage
go test -timeout 3m -coverprofile=coverage.txt -covermode=atomic ./...

# Run tests for a specific package
go test ./commands/...
go test ./daemon/...
go test ./model/...

# Run a single test
go test -run TestHandlerName ./daemon/
```

### Code Generation
```bash
# Install mockery if not already installed
go install github.com/vektra/mockery/v2@v2.42.0

# Generate mocks (uses .mockery.yml configuration)
mockery
```

### Linting
```bash
# Run go vet
go vet ./...

# Format code
go fmt ./...
```

## Architecture

### Package Structure
- **cmd/**: Entry points for the binaries
  - `cli/`: Main CLI application entry point
  - `daemon/`: Daemon service entry point

- **commands/**: CLI command implementations (auth, track, sync, gc, daemon management, hooks)
  - Each command is self-contained in its own file
  - `base.go` provides shared functionality across commands
  - Hook management for shell integrations (bash, zsh, fish)

- **daemon/**: Daemon service implementation
  - Unix domain socket-based IPC communication with CLI
  - Watermill pub/sub messaging for async command processing
  - Channel-based architecture for concurrent operations

- **model/**: Core business logic and data models
  - API client implementations with encryption support
  - Database operations (local SQLite storage)
  - Shell-specific hook implementations
  - System service installers (systemd/launchctl)

### Key Architectural Patterns

1. **Command Pattern**: Each CLI command implements the `urfave/cli/v2` command interface
2. **Service Pattern**: Interfaces (ConfigService, AIService, CommandService) with dependency injection via `commands.InjectVar()` and `commands.InjectAIService()`
3. **IPC Communication**: Unix domain sockets for CLI-daemon communication
4. **Pub/Sub Messaging**: Watermill GoChannel for internal daemon message routing
5. **Batch Processing**: Commands are buffered locally and synced in batches
6. **Encryption**: Hybrid RSA/AES-GCM encryption for secure command transmission

### Data Flow
1. Shell hooks capture commands →
2. CLI stores commands locally (file based) →
3. Daemon (if installed) processes commands asynchronously →
4. Batch sync to shelltime.xyz API

### Configuration
- Config file location: `$HOME/.shelltime/config.toml`
- Local overrides: `$HOME/.shelltime/config.local.toml` (merged with main config, not committed to version control)
- Database location: `$HOME/.shelltime/shelltime.db`
- Daemon socket: `/tmp/shelltime-daemon.sock` (Unix) or named pipe (Windows)

## Commit Rules

You must follow the Conventional Commits rules, ensuring that the scope and module are included.

For example:

```md
fix(home): add price link on home page
feat(ai): add AI module
refactor(cell): update cell module for better maintenance
perf(parser): improve parser performance by over 30%
```

## Important Notes

- The daemon is optional but recommended for better performance
- Encryption requires daemon mode and a special token
- All local storage uses SQLite for reliability
- The project uses OpenTelemetry for observability (when enabled)
- Shell hooks are platform-specific and require careful testing