package model

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"text/template"

	"github.com/gookit/color"
)

//go:embed sys-desc/shelltime.service
var daemonLinuxServiceDesc []byte

// LinuxDaemonInstaller implements DaemonInstaller for Linux systems
type LinuxDaemonInstaller struct {
	baseFolder string
	user       string
}

func NewLinuxDaemonInstaller(baseFolder, user string) *LinuxDaemonInstaller {
	return &LinuxDaemonInstaller{baseFolder: baseFolder, user: user}
}

func (l *LinuxDaemonInstaller) Check() error {
	cmd := exec.Command("systemctl", "--user", "is-active", "shelltime")
	if err := cmd.Run(); err == nil {
		return nil
	}
	return fmt.Errorf("service shelltime is not running")
}

func (l *LinuxDaemonInstaller) CheckAndStopExistingService() error {
	color.Yellow.Println("🔍 Checking if service is running...")

	if err := l.Check(); err == nil {
		color.Yellow.Println("🛑 Stopping existing service...")
		currentUser, err := user.Current()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		servicePath := filepath.Join(currentUser.HomeDir, ".config/systemd/user/shelltime.service")
		if err := exec.Command("systemctl", "--user", "stop", "shelltime").Run(); err != nil {
			return fmt.Errorf("failed to stop existing service: %w", err)
		}
		// Also disable to clean up
		_ = exec.Command("systemctl", "--user", "disable", "shelltime").Run()
		// Remove old symlink if exists
		_ = os.Remove(servicePath)
	}
	return nil
}

func (l *LinuxDaemonInstaller) InstallService(username string) error {
	if l.baseFolder == "" {
		return fmt.Errorf("base folder is not set")
	}
	daemonPath := filepath.Join(l.baseFolder, "daemon")
	// Create daemon directory if not exists
	if err := os.MkdirAll(daemonPath, 0755); err != nil {
		return fmt.Errorf("failed to create daemon directory: %w", err)
	}

	servicePath := filepath.Join(daemonPath, "shelltime.service")
	if _, err := os.Stat(servicePath); err == nil {
		if err := os.Remove(servicePath); err != nil {
			return fmt.Errorf("failed to remove existing service file: %w", err)
		}
	}

	desc, err := l.GetDaemonServiceFile(username)
	if err != nil {
		return err
	}

	if err := os.WriteFile(servicePath, desc.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	return nil
}

func (l *LinuxDaemonInstaller) RegisterService() error {
	if l.baseFolder == "" {
		return fmt.Errorf("base folder is not set")
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Create systemd user directory if it doesn't exist
	systemdUserDir := filepath.Join(currentUser.HomeDir, ".config/systemd/user")
	if err := os.MkdirAll(systemdUserDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd user directory: %w", err)
	}

	servicePath := filepath.Join(systemdUserDir, "shelltime.service")
	if _, err := os.Stat(servicePath); err != nil {
		sourceFile := filepath.Join(l.baseFolder, "daemon/shelltime.service")
		if err := os.Symlink(sourceFile, servicePath); err != nil {
			return fmt.Errorf("failed to create service symlink: %w", err)
		}
	}
	return nil
}

func (l *LinuxDaemonInstaller) StartService() error {
	color.Yellow.Println("🔄 Reloading systemd...")
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	color.Yellow.Println("✨ Enabling service...")
	if err := exec.Command("systemctl", "--user", "enable", "shelltime").Run(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	color.Yellow.Println("🚀 Starting service...")
	if err := exec.Command("systemctl", "--user", "start", "shelltime").Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}
	return nil
}

func (l *LinuxDaemonInstaller) UnregisterService() error {
	if l.baseFolder == "" {
		return fmt.Errorf("base folder is not set")
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	servicePath := filepath.Join(currentUser.HomeDir, ".config/systemd/user/shelltime.service")

	color.Yellow.Println("🛑 Stopping and disabling service if running...")
	// Try to stop and disable the service
	_ = exec.Command("systemctl", "--user", "stop", "shelltime").Run()
	_ = exec.Command("systemctl", "--user", "disable", "shelltime").Run()

	color.Yellow.Println("🗑  Removing service files...")
	// Remove symlink from systemd
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove systemd service symlink: %w", err)
	}

	color.Yellow.Println("🔄 Reloading systemd...")
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	color.Green.Println("✅ Service unregistered successfully")
	return nil
}

func (l *LinuxDaemonInstaller) GetDaemonServiceFile(username string) (buf bytes.Buffer, err error) {
	tmpl, err := template.New("daemon").Parse(string(daemonLinuxServiceDesc))
	if err != nil {
		return
	}
	err = tmpl.Execute(&buf, map[string]string{
		"UserName": username,
	})
	return
}
