package model

import (
	"os"
	"strings"
)

func readSysStat() SysStat {
	var s SysStat
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if b, err := os.ReadFile(path); err == nil {
			s.OSVersion = parseOSRelease(string(b))
			break
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		if release := strings.TrimSpace(string(b)); release != "" {
			s.Kernel = "Linux " + release
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		s.UptimeSec, _ = parseProcUptime(string(b))
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		s.LoadAvg, _ = parseProcLoadavg(string(b))
	}

	switch {
	case os.Getenv("WSL_DISTRO_NAME") != "" || strings.Contains(strings.ToLower(s.Kernel), "microsoft"):
		s.Container = "wsl"
	case fileExists("/run/.containerenv"):
		s.Container = "podman"
	case fileExists("/.dockerenv"):
		s.Container = "docker"
	}
	return s
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
