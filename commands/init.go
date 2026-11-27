package commands

import (
	"bytes"
	"io"
	"os"
	"os/exec"

	"github.com/gookit/color"
	"github.com/urfave/cli/v2"
)

var InitCommand *cli.Command = &cli.Command{
	Name:  "init",
	Usage: "Initialize shelltime: authenticate, install hooks, start daemon, and configure AI code integrations",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "token",
			Aliases:  []string{"t"},
			Usage:    "Authentication token",
			Required: false,
		},
		emailFlag,
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "Print every step's details instead of one line per step",
		},
	},
	Action: commandInit,
}

func commandInit(c *cli.Context) error {
	verbose := c.Bool("verbose")
	color.Yellow.Println("Initializing ShellTime...")

	// Step 1: Authenticate
	if err := authenticate(c, verbose); err != nil {
		return err
	}
	if !verbose {
		color.Green.Println("✅ Signed in")
	}

	// Step 2: Install shell hooks. A shell that fails doesn't stop the other steps.
	if verbose {
		if err := commandHooksInstall(c); err != nil {
			return err
		}
	} else if shellHooksInstallable(c.Context) {
		printHookSummary(installShellHooks())
	}

	// Step 3: Install daemon service
	if err := runInitStep(verbose, func() error { return commandDaemonInstall(c) }); err != nil {
		color.Red.Printf("❌ Failed to install the daemon: %v\n", err)
		return err
	}
	if !verbose {
		color.Green.Println("✅ Daemon installed and running")
	}

	// Steps 4 and 5: Claude Code and Codex OTEL configuration
	ccErr := runInitStep(verbose, func() error {
		_, err := installCCOtelConfig()
		return err
	})
	codexErr := runInitStep(verbose, installCodexOtelConfig)
	agents := []agentSetupStep{
		{name: "Claude Code", config: "Claude Code OTEL config", err: ccErr},
		{name: "Codex", config: "Codex OTEL config", err: codexErr},
	}

	printAgentTelemetrySummary(verbose, agents)
	// Step 6: the agents without OTEL take hooks in each project's repository, which init has
	// none of, so it says how to add them on machines that have the agents
	printProjectAgentHooksHint(initLookPath)

	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.name
	}
	color.Println()
	color.Green.Println("ShellTime is ready.")
	color.Normal.Printf("Open a new terminal to start tracking commands, and restart any running %s sessions.\n", joinWithOr(names))
	return nil
}

// initLookPath finds the coding agents init sets up; replaced in tests.
var initLookPath = exec.LookPath

// agentSetupStep is the outcome of setting up one coding agent's telemetry in init.
type agentSetupStep struct {
	name   string // the agent, e.g. Claude Code
	config string // what was installed, e.g. Claude Code OTEL config
	err    error
}

// initStepOutput is where a quiet step's held-back output goes when the step fails.
// Replaced in tests.
var initStepOutput io.Writer = os.Stdout

// runInitStep runs one step of init. With --verbose the step prints everything, as its
// own command does. Otherwise its progress lines, all printed through gookit/color, are
// held back: dropped when the step succeeds, printed when it fails, so a failure still
// shows what led to it.
func runInitStep(verbose bool, step func() error) error {
	if verbose {
		return step()
	}

	var held bytes.Buffer
	color.SetOutput(&held)
	err := func() error {
		defer color.ResetOutput()
		return step()
	}()
	if err != nil {
		_, _ = initStepOutput.Write(held.Bytes())
	}
	return err
}

// printProjectAgentHooksHint says how to track the detected agents whose hooks go into each
// project's repository: Cursor and Antigravity. Not on Windows, where their install refuses.
func printProjectAgentHooksHint(lookPath func(string) (string, error)) {
	if agentHooksGOOS == "windows" {
		return
	}
	var agents, commands []string
	if cursorDetected(lookPath) {
		agents = append(agents, "Cursor")
		commands = append(commands, "`shelltime cursor install`")
	}
	if antigravityDetected(lookPath) {
		agents = append(agents, "Antigravity")
		commands = append(commands, "`shelltime agy install`")
	}
	if len(agents) == 0 {
		return
	}
	color.Yellow.Printf("To track %s, run %s in each project's repository and commit the hook file.\n", joinWithAnd(agents), joinWithAnd(commands))
}

// printAgentTelemetrySummary reports the coding agent steps: a line per failure, and,
// quiet, one line for the ones that worked.
func printAgentTelemetrySummary(verbose bool, agents []agentSetupStep) {
	var configured []string
	for _, a := range agents {
		if a.err != nil {
			color.Red.Printf("❌ Failed to install %s: %v\n", a.config, a.err)
			continue
		}
		configured = append(configured, a.name)
	}
	if verbose || len(configured) == 0 {
		return
	}
	color.Green.Printf("✅ %s telemetry configured\n", joinWithAnd(configured))
}
