package model

import (
	"os/exec"
	"runtime"
	"strings"
)

type SysInfo struct {
	Os      string
	Version string
}

func GetOSAndVersion() (*SysInfo, error) {
	os := runtime.GOOS
	if os == "windows" {
		cmd := exec.Command("cmd", "/c", "ver")
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		return &SysInfo{
			Os:      os,
			Version: strings.TrimSpace(string(out)),
		}, nil
	}
	if os == "darwin" {
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err != nil {
			return nil, err
		}
		return &SysInfo{
			Os:      os,
			Version: strings.TrimSpace(string(out)),
		}, nil
	}
	if os == "linux" {
		cmd := exec.Command("lsb_release", "-a")
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		lines := strings.Split(string(out), "\n")
		var distro, version string
		for _, line := range lines {
			if strings.HasPrefix(line, "Distributor ID:") {
				distro = strings.TrimSpace(strings.TrimPrefix(line, "Distributor ID:"))
			} else if strings.HasPrefix(line, "Release:") {
				version = strings.TrimSpace(strings.TrimPrefix(line, "Release:"))
			}
		}

		return &SysInfo{
			Os:      distro,
			Version: version,
		}, nil
	}

	return &SysInfo{
		Os:      "unknown",
		Version: "unknown",
	}, nil
}

// IsSSHSession reports whether the current shell is an SSH login. sshd exports
// these variables into the login shell, and every child (like `shelltime track`)
// inherits them.
func IsSSHSession(getenv func(string) string) bool {
	return getenv("SSH_CONNECTION") != "" || getenv("SSH_CLIENT") != "" || getenv("SSH_TTY") != ""
}
