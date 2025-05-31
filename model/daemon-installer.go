package model

import (
	"bytes"
	"fmt"
	"runtime"
)

// DaemonInstaller interface defines methods for daemon installation across different platforms
type DaemonInstaller interface {
	Check() error
	CheckAndStopExistingService() error
	InstallService(username string) error
	RegisterService() error
	StartService() error
	UnregisterService() error
	GetDaemonServiceFile(username string) (bytes.Buffer, error)
}

// Factory function to create appropriate installer based on OS
func NewDaemonInstaller(baseFolder, username string) (DaemonInstaller, error) {
	switch runtime.GOOS {
	case "linux":
		return NewLinuxDaemonInstaller(baseFolder, username), nil
	case "darwin":
		return NewMacDaemonInstaller(baseFolder, username), nil
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}
