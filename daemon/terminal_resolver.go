package daemon

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Known terminal emulator process names (lowercase for case-insensitive matching)
var knownTerminals = []string{
	// macOS
	"terminal",
	"iterm2",
	"alacritty",
	"kitty",
	"wezterm",
	"hyper",
	"tabby",
	"warp",
	"ghostty",
	// Linux
	"gnome-terminal",
	"konsole",
	"xfce4-terminal",
	"xterm",
	"urxvt",
	"rxvt",
	"terminator",
	"tilix",
	"foot",
	// IDE terminals
	"code",
	"cursor",
}

// Known terminal multiplexer process names (lowercase for case-insensitive matching)
var knownMultiplexers = []string{
	"tmux",
	"screen",
	"zellij",
}

// Known remote/container process names (lowercase for case-insensitive matching)
var knownRemote = []string{
	"sshd",
	"docker",
	"containerd",
}

// matchKnownName checks if processName contains any of the known names (case-insensitive)
// Returns the matched known name if found, empty string otherwise
func matchKnownName(processName string, knownNames []string) string {
	lowerName := strings.ToLower(processName)
	for _, known := range knownNames {
		if strings.Contains(lowerName, known) {
			return known
		}
	}
	return ""
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
		if multiplexer == "" {
			if matched := matchKnownName(processName, knownMultiplexers); matched != "" {
				multiplexer = matched
			}
		}

		// Check for terminals
		if terminal == "" {
			if matched := matchKnownName(processName, knownTerminals); matched != "" {
				terminal = matched
			}
		}

		// Check for remote connections
		if terminal == "" {
			if matched := matchKnownName(processName, knownRemote); matched != "" {
				terminal = matched
			}
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
