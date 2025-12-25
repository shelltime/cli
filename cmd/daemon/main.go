package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	slog.SetDefault(l)

	ctx := context.Background()
	configFile := os.ExpandEnv(fmt.Sprintf("%s/%s/%s", "$HOME", model.COMMAND_BASE_STORAGE_FOLDER, "config.toml"))
	daemonConfigService := model.NewConfigService(configFile)
	cfg, err := daemonConfigService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to get daemon config", slog.Any("err", err))
		return
	}

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

	// Start CCOtel service if enabled (v2 - OTEL gRPC passthrough)
	var ccOtelServer *daemon.CCOtelServer
	if cfg.CCOtel != nil && cfg.CCOtel.Enabled != nil && *cfg.CCOtel.Enabled {
		ccOtelProcessor := daemon.NewCCOtelProcessor(cfg)
		ccOtelServer = daemon.NewCCOtelServer(cfg.CCOtel.GRPCPort, ccOtelProcessor)
		if err := ccOtelServer.Start(); err != nil {
			slog.Error("Failed to start CCOtel gRPC server", slog.Any("err", err))
		} else {
			slog.Info("CCOtel gRPC server started", slog.Int("port", cfg.CCOtel.GRPCPort))
			defer ccOtelServer.Stop()
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
