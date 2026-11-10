package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var HooksInstallCommand = &cli.Command{
	Name:   "install",
	Usage:  "Install shelltime shell hooks",
	Action: commandHooksInstall,
}

func commandHooksInstall(c *cli.Context) error {
	if !shellHooksInstallable(c.Context) {
		return nil
	}

	color.Yellow.Println("🔍 Starting hooks installation...")
	printHookResults(installShellHooks())
	return nil
}

// shellHooksInstallable reports whether shell hooks can be installed here, after saying
// why not when they can't.
func shellHooksInstallable(ctx context.Context) bool {
	if skippedInAICloudOnly(configForAICloudOnly(ctx), "Shell hooks not installed") {
		return false
	}

	binFolder := os.ExpandEnv(fmt.Sprintf("$HOME/%s/bin", model.COMMAND_BASE_STORAGE_FOLDER))
	_, binFolderErr := os.Stat(binFolder)
	_, lookPathErr := exec.LookPath("shelltime")
	if os.IsNotExist(binFolderErr) && lookPathErr != nil {
		color.Red.Println("📁 shelltime binary not found.")
		color.Red.Println("Install via Homebrew:  brew install shelltime/tap/shelltime")
		color.Red.Println("Or via curl installer: curl -sSL https://shelltime.xyz/i | bash")
		return false
	}
	return true
}

// installShellHooks installs the hooks of the shells the user has: those with a config
// file, and the login shell.
func installShellHooks() []model.HookInstallResult {
	return model.InstallShellHooks(model.ShellHookServices(), os.Getenv("SHELL"))
}

// printHookResults prints a line per shell, for `hooks install` and `init --verbose`.
func printHookResults(results []model.HookInstallResult) {
	var skipped []string
	for _, r := range results {
		switch r.Status {
		case model.HookInstalled:
			color.Green.Printf("✅ Installed the %s hook in %s\n", r.Shell, tildePath(r.ConfigPath))
		case model.HookAlreadyInstalled:
			color.Green.Printf("✅ The %s hook is already installed in %s\n", r.Shell, tildePath(r.ConfigPath))
		case model.HookFailed:
			color.Red.Printf("❌ Failed to install the %s hook: %v\n", r.Shell, r.Err)
		case model.HookSkipped:
			skipped = append(skipped, r.Shell)
		}
	}

	switch {
	case len(skipped) == len(results):
		printNoShellFound(results)
	case len(skipped) > 0:
		color.Gray.Printf("Skipped %s: no config file, and not your login shell.\n", joinWithAnd(skipped))
	}
}

// printHookSummary prints init's line for the shell hooks: the shells that have the hook,
// and a line per shell that failed. Skipped shells aren't mentioned.
func printHookSummary(results []model.HookInstallResult) {
	var hooked []string
	failed := false
	for _, r := range results {
		switch r.Status {
		case model.HookInstalled, model.HookAlreadyInstalled:
			hooked = append(hooked, r.Shell)
		case model.HookFailed:
			failed = true
			color.Red.Printf("❌ Failed to install the %s hook: %v\n", r.Shell, r.Err)
		}
	}

	if len(hooked) > 0 {
		color.Green.Printf("✅ Shell hooks installed for %s\n", joinWithAnd(hooked))
	} else if !failed {
		printNoShellFound(results)
	}
}

// printNoShellFound explains why no shell got the hook: none of the config files exists,
// and $SHELL is none of the supported shells.
func printNoShellFound(results []model.HookInstallResult) {
	paths := make([]string, 0, len(results))
	for _, r := range results {
		paths = append(paths, tildePath(r.ConfigPath))
	}
	color.Yellow.Printf("⚠️  No shell hook installed: none of %s exists, and $SHELL isn't zsh, fish or bash.\n", strings.Join(paths, ", "))
	color.Yellow.Println("Create your shell's config file, then run `shelltime hooks install`.")
}

// joinWithAnd joins names as "a", "a and b" or "a, b and c".
func joinWithAnd(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// tildePath shows a path under the home directory as ~/...
func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return path
	}
	if rel, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rel
	}
	return path
}
