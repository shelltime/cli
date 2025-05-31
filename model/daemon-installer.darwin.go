package model

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/gookit/color"
)

//go:embed sys-desc/xyz.shelltime.daemon.plist
var daemonMacServiceDesc []byte

// MacDaemonInstaller implements DaemonInstaller for macOS systems
type MacDaemonInstaller struct {
	baseFolder  string
	serviceName string
	user        string
}

func NewMacDaemonInstaller(baseFolder, user string) *MacDaemonInstaller {
	return &MacDaemonInstaller{
		baseFolder:  baseFolder,
		user:        user,
		serviceName: "xyz.shelltime.daemon",
	}
}

func (m *MacDaemonInstaller) Check() error {
	cmd := exec.Command("launchctl", "print", "system/"+m.serviceName)
	if err := cmd.Run(); err == nil {
		return nil
	}
	return fmt.Errorf("service %s is not running", m.serviceName)
}

func (m *MacDaemonInstaller) CheckAndStopExistingService() error {
	color.Yellow.Println("🔍 Checking if service is running...")

	if err := m.Check(); err != nil {
		return err
	}

	color.Yellow.Println("🛑 Stopping existing service...")
	if err := exec.Command("launchctl", "unload", fmt.Sprintf("/Library/LaunchDaemons/%s.plist", m.serviceName)).Run(); err != nil {
		return fmt.Errorf("failed to stop existing service: %w", err)
	}
	return nil
}

func (m *MacDaemonInstaller) InstallService(username string) error {
	if m.baseFolder == "" {
		return fmt.Errorf("base folder is not set")
	}
	daemonPath := filepath.Join(m.baseFolder, "daemon")
	// Create daemon directory if not exists
	if err := os.MkdirAll(daemonPath, 0755); err != nil {
		return fmt.Errorf("failed to create daemon directory: %w", err)
	}

	plistPath := filepath.Join(daemonPath, fmt.Sprintf("%s.plist", m.serviceName))
	if _, err := os.Stat(plistPath); err == nil {
		if err := os.Remove(plistPath); err != nil {
			return fmt.Errorf("failed to remove existing plist file: %w", err)
		}
	}

	desc, err := m.GetDaemonServiceFile(username)
	if err != nil {
		return err
	}

	if err := os.WriteFile(plistPath, desc.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}
	return nil
}

func (m *MacDaemonInstaller) RegisterService() error {
	if m.baseFolder == "" {
		return fmt.Errorf("base folder is not set")
	}
	plistPath := fmt.Sprintf("/Library/LaunchDaemons/%s.plist", m.serviceName)
	if _, err := os.Stat(plistPath); err != nil {
		sourceFile := filepath.Join(m.baseFolder, fmt.Sprintf("daemon/%s.plist", m.serviceName))
		if err := os.Symlink(sourceFile, plistPath); err != nil {
			return fmt.Errorf("failed to create plist symlink: %w", err)
		}
	}
	return nil
}

func (m *MacDaemonInstaller) StartService() error {
	color.Yellow.Println("🚀 Starting service...")
	if err := exec.Command("launchctl", "load", fmt.Sprintf("/Library/LaunchDaemons/%s.plist", m.serviceName)).Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}
	return nil
}

func (m *MacDaemonInstaller) UnregisterService() error {
	if m.baseFolder == "" {
		return fmt.Errorf("base folder is not set")
	}
	color.Yellow.Println("🛑 Stopping service if running...")
	// Try to stop the service first
	_ = exec.Command("launchctl", "unload", fmt.Sprintf("/Library/LaunchDaemons/%s.plist", m.serviceName)).Run()

	color.Yellow.Println("🗑 Removing service files...")
	// Remove symlink from LaunchDaemons
	if err := os.Remove(fmt.Sprintf("/Library/LaunchDaemons/%s.plist", m.serviceName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove launch daemon plist: %w", err)
	}

	color.Green.Println("✅ Service unregistered successfully")
	return nil
}

func (m *MacDaemonInstaller) GetDaemonServiceFile(username string) (buf bytes.Buffer, err error) {
	tmpl, err := template.New("daemon").Parse(string(daemonMacServiceDesc))
	if err != nil {
		return
	}
	err = tmpl.Execute(&buf, map[string]string{
		"UserName": username,
	})
	return
}
