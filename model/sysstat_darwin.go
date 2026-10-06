package model

import (
	"time"

	"golang.org/x/sys/unix"
)

func readSysStat() SysStat {
	var s SysStat
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil && v != "" {
		s.OSVersion = "macOS " + v
	}
	if v, err := unix.Sysctl("kern.osrelease"); err == nil && v != "" {
		s.Kernel = "Darwin " + v
	}
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		if boot := time.Unix(tv.Unix()); !boot.IsZero() {
			s.UptimeSec = int64(time.Since(boot).Seconds())
		}
	}
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil {
		s.LoadAvg, _ = parseDarwinLoadavg(raw)
	}
	return s
}
