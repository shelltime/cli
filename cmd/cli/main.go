package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/malamtime/cli/commands"
	"github.com/malamtime/cli/model"
	"github.com/uptrace/uptrace-go/uptrace"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
)

var (
	version    = "dev"
	commit     = "none"
	date       = "unknown"
	uptraceDsn = ""
)

func main() {
	os.Exit(run())
}

// run holds main's body so its deferred cleanup finishes before main sets the exit code.
func run() int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "print the version",
	}
	configDir := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", model.COMMAND_BASE_STORAGE_FOLDER))
	configService := model.NewConfigService(configDir)

	uptraceOptions := []uptrace.Option{
		uptrace.WithDSN(uptraceDsn),
		uptrace.WithServiceName("cli"),
		uptrace.WithServiceVersion(version),
	}

	hs, err := os.Hostname()
	if err == nil && hs != "" {
		uptraceOptions = append(uptraceOptions, uptrace.WithResourceAttributes(attribute.String("hostname", hs)))
	}

	cfg, err := configService.ReadConfigFile(ctx)
	if err == nil {
		if proxyErr := model.ConfigureProxy(cfg.Proxy); proxyErr != nil {
			slog.Warn("invalid proxy config, falling back to environment proxy", slog.Any("err", proxyErr))
		}
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

	model.InjectVar(version)
	commands.InjectVar(version, configService)

	// Initialize AI service if user has a token configured
	if cfg.Token != "" {
		aiService := model.NewAIService()
		commands.InjectAIService(aiService)
	}
	app := cli.NewApp()
	app.Name = "shelltime CLI"
	app.Description = "shelltime.xyz CLI for track DevOps works"
	app.Usage = "shelltime.xyz CLI for track DevOps works"
	app.Version = version
	app.Copyright = "Copyright (c) 2026 shelltime.xyz Team"
	app.Authors = []*cli.Author{
		{
			Name:  "shelltime.xyz Team",
			Email: "annatar.he+shelltime.xyz@gmail.com",
		},
	}
	app.Suggest = true
	app.HideVersion = false
	app.Metadata = map[string]interface{}{
		"version": version,
	}

	app.Commands = []*cli.Command{
		commands.InitCommand,
		commands.AuthCommand,
		commands.TrackCommand,
		commands.GCCommand,
		commands.SyncCommand,
		commands.DaemonCommand,
		commands.HooksCommand,
		commands.LsCommand,
		commands.WebCommand,
		commands.AliasCommand,
		commands.DotfilesCommand,
		commands.DoctorCommand,
		commands.QueryCommand,
		commands.CCCommand,
		commands.CodexCommand,
		commands.SchemaCommand,
		commands.GrepCommand,
		commands.ConfigCommand,
		commands.IosCommand,
		commands.UpdateCommand,
	}
	err = app.Run(os.Args)
	// doctor already reported its problems; it only needs a non-zero exit code.
	doctorFailed := errors.Is(err, commands.ErrDoctorFoundProblems)
	if err != nil && !doctorFailed {
		slog.Error("CLI error", slog.Any("err", err))
	}
	commands.CloseLogger()
	if doctorFailed {
		return 1
	}
	return 0
}
