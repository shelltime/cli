package commands

import (
	"errors"
	"os"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
)

// agentHooksFile is an agent's hook file ShellTime registers in or removes itself from.
type agentHooksFile interface {
	Install() error
	Uninstall() error
	Status() (model.AgentHooksStatus, error)
	Path() string
}

// agentHooks describes one agent for `<agent> install` and `<agent> uninstall`.
type agentHooks struct {
	name string // the agent, as messages name it
	// project is the hook file of the project at root, the one ShellTime installs into
	project func(root string) agentHooksFile
	// user is the agent's user-level hook file, where older versions installed the hooks; read
	// when called, since it lives under $HOME
	user func() agentHooksFile
	// commit says why the project file is committed
	commit string
	// restart says what to restart for the hooks to apply
	restart string
}

var cursorHooks = agentHooks{
	name:    "Cursor",
	project: func(root string) agentHooksFile { return model.NewCursorProjectHooksService(root) },
	user:    func() agentHooksFile { return model.NewCursorHooksService() },
	commit:  "Commit it: Cursor Cloud Agents run only the hooks in the repository.",
	restart: "Restart Cursor (the IDE and any running cursor-agent sessions) to apply them.",
}

var antigravityHooks = agentHooks{
	name:    "Antigravity",
	project: func(root string) agentHooksFile { return model.NewAntigravityProjectHooksService(root) },
	user:    func() agentHooksFile { return model.NewAntigravityHooksService() },
	commit:  "Commit it: the Antigravity agent reads .agents/hooks.json from the repository its Gemini API sandbox mounts.",
	restart: "Restart Antigravity (the app and any running agy sessions) to apply them.",
}

// installAgentHooks registers ShellTime in the hook file of the project the working directory
// is in, then removes the hooks an older version installed in the agent's user-level file, so
// the project's are the only ones.
func installAgentHooks(a agentHooks) error {
	color.Yellow.Printf("Installing %s hooks...\n", a.name)

	root, err := currentProjectRoot()
	if err != nil {
		color.Red.Printf("Failed to find the project: %v\n", err)
		return err
	}
	svc := a.project(root)
	if err := svc.Install(); err != nil {
		color.Red.Printf("Failed to install the %s hooks: %v\n", a.name, err)
		return err
	}
	color.Green.Printf("%s hooks installed in %s\n", a.name, svc.Path())
	// Reported inside; the project's hooks don't depend on it
	_ = removeUserAgentHooks(a)

	color.Yellow.Println(a.commit)
	color.Yellow.Println(a.restart)
	printProjectHooksNote()
	return nil
}

// removeUserAgentHooks removes the ShellTime hooks an older version installed in the agent's
// user-level file. A file it can't read is reported and left alone: the project's hooks leave
// events to working ones there, so nothing is counted twice.
func removeUserAgentHooks(a agentHooks) error {
	user := a.user()
	status, err := user.Status()
	if err == nil && !status.Installed {
		return nil
	}
	if err == nil {
		err = user.Uninstall()
	}
	if err != nil {
		color.Yellow.Printf("Couldn't remove the ShellTime hooks an older version put in %s: %v\n", user.Path(), err)
		return err
	}
	color.Green.Printf("Removed the ShellTime hooks an older version put in %s\n", user.Path())
	return nil
}

// uninstallAgentHooks removes ShellTime from the hook file of the project the working directory
// is in, and from the agent's user-level file. Outside a project only the user-level file is
// cleaned.
func uninstallAgentHooks(a agentHooks) error {
	color.Yellow.Printf("Removing %s hooks...\n", a.name)

	root, err := currentProjectRoot()
	switch {
	case err == nil:
		svc := a.project(root)
		if err := svc.Uninstall(); err != nil {
			color.Red.Printf("Failed to remove the %s hooks: %v\n", a.name, err)
			return err
		}
		color.Green.Printf("%s hooks removed from %s\n", a.name, svc.Path())
	case !errors.Is(err, model.ErrHomeIsNotAProject):
		color.Red.Printf("Failed to find the project: %v\n", err)
		return err
	}

	user := a.user()
	if status, err := user.Status(); err == nil && !status.Installed {
		return nil
	}
	if err := user.Uninstall(); err != nil {
		color.Red.Printf("Failed to remove the %s hooks: %v\n", a.name, err)
		return err
	}
	color.Green.Printf("%s hooks removed from %s\n", a.name, user.Path())
	return nil
}

// currentProjectRoot is the root of the project the working directory is in, where agents look
// for project hook files.
func currentProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return model.ProjectHooksRoot(wd)
}

// printProjectHooksNote says what a committed project hook file does on other machines and what
// a cloud environment needs for it.
func printProjectHooksNote() {
	color.Gray.Println("The hooks record events only where shelltime and shelltime-daemon are installed (on PATH, or in")
	color.Gray.Println("~/.shelltime/bin), each to that machine's account; anywhere else they let the agent carry on.")
	color.Gray.Println("In a cloud environment, install shelltime, sign it in (`shelltime auth --token …`), keep")
	color.Gray.Println("shelltime-daemon running and set SHELLTIME_CLOUD=true, so its sessions show as cloud sessions.")
}

// printAgentHooksPrivacyNote says what the Cursor or Antigravity hooks send.
func printAgentHooksPrivacyNote(agent string) {
	color.Gray.Printf("Privacy: the %s hooks send your prompts, the agent's responses, tool names and inputs\n", agent)
	color.Gray.Println("(shell commands, file paths) and the workspace folder to your ShellTime account. Data masking")
	color.Gray.Println("and your exclude patterns apply. To keep prompt, response and tool input text out and send only")
	color.Gray.Println("the activity, run `shelltime config disable agentHooks.sendContent`.")
}
