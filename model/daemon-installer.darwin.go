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
	cmd := exec.Command("launchctl", "print", "gui/"+fmt.Sprintf("%d", os.Getuid())+"/"+m.serviceName)
	if err := cmd.Run(); err == nil {
		return nil
	}
	return fmt.Errorf("service %s is not running", m.serviceName)
}

func (m *MacDaemonInstaller) CheckAndStopExistingService() error {
	color.Yellow.Println("🔍 Checking if service is running...")

	if err := m.Check(); err == nil {
		color.Yellow.Println("🛑 Stopping existing service...")
		currentUser, err := user.Current()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		agentPath := filepath.Join(currentUser.HomeDir, "Library/LaunchAgents", fmt.Sprintf("%s.plist", m.serviceName))
		if err := exec.Command("launchctl", "unload", agentPath).Run(); err != nil {
			return fmt.Errorf("failed to stop existing service: %w", err)
		}
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

	// Create logs directory if not exists
	logsPath := filepath.Join(m.baseFolder, "logs")
	if err := os.MkdirAll(logsPath, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
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

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Create LaunchAgents directory if it doesn't exist
	launchAgentsDir := filepath.Join(currentUser.HomeDir, "Library/LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	plistPath := filepath.Join(launchAgentsDir, fmt.Sprintf("%s.plist", m.serviceName))
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

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	agentPath := filepath.Join(currentUser.HomeDir, "Library/LaunchAgents", fmt.Sprintf("%s.plist", m.serviceName))
	if err := exec.Command("launchctl", "load", agentPath).Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}
	return nil
}

func (m *MacDaemonInstaller) UnregisterService() error {
	if m.baseFolder == "" {
		return fmt.Errorf("base folder is not set")
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	agentPath := filepath.Join(currentUser.HomeDir, "Library/LaunchAgents", fmt.Sprintf("%s.plist", m.serviceName))

	color.Yellow.Println("🛑 Stopping service if running...")
	// Try to stop the service first
	_ = exec.Command("launchctl", "unload", agentPath).Run()

	color.Yellow.Println("🗑  Removing service files...")
	// Remove symlink from LaunchAgents
	if err := os.Remove(agentPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove launch agent plist: %w", err)
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
		"UserName":   username,
		"BaseFolder": m.baseFolder,
	})
	return
}
