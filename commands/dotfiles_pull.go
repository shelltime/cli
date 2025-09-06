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

type dotfilePullFileResult struct {
	path      string
	isSuccess bool
	isSkipped bool
	isFailed  bool
}

func pullDotfiles(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "dotfiles-pull", trace.WithSpanKind(trace.SpanKindClient))
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

	// Initialize app handlers based on apps parameter
	var appHandlers map[model.DotfileAppName]model.DotfileApp

	// Prepare filter if apps are specified
	var filter *model.DotfileFilter
	if len(apps) > 0 {
		filter = &model.DotfileFilter{
			Apps: apps,
		}
		// Only include specified apps
		allAppsMap := model.GetAllAppsMap()
		appHandlers = make(map[model.DotfileAppName]model.DotfileApp)
		for _, appNameStr := range apps {
			appName := model.DotfileAppName(appNameStr)
			if appHandler, exists := allAppsMap[appName]; exists {
				appHandlers[appName] = appHandler
			}
		}
	}

	if len(appHandlers) == 0 {
		appHandlers = model.GetAllAppsMap()
	}

	// Fetch dotfiles from server
	logrus.Infof("Fetching dotfiles from server...")
	resp, err := model.FetchDotfilesFromServer(ctx, mainEndpoint, filter)
	if err != nil {
		logrus.Errorln("Failed to fetch dotfiles from server:", err)
		return err
	}

	if resp == nil || len(resp.Data.FetchUser.Dotfiles.Apps) == 0 {
		logrus.Infoln("No dotfiles found on server")
		fmt.Println("\n📭 No dotfiles found on server")
		return nil
	}

	result := map[model.DotfileAppName][]dotfilePullFileResult{}

	for _, appData := range resp.Data.FetchUser.Dotfiles.Apps {
		appName := model.DotfileAppName(appData.App)
		app, exists := appHandlers[appName]
		if !exists {
			logrus.Warnf("Unknown app type: %s", appData.App)
			continue
		}

		logrus.Infof("Processing %s dotfiles...", appData.App)

		// Collect files to process for this app
		filesToProcess := make(map[string]string)

		for _, file := range appData.Files {
			if len(file.Records) == 0 {
				logrus.Debugf("No records found for %s", file.Path)
				continue
			}

			// Get the best record: prioritize records without host, fallback to latest
			var selectedRecord *model.DotfileRecord
			var latestRecord *model.DotfileRecord

			result[appName] = make([]dotfilePullFileResult, 0)

			for i := range file.Records {
				record := &file.Records[i]

				// Track the latest record overall
				if latestRecord == nil || record.UpdatedAt.After(latestRecord.UpdatedAt) {
					latestRecord = record
				}

				// If we find a record without a host (general config), use it
				if record.Host.ID == 0 || record.Host.Hostname == "" {
					if selectedRecord == nil || record.UpdatedAt.After(selectedRecord.UpdatedAt) {
						selectedRecord = record
					}
				}
			}

			// If no host-less record found, use the latest record
			if selectedRecord == nil {
				selectedRecord = latestRecord
			}

			if selectedRecord == nil {
				continue
			}

			// Adjust path for current user
			adjustedPath := AdjustPathForCurrentUser(file.Path)
			filesToProcess[adjustedPath] = selectedRecord.Content
		}

		if len(filesToProcess) == 0 {
			continue
		}

		// Check which files are different
		equalityMap, err := app.IsEqual(ctx, filesToProcess)
		if err != nil {
			logrus.Warnf("Failed to check file equality for %s: %v", appData.App, err)
		}

		// Filter out files that are already equal
		filesToUpdate := make(map[string]string)
		var pathsToActuallyBackup []string

		for path, content := range filesToProcess {
			if isEqual, exists := equalityMap[path]; exists && isEqual {
				logrus.Debugf("Skipping %s - content is identical", path)
				result[appName] = append(result[appName], dotfilePullFileResult{
					path:      path,
					isSkipped: true,
				})
			} else {
				filesToUpdate[path] = content
				pathsToActuallyBackup = append(pathsToActuallyBackup, path)
			}
		}

		if len(filesToUpdate) == 0 {
			logrus.Infof("All %s files are up to date", appData.App)
			continue
		}

		// Backup files that will be modified
		if err := app.Backup(ctx, pathsToActuallyBackup); err != nil {
			logrus.Warnf("Failed to backup files for %s: %v", appData.App, err)
		}

		results := make([]dotfilePullFileResult, 0)
		// Save the updated files
		if err := app.Save(ctx, filesToUpdate); err != nil {
			logrus.Errorf("Failed to save files for %s: %v", appData.App, err)
			for f := range filesToUpdate {
				results = append(results, dotfilePullFileResult{
					path:      f,
					isSuccess: true,
				})
			}
		} else {
			for f := range filesToUpdate {
				results = append(results, dotfilePullFileResult{
					path:     f,
					isFailed: true,
				})
			}
		}
		result[appName] = append(result[appName], results...)
	}

	// Calculate totals from result map
	var totalProcessed, totalFailed, totalSkipped int
	for _, fileResults := range result {
		for _, fileResult := range fileResults {
			if fileResult.isSuccess {
				totalProcessed++
			} else if fileResult.isFailed {
				totalFailed++
			} else if fileResult.isSkipped {
				totalSkipped++
			}
		}
	}

	if totalProcessed == 0 && totalFailed == 0 && totalSkipped == 0 {
		logrus.Infoln("No dotfiles found to process")
		fmt.Println("\n📭 No dotfiles to process")
	} else if totalProcessed == 0 && totalFailed == 0 {
		logrus.Infof("All dotfiles are up to date - Skipped: %d", totalSkipped)
		fmt.Println("\n✅ All dotfiles are up to date")
		fmt.Printf("🔄 Skipped: %d files (already identical)\n", totalSkipped)

		// Show detailed breakdown by app
		for appName, fileResults := range result {
			if len(fileResults) > 0 {
				fmt.Printf("\n📦 %s:\n", appName)
				for _, fileResult := range fileResults {
					if fileResult.isSkipped {
						fmt.Printf("  🔄 %s (already identical)\n", fileResult.path)
					}
				}
			}
		}
	} else {
		logrus.Infof("Pull complete - Processed: %d, Skipped: %d, Failed: %d", totalProcessed, totalSkipped, totalFailed)
		fmt.Printf("\n✅ Pull complete\n")
		fmt.Printf("📥 Updated: %d files\n", totalProcessed)
		if totalSkipped > 0 {
			fmt.Printf("🔄 Skipped: %d files (already identical)\n", totalSkipped)
		}
		if totalFailed > 0 {
			fmt.Printf("⚠️  Failed: %d files\n", totalFailed)
		}

		// Show detailed breakdown by app
		for appName, fileResults := range result {
			if len(fileResults) > 0 {
				fmt.Printf("\n📦 %s:\n", appName)
				for _, fileResult := range fileResults {
					if fileResult.isSuccess {
						fmt.Printf("  ✅ %s (updated)\n", fileResult.path)
					} else if fileResult.isFailed {
						fmt.Printf("  ⚠️  %s (failed)\n", fileResult.path)
					} else if fileResult.isSkipped {
						fmt.Printf("  🔄 %s (already identical)\n", fileResult.path)
					}
				}
			}
		}
	}

	return nil
}
