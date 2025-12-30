# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ShellTime CLI is a Go-based command-line tool for tracking DevOps work. It consists of two main binaries:
- `shelltime`: The main CLI tool for command tracking and management
- `shelltime-daemon`: A background service for asynchronous command tracking, synchronization, and OTEL data collection

## Development Commands

### Building
```bash
# Build the CLI binary
go build -o shelltime ./cmd/cli/main.go

# Build the daemon binary
go build -o shelltime-daemon ./cmd/daemon/main.go

# Build with all ldflags (version, commit, AI service config)
go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%d) -X main.ppEndpoint=<endpoint> -X main.ppToken=<token>" -o shelltime ./cmd/cli/main.go
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
# Generate mocks (uses .mockery.yml configuration)
mockery
```

### Linting
```bash
go vet ./...
go fmt ./...
```

## Architecture

### Package Structure
- **cmd/cli/**: CLI entry point - registers all commands, initializes services via dependency injection
- **cmd/daemon/**: Daemon entry point - sets up pub/sub, socket handler, and optional AICodeOtel gRPC server
- **commands/**: CLI command implementations - each command in its own file, `base.go` holds injected services
- **daemon/**: Daemon internals - socket handler, Watermill pub/sub channel, AICodeOtel gRPC server/processor
- **model/**: Business logic - API clients, config, crypto, shell hooks, service installers, dotfile handlers

### Service Interfaces (model package)
Three key interfaces with dependency injection:
- `ConfigService`: Reads and merges config from `config.toml` and `config.local.toml`
- `AIService`: PromptPal integration for AI-powered command suggestions (`shelltime q`)
- `CommandService`: Executable lookup with fallback paths (handles daemon's limited PATH)

Injection happens in `cmd/*/main.go` via `commands.InjectVar()` and `commands.InjectAIService()`.

### Daemon Architecture
1. **SocketHandler**: Unix domain socket server accepting JSON messages from CLI
2. **GoChannel**: Watermill pub/sub for decoupled message processing
3. **SocketTopicProcessor**: Consumes messages and routes to appropriate handlers
4. **AICodeOtelServer** (optional): gRPC server implementing OTEL collector for AI coding CLI metrics/logs passthrough (Claude Code, Codex, etc.)

### Data Flow
1. Shell hooks capture commands → CLI stores locally (file-based buffer)
2. CLI sends sync message to daemon via Unix socket
3. Daemon's pub/sub routes to sync handler
4. Batch sync to shelltime.xyz API with optional encryption

### Configuration
- Main config: `$HOME/.shelltime/config.toml`
- Local overrides: `$HOME/.shelltime/config.local.toml` (merged, gitignored)
- Daemon socket: `/tmp/shelltime.sock` (configurable via `socketPath`)
- AICodeOtel gRPC port: configurable via `aiCodeOtel.grpcPort` (default: 54027)

## Commit Rules

Follow Conventional Commits with scope: `fix(daemon): ...`, `feat(cli): ...`, `refactor(model): ...`

## Important Notes

- Daemon is optional but recommended (<8ms latency vs ~100ms+ direct)
- Encryption requires daemon mode and a token with encryption capability
- Shell hooks are platform-specific (bash, zsh, fish) - test on target shells
- AICodeOtel feature enables AI coding CLI metrics/logs passthrough via gRPC (port 54027) - supports Claude Code, Codex, and other OTEL-compatible CLIs