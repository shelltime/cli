package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

const (
	logFileMaxSize = 100 * 1024 * 1024 // 100MB
)

var DoctorCommand *cli.Command = &cli.Command{
	Name:   "doctor",
	Usage:  "Check the shelltime setup and environment",
	Action: commandDoctor,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

func commandDoctor(c *cli.Context) error {
	ctx := c.Context
	_ = ctx // placeholder for future use with tracing

	color.Cyan.Println("🩺 Running Shelltime Doctor...")

	// 1. Print basic system info
	printSectionHeader("System Information")
	fmt.Printf("  OS: %s\n", runtime.GOOS)
	fmt.Printf("  Arch: %s\n", runtime.GOARCH)

	// 2. Check ~/.shelltime folder
	printSectionHeader("Shelltime Directory")
	shelltimeDir := os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER)
	info, err := os.Stat(shelltimeDir)
	if err != nil {
		if os.IsNotExist(err) {
			printError(fmt.Sprintf("Directory %s does not exist.", shelltimeDir))
		} else {
			printError(fmt.Sprintf("Error accessing directory %s: %v", shelltimeDir, err))
		}
	} else if !info.IsDir() {
		printError(fmt.Sprintf("%s is not a directory.", shelltimeDir))
	} else {
		printSuccess(fmt.Sprintf("Directory %s found and accessible.", shelltimeDir))
	}

	// 3. Check ~/.shelltime/log.log size
	logFilePath := filepath.Join(shelltimeDir, "log.log")
	logInfo, err := os.Stat(logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			printInfo(fmt.Sprintf("Log file %s does not exist.", logFilePath))
		} else {
			printError(fmt.Sprintf("Error accessing log file %s: %v", logFilePath, err))
		}
	} else {
		if logInfo.Size() > logFileMaxSize {
			printWarning(fmt.Sprintf("Log file %s is large (%dMB). Consider archiving or clearing it.", logFilePath, logInfo.Size()/(1024*1024)))
		} else {
			printSuccess(fmt.Sprintf("Log file %s size is normal (%dMB).", logFilePath, logInfo.Size()/(1024*1024)))
		}
	}

	// 4. Check configuration
	printSectionHeader("Configuration")
	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		printError(fmt.Sprintf("Error reading config file: %v", err))
		return err
	}
	printSuccess("Configuration file is valid.")
	if cfg.EnableMetrics != nil && *cfg.EnableMetrics {
		printWarning("Metrics are enabled. it would has performance impact.")
	}

	if cfg.DataMasking != nil && *cfg.DataMasking {
		printSuccess("Data masking is enabled.")
	}
	if cfg.Encrypted != nil && *cfg.Encrypted {
		printSuccess("Encrypted is enabled.")
	}

	// 5. Check daemon process
	printSectionHeader("Daemon Process")
	daemonInstaller, err := model.NewDaemonInstaller("", "")
	if err != nil {
		printError(fmt.Sprintf("Error checking daemon installer: %v", err))
		return err
	}
	if err := daemonInstaller.Check(); err != nil {
		printWarning(fmt.Sprintf("Daemon is not running: %v. it's ok if you haven't installed it yet.", err))
	} else {
		printSuccess("Daemon is running.")
	}

	// 6. Check user's current shell and PATH
	printSectionHeader("Shell Environment")
	currentShell := os.Getenv("SHELL")
	if currentShell == "" {
		printWarning("Could not determine current shell from $SHELL environment variable.")
	} else {
		fmt.Printf("  Current Shell: %s\n", currentShell)
	}

	zshHookService := model.NewZshHookService()
	fishHookService := model.NewFishHookService()
	bashHookService := model.NewBashHookService()

	hookServices := []model.ShellHookService{
		zshHookService,
		fishHookService,
		bashHookService,
	}

	for _, hookService := range hookServices {
		if !hookService.Match(currentShell) {
			continue
		}
		if err := hookService.Check(); err == nil {
			printSuccess(fmt.Sprintf("Hook is already installed for %s.", hookService.ShellName()))
		} else {
			printError(fmt.Sprintf("Hook is NOT installed for %s.", hookService.ShellName()))
			printInfo("Consider running 'shelltime hooks install' to install it.")
		}
	}

	color.Green.Println("\nDoctor check complete.")
	return nil
}

func printSectionHeader(title string) {
	color.Style{color.FgCyan, color.OpBold}.Println("\n" + title)
	fmt.Println(strings.Repeat("-", len(title)))
}

func printSuccess(message string) {
	color.Green.Printf("  ✓ %s\n", message)
}

func printError(message string) {
	color.Red.Printf("  ✗ %s\n", message)
}

func printWarning(message string) {
	color.Yellow.Printf("  ⚠️ %s\n", message)
}

func printInfo(message string) {
	color.Gray.Printf("  ℹ️ %s\n", message)
}
