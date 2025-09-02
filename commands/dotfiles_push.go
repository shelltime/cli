package commands

import (
	"fmt"
	"os"

	"github.com/malamtime/cli/model"
	"github.com/sirupsen/logrus"
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
		logrus.Errorln(err)
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
				logrus.Warnf("Unknown app: %s", appName)
			}
		}
	}

	// Collect all dotfiles
	var allDotfiles []model.DotfileItem
	for _, app := range selectedApps {
		logrus.Infof("Collecting dotfiles for %s", app.Name())
		dotfiles, err := app.CollectDotfiles(ctx)
		if err != nil {
			logrus.Errorf("Failed to collect dotfiles for %s: %v", app.Name(), err)
			continue
		}
		allDotfiles = append(allDotfiles, dotfiles...)
	}

	if len(allDotfiles) == 0 {
		logrus.Infoln("No dotfiles found to push")
		return nil
	}

	// Send to server
	logrus.Infof("Pushing %d dotfiles to server", len(allDotfiles))
	userID, err := model.SendDotfilesToServer(ctx, mainEndpoint, allDotfiles)
	if err != nil {
		logrus.Errorln("Failed to send dotfiles to server:", err)
		return err
	}

	// Generate web link for managing dotfiles
	webLink := fmt.Sprintf("%s/users/%d/settings/dotfiles", config.WebEndpoint, userID)
	logrus.Infof("Successfully pushed dotfiles. Manage them at: %s", webLink)
	fmt.Printf("\n✅ Successfully pushed %d dotfiles to server\n", len(allDotfiles))
	fmt.Printf("📁 Manage your dotfiles at: %s\n", webLink)

	return nil
}
