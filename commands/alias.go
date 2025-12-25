package commands

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var AliasCommand *cli.Command = &cli.Command{
	Name:  "alias",
	Usage: "manage shell aliases",
	Subcommands: []*cli.Command{
		{
			Name:   "import",
			Usage:  "import aliases from shell configuration files",
			Action: importAliases,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "fully-refresh",
					Aliases: []string{"full"},
					Usage:   "fully refresh all aliases instead of incremental import",
					Value:   false,
				},
				&cli.StringFlag{
					Name:    "fish-config",
					Aliases: []string{"fc"},
					Usage:   "the fish config file. default is ~/.config/fish/config.fish",
					Value:   "~/.config/fish/config.fish",
				},
				&cli.StringFlag{
					Name:    "zsh-config",
					Aliases: []string{"zc"},
					Usage:   "the zsh config file. default is ~/.zshrc",
					Value:   "~/.zshrc",
				},
			},
		},
	},
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		return nil
	},
}

func importAliases(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "alias-import", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	isFullyRefresh := c.Bool("fully-refresh")
	span.SetAttributes(attribute.Bool("fully-refresh", isFullyRefresh))

	zshConfigFile, err := expandPath(c.String("zsh-config"))
	if err != nil {
		slog.Error("failed to expand zsh config path", slog.Any("err", err))
		return err
	}
	fishConfigFile, err := expandPath(c.String("fish-config"))
	if err != nil {
		slog.Error("failed to expand fish config path", slog.Any("err", err))
		return err
	}

	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("failed to read config file", slog.Any("err", err))
		return err
	}

	mainEndpoint := model.Endpoint{
		APIEndpoint: config.APIEndpoint,
		Token:       config.Token,
	}

	if _, err := os.Stat(zshConfigFile); err == nil {
		aliases, err := parseZshAliases(ctx, zshConfigFile)
		if err != nil {
			slog.Error("Failed to parse zsh aliases", slog.Any("err", err))
			return err
		}
		slog.Debug("Found aliases in zsh configuration", slog.Int("count", len(aliases)))
		err = model.SendAliasesToServer(
			ctx,
			mainEndpoint,
			aliases,
			isFullyRefresh,
			"zsh",
			zshConfigFile,
		)
		if err != nil {
			slog.Error("Failed to send aliases to server", slog.Any("err", err))
			return err
		}
	}

	if _, err := os.Stat(fishConfigFile); err == nil {
		aliases, err := parseFishAliases(ctx, fishConfigFile)
		if err != nil {
			slog.Error("Failed to parse fish aliases", slog.Any("err", err))
			return err
		}
		slog.Debug("Found aliases in fish configuration", slog.Int("count", len(aliases)))
		err = model.SendAliasesToServer(
			ctx,
			mainEndpoint,
			aliases,
			isFullyRefresh,
			"fish",
			fishConfigFile,
		)
		if err != nil {
			slog.Error("Failed to send aliases to server", slog.Any("err", err))
			return err
		}
	}

	slog.Info("Successfully imported aliases")
	return nil
}

func parseZshAliases(ctx context.Context, zshConfigFile string) ([]string, error) {
	ctx, span := commandTracer.Start(ctx, "parse-zsh-aliases")
	defer span.End()

	return parseAliasFile(zshConfigFile, parseZshAliasLine)
}

func parseFishAliases(ctx context.Context, fishConfigFile string) ([]string, error) {
	ctx, span := commandTracer.Start(ctx, "parse-fish-aliases")
	defer span.End()

	return parseAliasFile(fishConfigFile, parseFishAliasLine)
}

func parseAliasFile(filePath string, lineParser func(string) (string, bool)) ([]string, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(fileContent), "\n")
	var aliases []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if alias, ok := lineParser(line); ok {
			aliases = append(aliases, alias)
		}
	}

	return aliases, nil
}

func parseZshAliasLine(line string) (string, bool) {
	return line, true
}

func parseFishAliasLine(line string) (string, bool) {
	return line, true
}
