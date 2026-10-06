package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
)

const (
	// queryContextTimeout bounds how long `shelltime q` spends collecting
	// context before it asks the AI. Collectors that miss it are dropped.
	queryContextTimeout = 500 * time.Millisecond
	// queryGitTimeout is shorter so a slow `git status` is cut off early
	// enough for the HEAD fallback to make the overall deadline.
	queryGitTimeout = 400 * time.Millisecond
)

// queryContextTools are probed on PATH and reported when found. Standard
// POSIX utilities are deliberately absent: the model may assume those.
var queryContextTools = []string{
	"git", "gh", "glab", "rg", "fd", "fdfind", "fzf", "jq", "yq", "bat", "batcat", "eza", "lsd", "delta",
	"zoxide", "httpie", "http", "xh", "tldr", "ncdu", "dust", "duf", "btop", "htop", "procs", "hyperfine",
	"docker", "podman", "kubectl", "helm", "k9s", "terraform", "tofu", "aws", "gcloud", "az",
	"brew", "apt", "dnf", "yum", "pacman", "yay", "paru", "apk", "zypper", "nix", "port",
	"node", "npm", "pnpm", "yarn", "bun", "deno", "python3", "uv", "pipx", "poetry",
	"go", "cargo", "rustup", "java", "ruby", "php",
	"make", "just", "task", "tmux", "zellij",
	"pbcopy", "wl-copy", "xclip", "xsel",
}

// queryContextDarwinTools are GNU variants commonly installed via Homebrew;
// their presence tells the model GNU flags are available on macOS.
var queryContextDarwinTools = []string{"gsed", "gawk", "gdate", "gfind", "ggrep", "gtimeout", "gstat", "greadlink"}

// knownShells are parent process names accepted as the user's current shell.
var knownShells = map[string]bool{
	"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true, "ksh": true, "mksh": true,
	"tcsh": true, "csh": true, "nu": true, "pwsh": true, "elvish": true, "xonsh": true,
}

// Test seams.
var (
	gatherQueryContextFn = gatherQueryContext
	parentProcessNameFn  = parentProcessName
)

// gatherQueryContext collects the context sent with `shelltime q` when
// ai.shareContext is enabled. Collectors run concurrently and each returns
// a closure that the loop below applies, so only this goroutine touches qc.
func gatherQueryContext(ctx context.Context, pwd string) *model.QueryContext {
	ctx, cancel := context.WithTimeout(ctx, queryContextTimeout)
	defer cancel()

	home, _ := os.UserHomeDir()
	collectors := []func() func(*model.QueryContext){
		func() func(*model.QueryContext) {
			system := collectSystemContext(os.Getenv)
			return func(qc *model.QueryContext) { qc.System = system }
		},
		func() func(*model.QueryContext) {
			tools := lookupTools(ctx)
			return func(qc *model.QueryContext) { qc.Tools = tools }
		},
		func() func(*model.QueryContext) {
			// Listing the home directory adds noise rather than signal
			if pwd == "" || pwd == home {
				return nil
			}
			dir := model.ListDir(pwd)
			return func(qc *model.QueryContext) { qc.Dir = dir }
		},
		func() func(*model.QueryContext) {
			project := model.DetectProject(pwd, home)
			return func(qc *model.QueryContext) { qc.Project = project }
		},
		func() func(*model.QueryContext) {
			gitCtx, gitCancel := context.WithTimeout(ctx, queryGitTimeout)
			defer gitCancel()
			git := daemon.GetGitContext(gitCtx, pwd)
			return func(qc *model.QueryContext) { qc.Git = git }
		},
	}

	results := make(chan func(*model.QueryContext), len(collectors))
	for _, collect := range collectors {
		go func() { results <- collect() }()
	}

	qc := &model.QueryContext{}
	for range collectors {
		select {
		case apply := <-results:
			if apply != nil {
				apply(qc)
			}
		case <-ctx.Done():
			return qc
		}
	}
	return qc
}

func collectSystemContext(getenv func(string) string) *model.QuerySystemContext {
	st := model.ReadSysStat()
	return &model.QuerySystemContext{
		OSVersion:   model.SanitizeContextString(st.OSVersion, model.QueryContextMaxRunes),
		Kernel:      model.SanitizeContextString(st.Kernel, model.QueryContextMaxRunes),
		Arch:        runtime.GOARCH,
		CPUCount:    runtime.NumCPU(),
		UptimeSec:   st.UptimeSec,
		LoadAvg:     st.LoadAvg,
		IsRoot:      os.Geteuid() == 0,
		SSH:         getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "",
		Container:   st.Container,
		Multiplexer: detectMultiplexer(getenv),
		TermProgram: model.SanitizeContextString(getenv("TERM_PROGRAM"), 64),
		Display:     detectDisplay(runtime.GOOS, getenv),
		Timezone:    localTimezone(getenv),
		LocalTime:   time.Now().Format(time.RFC3339),
	}
}

func detectMultiplexer(getenv func(string) string) string {
	switch {
	case getenv("TMUX") != "":
		return "tmux"
	case getenv("ZELLIJ") != "":
		return "zellij"
	case getenv("STY") != "":
		return "screen"
	}
	return ""
}

func detectDisplay(goos string, getenv func(string) string) string {
	if goos != "linux" {
		return ""
	}
	switch {
	case getenv("WAYLAND_DISPLAY") != "":
		return "wayland"
	case getenv("DISPLAY") != "":
		return "x11"
	}
	return ""
}

// localTimezone returns an IANA zone name from $TZ or the /etc/localtime
// symlink, falling back to the zone abbreviation.
func localTimezone(getenv func(string) string) string {
	if tz := strings.TrimPrefix(getenv("TZ"), ":"); tz != "" && !strings.HasPrefix(tz, "/") {
		return model.SanitizeContextString(tz, 64)
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok && zone != "" {
			return model.SanitizeContextString(zone, 64)
		}
	}
	name, _ := time.Now().Zone()
	return name
}

func lookupTools(ctx context.Context) []string {
	candidates := queryContextTools
	if runtime.GOOS == "darwin" {
		candidates = append(append([]string{}, queryContextTools...), queryContextDarwinTools...)
	}
	var found []string
	for _, name := range candidates {
		if ctx.Err() != nil {
			break
		}
		if _, err := exec.LookPath(name); err == nil {
			found = append(found, name)
		}
	}
	return found
}

// currentShell returns the shell the user is typing in: the parent process
// when it is a known shell, otherwise the login shell from $SHELL. They
// differ when, say, fish is started from a zsh login shell.
func currentShell() string {
	if name := parentProcessNameFn(); name != "" {
		name = strings.TrimPrefix(filepath.Base(name), "-")
		if knownShells[name] {
			return name
		}
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "unknown"
	}
	return filepath.Base(shell)
}

// parentProcessName returns the command name of the parent process, or ""
// when it cannot be determined cheaply.
func parentProcessName() string {
	ppid := os.Getppid()
	if ppid <= 1 {
		return ""
	}
	switch runtime.GOOS {
	case "linux":
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", ppid))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		out, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(ppid), "-o", "comm=").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return ""
}
