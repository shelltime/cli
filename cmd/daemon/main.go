package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/uptrace/uptrace-go/uptrace"
	"go.opentelemetry.io/otel/attribute"
)

var (
	version    = "dev"
	commit     = "none"
	date       = "unknown"
	uptraceDsn = ""

	ppEndpoint = ""
	ppToken    = ""
)

func main() {
	// Handle version flag first, before any service initialization
	showVersion := flag.Bool("v", false, "Show version information")
	showVersionLong := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion || *showVersionLong {
		printVersionInfo()
		return
	}

	l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	slog.SetDefault(l)

	ctx := context.Background()
	configDir := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", model.COMMAND_BASE_STORAGE_FOLDER))
	daemonConfigService := model.NewConfigService(configDir)
	cfg, err := daemonConfigService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to get daemon config", slog.Any("err", err))
		return
	}

	slog.DebugContext(ctx, "daemon.config", slog.Any("config", cfg))

	uptraceOptions := []uptrace.Option{
		uptrace.WithDSN(uptraceDsn),
		uptrace.WithServiceName("cli-daemon"),
		uptrace.WithServiceVersion(version),
	}

	hs, err := os.Hostname()
	if err == nil && hs != "" {
		uptraceOptions = append(uptraceOptions, uptrace.WithResourceAttributes(attribute.String("hostname", hs)))
	}

	if err != nil ||
		cfg.EnableMetrics == nil ||
		*cfg.EnableMetrics == false ||
		uptraceDsn == "" {
		uptraceOptions = append(
			uptraceOptions,
			uptrace.WithMetricsDisabled(),
			uptrace.WithTracingDisabled(),
			uptrace.WithLoggingDisabled(),
		)
	}
	uptrace.ConfigureOpentelemetry(uptraceOptions...)
	defer uptrace.Shutdown(ctx)
	defer uptrace.ForceFlush(ctx)

	daemon.Init(daemonConfigService, version)
	model.InjectVar(version)
	cmdService := model.NewCommandService()

	pubsub := daemon.NewGoChannel(daemon.PubSubConfig{}, watermill.NewSlogLogger(slog.Default()))
	msg, err := pubsub.Subscribe(context.Background(), daemon.PubSubTopic)

	if err != nil {
		slog.Error("Failed to subscribe the message queue topic", slog.String("topic", daemon.PubSubTopic), slog.Any("err", err))
		return
	}

	// Start sync circuit breaker service
	syncCircuitBreakerService := daemon.NewSyncCircuitBreakerService(pubsub)
	if err := syncCircuitBreakerService.Start(ctx); err != nil {
		slog.Error("Failed to start sync circuit breaker service", slog.Any("err", err))
	} else {
		slog.Info("Sync circuit breaker service started")
		defer syncCircuitBreakerService.Stop()
	}

	// Start cleanup timer service if enabled (enabled by default)
	if cfg.LogCleanup != nil && cfg.LogCleanup.Enabled != nil && *cfg.LogCleanup.Enabled {
		cleanupTimerService := daemon.NewCleanupTimerService(cfg)
		if err := cleanupTimerService.Start(ctx); err != nil {
			slog.Error("Failed to start cleanup timer service", slog.Any("err", err))
		} else {
			slog.Info("Cleanup timer service started",
				slog.Int64("thresholdMB", cfg.LogCleanup.ThresholdMB))
			defer cleanupTimerService.Stop()
		}
	}

	go daemon.SocketTopicProcessor(msg)

	// Start CCUsage service if enabled (v1 - ccusage CLI based)
	if cfg.CCUsage != nil && cfg.CCUsage.Enabled != nil && *cfg.CCUsage.Enabled {
		ccUsageService := model.NewCCUsageService(cfg, cmdService)
		if err := ccUsageService.Start(ctx); err != nil {
			slog.Error("Failed to start CCUsage service", slog.Any("err", err))
		} else {
			slog.Info("CCUsage service started")
			defer ccUsageService.Stop()
		}
	}

	// Start AICodeOtel service if enabled (OTEL gRPC passthrough for Claude Code, Codex, etc.)
	var aiCodeOtelServer *daemon.AICodeOtelServer
	if cfg.AICodeOtel != nil && cfg.AICodeOtel.Enabled != nil && *cfg.AICodeOtel.Enabled {
		aiCodeOtelProcessor := daemon.NewAICodeOtelProcessor(cfg)
		aiCodeOtelServer = daemon.NewAICodeOtelServer(cfg.AICodeOtel.GRPCPort, aiCodeOtelProcessor)
		if err := aiCodeOtelServer.Start(); err != nil {
			slog.Error("Failed to start AICodeOtel gRPC server", slog.Any("err", err))
		} else {
			slog.Info("AICodeOtel gRPC server started", slog.Int("port", cfg.AICodeOtel.GRPCPort))
			defer aiCodeOtelServer.Stop()
		}
	}

	// Start heartbeat resync service if codeTracking is enabled
	if cfg.CodeTracking != nil && cfg.CodeTracking.Enabled != nil && *cfg.CodeTracking.Enabled {
		heartbeatResyncService := daemon.NewHeartbeatResyncService(cfg)
		if err := heartbeatResyncService.Start(ctx); err != nil {
			slog.Error("Failed to start heartbeat resync service", slog.Any("err", err))
		} else {
			slog.Info("Heartbeat resync service started")
			defer heartbeatResyncService.Stop()
		}
	}

	// Create processor instance
	processor := daemon.NewSocketHandler(&cfg, pubsub)

	// Start processor
	if err := processor.Start(); err != nil {
		slog.Error("Failed to start processor", slog.Any("err", err))
	}

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cleanup
	pubsub.Close()
	processor.Stop()
}

func printVersionInfo() {
	fmt.Printf("shelltime-daemon %s\n", version)
	fmt.Printf("  Commit:     %s\n", commit)
	fmt.Printf("  Build Date: %s\n", date)
	fmt.Printf("  Go Version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()
	fmt.Println("Use 'shelltime daemon status' to check the running daemon status.")
}
