package daemon

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Known terminal emulator process names
var knownTerminals = map[string]bool{
	// macOS
	"Terminal":        true,
	"iTerm2":          true,
	"Alacritty":       true,
	"alacritty":       true,
	"kitty":           true,
	"WezTerm":         true,
	"wezterm":         true,
	"wezterm-gui":     true,
	"Hyper":           true,
	"Tabby":           true,
	"Warp":            true,
	"Ghostty":         true,
	"ghostty":         true,
	// Linux
	"gnome-terminal":  true,
	"gnome-terminal-": true, // gnome-terminal-server
	"konsole":         true,
	"xfce4-terminal":  true,
	"xterm":           true,
	"urxvt":           true,
	"rxvt":            true,
	"terminator":      true,
	"tilix":           true,
	"st":              true,
	"foot":            true,
	"footclient":      true,
	// IDE terminals
	"code":            true,
	"Code":            true,
	"cursor":          true,
	"Cursor":          true,
}

// Known terminal multiplexer process names
var knownMultiplexers = map[string]bool{
	"tmux":   true,
	"screen": true,
	"zellij": true,
}

// Known remote/container process names
var knownRemote = map[string]bool{
	"sshd":       true,
	"docker":     true,
	"containerd": true,
}

// ResolveTerminal walks up the process tree starting from ppid
// to find the terminal emulator and multiplexer separately.
// Returns (terminal, multiplexer) as separate values.
func ResolveTerminal(ppid int) (terminal string, multiplexer string) {
	if ppid <= 0 {
		return "", ""
	}

	currentPID := ppid
	visited := make(map[int]bool)

	// Walk up the process tree (max 10 levels to prevent infinite loops)
	for i := 0; i < 10; i++ {
		if currentPID <= 1 || visited[currentPID] {
			break
		}
		visited[currentPID] = true

		processName := getProcessName(currentPID)
		if processName == "" {
			break
		}

		// Check for multiplexers first (they're closer to the shell)
		if multiplexer == "" && knownMultiplexers[processName] {
			multiplexer = processName
		}

		// Check for terminals
		if terminal == "" && knownTerminals[processName] {
			terminal = processName
		}

		// Check for remote connections
		if terminal == "" && knownRemote[processName] {
			terminal = processName
		}

		// If we found a terminal, we can stop
		if terminal != "" {
			break
		}

		// Get parent PID and continue
		parentPID := getParentPID(currentPID)
		if parentPID <= 1 || parentPID == currentPID {
			break
		}
		currentPID = parentPID
	}

	// If neither found, return "unknown" for terminal
	if terminal == "" && multiplexer == "" {
		return "unknown", ""
	}

	return terminal, multiplexer
}

// getProcessName returns the process name for the given PID
func getProcessName(pid int) string {
	switch runtime.GOOS {
	case "darwin":
		// macOS: ps -p <pid> -o comm=
		out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
		if err != nil {
			return ""
		}
		name := strings.TrimSpace(string(out))
		// Remove path prefix if present
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		return name

	case "linux":
		// Linux: /proc/<pid>/comm
		out, err := exec.Command("cat", "/proc/"+strconv.Itoa(pid)+"/comm").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	return ""
}

// getParentPID returns the parent process ID for the given PID
func getParentPID(pid int) int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "ppid=").Output()
		if err != nil {
			return 0
		}
		ppid, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return 0
		}
		return ppid

	case "linux":
		out, err := exec.Command("cat", "/proc/"+strconv.Itoa(pid)+"/stat").Output()
		if err != nil {
			return 0
		}
		// /proc/pid/stat format: pid (comm) state ppid ...
		// Find the closing ) and get the 4th field after it
		data := string(out)
		idx := strings.LastIndex(data, ")")
		if idx < 0 {
			return 0
		}
		fields := strings.Fields(data[idx+1:])
		if len(fields) < 2 {
			return 0
		}
		ppid, _ := strconv.Atoi(fields[1])
		return ppid
	}

	return 0
}
