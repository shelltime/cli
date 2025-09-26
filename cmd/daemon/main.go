package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
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

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Error("Failed to get user home directory", slog.Any("err", err))
		return ""
	}
	return filepath.Join(homeDir, ".shelltime", "config.toml")
}

func main() {
	l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	slog.SetDefault(l)

	daemonConfigService := daemon.NewConfigService(daemon.DefaultConfigPath)
	daemonConfig, err := daemonConfigService.GetConfig()
	if err != nil {
		slog.Error("Failed to get daemon config", slog.Any("err", err))
		return
	}

	cs, err := daemonConfigService.GetUserConfig()
	if err != nil {
		slog.Error("Failed to get user config", slog.Any("err", err))
		return
	}

	ctx := context.Background()
	uptraceOptions := []uptrace.Option{
		uptrace.WithDSN(uptraceDsn),
		uptrace.WithServiceName("cli-daemon"),
		uptrace.WithServiceVersion(version),
	}

	hs, err := os.Hostname()
	if err == nil && hs != "" {
		uptraceOptions = append(uptraceOptions, uptrace.WithResourceAttributes(attribute.String("hostname", hs)))
	}

	cfg, err := cs.ReadConfigFile(ctx)
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

	daemon.Init(cs, version)
	model.InjectVar(version)

	pubsub := daemon.NewGoChannel(daemon.PubSubConfig{}, watermill.NewSlogLogger(slog.Default()))
	msg, err := pubsub.Subscribe(context.Background(), daemon.PubSubTopic)

	if err != nil {
		slog.Error("Failed to subscribe the message queue topic", slog.String("topic", daemon.PubSubTopic), slog.Any("err", err))
		return
	}

	go daemon.SocketTopicProccessor(msg)

	// Start CCUsage service if enabled
	if cfg.CCUsage != nil && cfg.CCUsage.Enabled != nil && *cfg.CCUsage.Enabled {
		ccUsageService := model.NewCCUsageService(cfg)
		if err := ccUsageService.Start(ctx); err != nil {
			slog.Error("Failed to start CCUsage service", slog.Any("err", err))
		} else {
			slog.Info("CCUsage service started")
			defer ccUsageService.Stop()
		}
	}

	// Create processor instance
	processor := daemon.NewSocketHandler(daemonConfig, pubsub)

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
