package model

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
)

// SysStat holds machine facts that `shelltime q` sends as context.
type SysStat struct {
	OSVersion string
	Kernel    string
	UptimeSec int64
	LoadAvg   []float64
	// Container is "wsl", "podman" or "docker" when running inside one.
	Container string
}

// ReadSysStat collects SysStat without forking. Fields that cannot be read
// on the current platform are left empty.
func ReadSysStat() SysStat {
	return readSysStat()
}

// parseOSRelease returns a readable OS name from /etc/os-release content:
// PRETTY_NAME, or NAME plus VERSION_ID.
func parseOSRelease(content string) string {
	values := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		values[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if v := values["PRETTY_NAME"]; v != "" {
		return v
	}
	return strings.TrimSpace(values["NAME"] + " " + values["VERSION_ID"])
}

// parseProcUptime parses /proc/uptime ("12345.67 54321.00").
func parseProcUptime(content string) (int64, bool) {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return 0, false
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || secs < 0 {
		return 0, false
	}
	return int64(secs), true
}

// parseProcLoadavg parses the 1, 5 and 15 minute averages from /proc/loadavg.
func parseProcLoadavg(content string) ([]float64, bool) {
	fields := strings.Fields(content)
	if len(fields) < 3 {
		return nil, false
	}
	loads := make([]float64, 3)
	for i := range loads {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return nil, false
		}
		loads[i] = v
	}
	return loads, true
}

// parseDarwinLoadavg decodes the vm.loadavg sysctl: struct loadavg
// { fixpt_t ldavg[3]; long fscale; }, i.e. three uint32 values, four bytes
// of padding and an int64 scale on 64-bit little-endian Macs.
func parseDarwinLoadavg(raw []byte) ([]float64, bool) {
	if len(raw) < 24 {
		return nil, false
	}
	scale := float64(int64(binary.LittleEndian.Uint64(raw[16:24])))
	if scale <= 0 {
		return nil, false
	}
	loads := make([]float64, 3)
	for i := range loads {
		v := float64(binary.LittleEndian.Uint32(raw[i*4:])) / scale
		loads[i] = math.Round(v*100) / 100
	}
	return loads, true
}
