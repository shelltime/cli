package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

// ErrDoctorFoundProblems is returned by `shelltime doctor` when at least one check fails, so the
// process exits non-zero.
var ErrDoctorFoundProblems = errors.New("shelltime doctor found problems")

var DoctorCommand *cli.Command = &cli.Command{
	Name:  "doctor",
	Usage: "Diagnose the shelltime setup and show how to fix any problems",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "offline",
			Usage: "skip checks that need the network (token, encryption key, latest version)",
		},
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Value:   "table",
			Usage:   "output format (table/json)",
		},
		&cli.BoolFlag{
			Name:  "fix",
			Usage: "after the report, apply the automatic fixes (asks for confirmation)",
		},
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "with --fix, apply the fixes without asking",
		},
	},
	Action: commandDoctor,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

type doctorStatus string

const (
	doctorOK   doctorStatus = "ok"
	doctorInfo doctorStatus = "info"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
	doctorSkip doctorStatus = "skip"
)

// doctorResult is the outcome of one check. Message says what was found in plain language and
// Fix says exactly what to run or change.
type doctorResult struct {
	ID      string       `json:"id"`
	Section string       `json:"section"`
	Status  doctorStatus `json:"status"`
	Message string       `json:"message"`
	Fix     string       `json:"fix,omitempty"`
	AutoFix *doctorFix   `json:"autoFix,omitempty"`
}

// doctorFix is a safe, idempotent fix that `doctor --fix` can apply. Fixes are deduplicated by Key.
type doctorFix struct {
	Key   string                     `json:"key"`
	Label string                     `json:"label"`
	Run   func(c *cli.Context) error `json:"-"`
}

type doctorSummary struct {
	OK   int `json:"ok"`
	Info int `json:"info"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

type doctorReport struct {
	Version string         `json:"version"`
	OS      string         `json:"os"`
	Arch    string         `json:"arch"`
	Summary doctorSummary  `json:"summary"`
	Checks  []doctorResult `json:"checks"`
}

func commandDoctor(c *cli.Context) error {
	format := c.String("format")
	if format != "table" && format != "json" {
		return fmt.Errorf("unsupported format: %s (use table or json)", format)
	}
	fix := c.Bool("fix")
	if fix && format == "json" {
		return fmt.Errorf("--fix can't be combined with --format json")
	}

	baseFolder := os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER)
	if info, err := os.Stat(baseFolder); err == nil && info.IsDir() {
		SetupLogger(baseFolder)
	} else if !SKIP_LOGGER_SETTINGS {
		// Don't create ~/.shelltime just to log, and keep library logs out of the report.
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}

	opts := doctorOptions{offline: c.Bool("offline")}

	if format == "json" {
		results := runDoctorChecks(c.Context, opts)
		if err := writeDoctorJSON(results); err != nil {
			return err
		}
		return doctorOutcome(results)
	}

	color.Cyan.Println("🩺 Running shelltime doctor...")
	results := runDoctorChecks(c.Context, opts)
	printDoctorReport(results)

	fixes := collectDoctorFixes(results)
	summary := summarizeDoctor(results)
	switch {
	case fix && len(fixes) == 0 && summary.Fail+summary.Warn > 0:
		color.Yellow.Println("\nNone of these problems can be fixed automatically; follow the steps above.")
	case fix && len(fixes) > 0:
		if applyDoctorFixes(c, fixes, c.Bool("yes")) {
			color.Cyan.Println("\n🩺 Re-running checks...")
			results = runDoctorChecks(c.Context, opts)
			printDoctorSummary(results, false)
		}
	case len(fixes) > 0:
		color.Cyan.Printf("\nRun 'shelltime doctor --fix' to apply %d of these fixes automatically.\n", len(fixes))
	}

	return doctorOutcome(results)
}

func doctorOutcome(results []doctorResult) error {
	if summarizeDoctor(results).Fail > 0 {
		return ErrDoctorFoundProblems
	}
	return nil
}

func summarizeDoctor(results []doctorResult) doctorSummary {
	var s doctorSummary
	for _, r := range results {
		switch r.Status {
		case doctorOK:
			s.OK++
		case doctorInfo:
			s.Info++
		case doctorWarn:
			s.Warn++
		case doctorFail:
			s.Fail++
		case doctorSkip:
			s.Skip++
		}
	}
	return s
}

func doctorVersion() string {
	if commitID == "" {
		return "dev"
	}
	return commitID
}

func writeDoctorJSON(results []doctorResult) error {
	report := doctorReport{
		Version: doctorVersion(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Summary: summarizeDoctor(results),
		Checks:  results,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal doctor report: %w", err)
	}
	_, err = fmt.Fprintln(doctorOut, string(data))
	return err
}

func printDoctorReport(results []doctorResult) {
	section := ""
	for _, r := range results {
		if r.Section != section {
			printSectionHeader(r.Section)
			section = r.Section
		}
		printDoctorResult(r)
	}
	printDoctorSummary(results, true)
}

func printDoctorResult(r doctorResult) {
	switch r.Status {
	case doctorOK:
		printSuccess(r.Message)
	case doctorInfo:
		printInfo(r.Message)
	case doctorWarn:
		printWarning(r.Message)
	case doctorFail:
		printError(r.Message)
	case doctorSkip:
		printSkip(r.Message)
	}
	if r.Fix == "" {
		return
	}
	label := "fix"
	if r.Status != doctorWarn && r.Status != doctorFail {
		label = "tip"
	}
	color.Gray.Printf("      → %s: %s\n", label, r.Fix)
}

// doctorAction is one step of the "How to fix" list together with the problems it resolves.
type doctorAction struct {
	fix      string
	auto     bool
	problems []doctorResult
}

// groupDoctorActions groups warnings and failures by the action that fixes them (the auto-fix key,
// else the fix text), so one command that solves several problems is listed once. Actions that fix
// a failure come first.
func groupDoctorActions(results []doctorResult) []*doctorAction {
	var actions []*doctorAction
	byKey := map[string]*doctorAction{}
	for _, status := range []doctorStatus{doctorFail, doctorWarn} {
		for _, r := range results {
			if r.Status != status {
				continue
			}
			key := "fix:" + r.Fix
			if r.AutoFix != nil {
				key = "auto:" + r.AutoFix.Key
			}
			if r.Fix == "" {
				key = "id:" + r.ID
			}
			action, ok := byKey[key]
			if !ok {
				action = &doctorAction{fix: r.Fix, auto: r.AutoFix != nil}
				byKey[key] = action
				actions = append(actions, action)
			}
			action.problems = append(action.problems, r)
		}
	}
	return actions
}

// printDoctorSummary prints the counts and the "How to fix" list. markAuto tags the steps
// `doctor --fix` can handle.
func printDoctorSummary(results []doctorResult, markAuto bool) {
	s := summarizeDoctor(results)
	fmt.Println()
	color.Style{color.FgCyan, color.OpBold}.Println("Summary")
	fmt.Printf("  %d passed · %d warnings · %d failed · %d skipped\n", s.OK, s.Warn, s.Fail, s.Skip)

	actions := groupDoctorActions(results)
	if len(actions) == 0 {
		color.Green.Println("\n✓ Everything looks good.")
		return
	}

	fmt.Println()
	color.Style{color.FgCyan, color.OpBold}.Println("How to fix")
	for i, action := range actions {
		heading := action.fix
		if heading == "" {
			heading = action.problems[0].Message
		}
		auto := ""
		if markAuto && action.auto {
			auto = color.Green.Render(" (auto-fixable)")
		}
		fmt.Printf("  %d. %s%s\n", i+1, heading, auto)
		if action.fix == "" {
			continue
		}
		for _, r := range action.problems {
			mark := color.Yellow.Render("⚠️")
			if r.Status == doctorFail {
				mark = color.Red.Render("✗")
			}
			color.Gray.Printf("     %s [%s] %s\n", mark, r.Section, r.Message)
		}
	}
}

// collectDoctorFixes returns the automatic fixes for every failure, then every warning, once per Key.
func collectDoctorFixes(results []doctorResult) []*doctorFix {
	var fixes []*doctorFix
	for _, action := range groupDoctorActions(results) {
		for _, r := range action.problems {
			if r.AutoFix != nil {
				fixes = append(fixes, r.AutoFix)
				break
			}
		}
	}
	return fixes
}

// applyDoctorFixes asks for confirmation (unless assumeYes) and runs the fixes. It reports whether
// any fix was attempted.
func applyDoctorFixes(c *cli.Context, fixes []*doctorFix, assumeYes bool) bool {
	fmt.Println()
	color.Style{color.FgCyan, color.OpBold}.Println("Automatic fixes")
	for i, f := range fixes {
		fmt.Printf("  %d. %s\n", i+1, f.Label)
	}

	if !assumeYes {
		fmt.Printf("\nApply %d fix(es)? [y/N]: ", len(fixes))
		answer, _ := bufio.NewReader(doctorStdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			color.Yellow.Println("No changes made.")
			return false
		}
	}

	for _, f := range fixes {
		color.Cyan.Printf("\n→ %s\n", f.Label)
		if err := f.Run(c); err != nil {
			printError(fmt.Sprintf("Fix failed: %v", err))
			continue
		}
		printSuccess("Done.")
	}
	return true
}

func printSectionHeader(title string) {
	color.Style{color.FgCyan, color.OpBold}.Println("\n" + title)
	fmt.Println(strings.Repeat("-", len(title)))
}

func printSuccess(message string) {
	color.Green.Printf("  ✓ %s\n", message)
}

func printError(message string) {
	color.Red.Printf("  ✗ %s\n", message)
}

func printWarning(message string) {
	color.Yellow.Printf("  ⚠️ %s\n", message)
}

func printInfo(message string) {
	color.Gray.Printf("  ℹ️ %s\n", message)
}

func printSkip(message string) {
	color.Gray.Printf("  - %s\n", message)
}
