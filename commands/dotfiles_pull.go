package commands

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/malamtime/cli/model"
	"github.com/pterm/pterm"
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

// printPullResults prints the pull operation results in a formatted way
func printPullResults(result map[model.DotfileAppName][]dotfilePullFileResult, dryRun bool) {
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

	// No files to process
	if totalProcessed == 0 && totalFailed == 0 && totalSkipped == 0 {
		slog.Info("No dotfiles found to process")
		pterm.Info.Println("No dotfiles to process")
		return
	}

	// Print header
	fmt.Println()
	if dryRun {
		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).Println("DRY RUN - Dotfiles Pull Summary")
	} else {
		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgGreen)).Println("Dotfiles Pull Summary")
	}

	// Print summary statistics
	summaryData := pterm.TableData{
		{"Status", "Count"},
	}

	if totalProcessed > 0 {
		if dryRun {
			summaryData = append(summaryData, []string{pterm.FgYellow.Sprint("Would Update"), fmt.Sprintf("%d", totalProcessed)})
		} else {
			summaryData = append(summaryData, []string{pterm.FgGreen.Sprint("Updated"), fmt.Sprintf("%d", totalProcessed)})
		}
	}

	if totalFailed > 0 {
		summaryData = append(summaryData, []string{pterm.FgRed.Sprint("Failed"), fmt.Sprintf("%d", totalFailed)})
	}

	if totalSkipped > 0 {
		summaryData = append(summaryData, []string{pterm.FgGray.Sprint("Skipped"), fmt.Sprintf("%d", totalSkipped)})
	}

	pterm.DefaultTable.WithHasHeader().WithData(summaryData).Render()

	// If there are no updates or failures, just show the summary
	if totalProcessed == 0 && totalFailed == 0 {
		pterm.Success.Println("All dotfiles are up to date")
		return
	}

	// Build detailed table for updated and failed files
	var detailsData pterm.TableData
	detailsData = append(detailsData, []string{"App", "File", "Status"})

	// Collect all non-skipped files
	for appName, fileResults := range result {
		for _, fileResult := range fileResults {
			if fileResult.isSkipped {
				continue // Skip files that are already identical
			}

			var status string
			if fileResult.isSuccess {
				if dryRun {
					status = pterm.FgYellow.Sprint("Would Update")
				} else {
					status = pterm.FgGreen.Sprint("Updated")
				}
			} else if fileResult.isFailed {
				status = pterm.FgRed.Sprint("Failed")
			}

			detailsData = append(detailsData, []string{
				string(appName),
				fileResult.path,
				status,
			})
		}
	}

	// Only show details table if there are updated or failed files
	if len(detailsData) > 1 {
		fmt.Println() // Add spacing
		pterm.DefaultSection.Println("File Details")
		pterm.DefaultTable.WithHasHeader().WithData(detailsData).Render()
	}

	// Log for debugging
	slog.Info("Pull complete", slog.Int("processed", totalProcessed), slog.Int("skipped", totalSkipped), slog.Int("failed", totalFailed))
}

func pullDotfiles(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "dotfiles-pull", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	apps := c.StringSlice("apps")
	dryRun := c.Bool("dry-run")
	span.SetAttributes(attribute.StringSlice("apps", apps), attribute.Bool("dry-run", dryRun))

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
	slog.Info("Fetching dotfiles from server...")
	resp, err := model.FetchDotfilesFromServer(ctx, mainEndpoint, filter)
	if err != nil {
		slog.Error("Failed to fetch dotfiles from server", slog.Any("err", err))
		return err
	}

	if resp == nil || len(resp.Data.FetchUser.Dotfiles.Apps) == 0 {
		slog.Info("No dotfiles found on server")
		fmt.Println("\n📭 No dotfiles found on server")
		return nil
	}

	result := map[model.DotfileAppName][]dotfilePullFileResult{}

	for _, appData := range resp.Data.FetchUser.Dotfiles.Apps {
		appName := model.DotfileAppName(appData.App)
		app, exists := appHandlers[appName]
		if !exists {
			slog.Warn("Unknown app type", slog.String("app", appData.App))
			continue
		}

		slog.Info("Processing dotfiles...", slog.String("app", appData.App))

		// Collect files to process for this app
		filesToProcess := make(map[string]string)

		for _, file := range appData.Files {
			if len(file.Records) == 0 {
				slog.Debug("No records found", slog.String("path", file.Path))
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
			slog.Warn("Failed to check file equality", slog.String("app", appData.App), slog.Any("err", err))
		}

		// Filter out files that are already equal
		filesToUpdate := make(map[string]string)
		var pathsToActuallyBackup []string

		for path, content := range filesToProcess {
			if isEqual, exists := equalityMap[path]; exists && isEqual {
				slog.Debug("Skipping - content is identical", slog.String("path", path))
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
			slog.Info("All files are up to date", slog.String("app", appData.App))
			continue
		}

		results := make([]dotfilePullFileResult, 0)

		// Backup files that will be modified (handles dry-run internally)
		if err := app.Backup(ctx, pathsToActuallyBackup, dryRun); err != nil {
			slog.Warn("Failed to backup files", slog.String("app", appData.App), slog.Any("err", err))
		}

		// Save the updated files (handles dry-run internally)
		if err := app.Save(ctx, filesToUpdate, dryRun); err != nil {
			slog.Error("Failed to save files", slog.String("app", appData.App), slog.Any("err", err))
			for f := range filesToUpdate {
				results = append(results, dotfilePullFileResult{
					path:     f,
					isFailed: true,
				})
			}
		} else {
			for f := range filesToUpdate {
				results = append(results, dotfilePullFileResult{
					path:      f,
					isSuccess: true,
				})
			}
		}
		result[appName] = append(result[appName], results...)
	}

	// Print the results
	printPullResults(result, dryRun)

	return nil
}
