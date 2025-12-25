package commands

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func pushDotfiles(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "dotfiles-push", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	apps := c.StringSlice("apps")
	span.SetAttributes(attribute.StringSlice("apps", apps))

	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("failed to read config file", slog.Any("err", err))
		return err
	}

	if config.Token == "" {
		return fmt.Errorf("no token found, please run 'shelltime auth login' first")
	}

	mainEndpoint := model.Endpoint{
		APIEndpoint: config.APIEndpoint,
		Token:       config.Token,
	}

	// Initialize all available app handlers
	allApps := []model.DotfileApp{
		model.NewNvimApp(),
		model.NewFishApp(),
		model.NewGitApp(),
		model.NewZshApp(),
		model.NewBashApp(),
		model.NewGhosttyApp(),
		model.NewClaudeApp(),
		model.NewStarshipApp(),
		model.NewNpmApp(),
		model.NewSshApp(),
		model.NewKittyApp(),
		model.NewKubernetesApp(),
	}

	// Filter apps based on user input
	var selectedApps []model.DotfileApp
	if len(apps) == 0 {
		// If no apps specified, use all
		selectedApps = allApps
	} else {
		// Filter based on user selection
		appMap := make(map[string]model.DotfileApp)
		for _, app := range allApps {
			appMap[app.Name()] = app
		}

		for _, appName := range apps {
			if app, ok := appMap[appName]; ok {
				selectedApps = append(selectedApps, app)
			} else {
				slog.Warn("Unknown app", slog.String("app", appName))
			}
		}
	}

	// Collect all dotfiles
	var allDotfiles []model.DotfileItem
	for _, app := range selectedApps {
		slog.Info("Collecting dotfiles", slog.String("app", app.Name()))
		dotfiles, err := app.CollectDotfiles(ctx)
		if err != nil {
			slog.Error("Failed to collect dotfiles", slog.String("app", app.Name()), slog.Any("err", err))
			continue
		}
		allDotfiles = append(allDotfiles, dotfiles...)
	}

	if len(allDotfiles) == 0 {
		slog.Info("No dotfiles found to push")
		return nil
	}

	// Send to server
	slog.Info("Pushing dotfiles to server", slog.Int("count", len(allDotfiles)))
	userID, err := model.SendDotfilesToServer(ctx, mainEndpoint, allDotfiles)
	if err != nil {
		slog.Error("Failed to send dotfiles to server", slog.Any("err", err))
		return err
	}

	// Generate web link for managing dotfiles
	webLink := fmt.Sprintf("%s/users/%d/settings/dotfiles", config.WebEndpoint, userID)
	slog.Info("Successfully pushed dotfiles", slog.String("webLink", webLink))
	fmt.Printf("\n✅ Successfully pushed %d dotfiles to server\n", len(allDotfiles))
	fmt.Printf("📁 Manage your dotfiles at: %s\n", webLink)

	return nil
}
