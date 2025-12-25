package commands

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var DaemonStatusCommand = &cli.Command{
	Name:   "status",
	Usage:  "Check the status of the shelltime daemon service",
	Action: commandDaemonStatus,
}

func commandDaemonStatus(c *cli.Context) error {
	ctx := c.Context

	printSectionHeader("Daemon Status")

	// Read config for socket path and feature flags
	cfg, err := configService.ReadConfigFile(ctx)
	socketPath := model.DefaultSocketPath
	if err == nil && cfg.SocketPath != "" {
		socketPath = cfg.SocketPath
	}

	// Check 1: Socket file existence
	socketExists := checkSocketFileExists(socketPath)
	if socketExists {
		printSuccess(fmt.Sprintf("Socket file exists at %s", socketPath))
	} else {
		printError(fmt.Sprintf("Socket file does not exist at %s", socketPath))
	}

	// Check 2: Socket connectivity
	connected, latency, connErr := checkSocketConnection(socketPath, 2*time.Second)
	if connected {
		printSuccess(fmt.Sprintf("Daemon is responding (latency: %v)", latency.Round(time.Microsecond)))
	} else {
		if socketExists {
			printError(fmt.Sprintf("Cannot connect to daemon: %v", connErr))
		} else {
			printError("Cannot connect to daemon (socket not found)")
		}
	}

	// Check 3: Service manager status
	installer, installerErr := model.NewDaemonInstaller("", "")
	if installerErr == nil {
		if err := installer.Check(); err == nil {
			printSuccess("Service is registered and running")
		} else {
			printWarning("Service is not running via system service manager")
		}
	}

	// Configuration section
	printSectionHeader("Configuration")
	fmt.Printf("  Socket Path: %s\n", socketPath)

	if cfg.CCOtel != nil && cfg.CCOtel.Enabled != nil && *cfg.CCOtel.Enabled {
		debugStatus := "off"
		if cfg.CCOtel.Debug != nil && *cfg.CCOtel.Debug {
			debugStatus = "on"
		}
		fmt.Printf("  CCOtel: enabled (port %d, debug %s)\n", cfg.CCOtel.GRPCPort, debugStatus)
	} else {
		fmt.Println("  CCOtel: disabled")
	}

	if cfg.CodeTracking != nil && cfg.CodeTracking.Enabled != nil && *cfg.CodeTracking.Enabled {
		fmt.Println("  Code Tracking: enabled")
	} else {
		fmt.Println("  Code Tracking: disabled")
	}

	// Overall status
	fmt.Println()
	if connected {
		color.Green.Println("Status: Running")
	} else {
		color.Red.Println("Status: Stopped")
		fmt.Println()
		color.Yellow.Println("Run 'shelltime daemon install' to start the daemon service.")
	}

	return nil
}

func checkSocketFileExists(socketPath string) bool {
	_, err := os.Stat(socketPath)
	return err == nil
}

func checkSocketConnection(socketPath string, timeout time.Duration) (bool, time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return false, 0, err
	}
	defer conn.Close()
	latency := time.Since(start)
	return true, latency, nil
}
