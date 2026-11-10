package commands

import (
	"bytes"
	"io"
	"os"

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
	printAgentTelemetrySummary(verbose, ccErr, codexErr)

	color.Println()
	color.Green.Println("ShellTime is ready.")
	color.Normal.Println("Open a new terminal to start tracking commands, and restart any running Claude Code or Codex sessions.")
	return nil
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

// printAgentTelemetrySummary reports the Claude Code and Codex steps: a line per
// failure, and, quiet, one line for the ones that worked.
func printAgentTelemetrySummary(verbose bool, ccErr, codexErr error) {
	if ccErr != nil {
		color.Red.Printf("❌ Failed to install Claude Code OTEL config: %v\n", ccErr)
	}
	if codexErr != nil {
		color.Red.Printf("❌ Failed to install Codex OTEL config: %v\n", codexErr)
	}
	if verbose {
		return
	}

	switch {
	case ccErr == nil && codexErr == nil:
		color.Green.Println("✅ Claude Code and Codex telemetry configured")
	case ccErr == nil:
		color.Green.Println("✅ Claude Code telemetry configured")
	case codexErr == nil:
		color.Green.Println("✅ Codex telemetry configured")
	}
}
