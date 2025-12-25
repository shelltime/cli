package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/malamtime/cli/commands"
	"github.com/malamtime/cli/model"
	"github.com/sirupsen/logrus"
	"github.com/uptrace/uptrace-go/uptrace"
	"github.com/urfave/cli/v2"
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
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "print the version",
	}
	configFile := os.ExpandEnv(fmt.Sprintf("%s/%s/%s", "$HOME", model.COMMAND_BASE_STORAGE_FOLDER, "config.toml"))
	configService := model.NewConfigService(configFile)

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

	// Initialize AI service if configured
	if ppEndpoint != "" && ppToken != "" {
		aiService := model.NewAIService(model.AIServiceConfig{
			Endpoint:  ppEndpoint,
			Token:     ppToken,
			Timeout:   60 * time.Second,
			UserToken: cfg.Token,
		})
		commands.InjectAIService(aiService)
	}
	app := cli.NewApp()
	app.Name = "shelltime CLI"
	app.Description = "shelltime.xyz CLI for track DevOps works"
	app.Usage = "shelltime.xyz CLI for track DevOps works"
	app.Version = version
	app.Copyright = "Copyright (c) 2025 shelltime.xyz Team"
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
	}
	err = app.Run(os.Args)
	if err != nil {
		logrus.Errorln(err)
	}
	commands.CloseLogger()
}
