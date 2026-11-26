package commands

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AICodeProjectsCommand is `shelltime cc projects` and `shelltime codex projects`: the projects
// of both agents are grouped together, so either runs the same migration.
var AICodeProjectsCommand = &cli.Command{
	Name:  "projects",
	Usage: "Group this machine's coding agent projects by git repository instead of folder",
	Description: "Sessions recorded before ShellTime named their git repository are grouped into projects by\n" +
		"machine and folder, so a renamed or moved clone shows up as another project. This finds the\n" +
		"repository of each such folder that still exists on this machine and moves its sessions into\n" +
		"that repository's project. The daemon does this once on its own; run it again any time.",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show which folders would move without changing anything",
		},
	},
	Action: commandAICodeProjects,
}

func commandAICodeProjects(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "aicode.projects", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("no ShellTime token configured, run `shelltime init` first")
	}
	if skippedInAICloudOnly(cfg, "Projects not grouped") {
		return nil
	}
	endpoint := model.Endpoint{Token: cfg.Token, APIEndpoint: cfg.APIEndpoint}
	hosts := model.ProjectMigrationHosts(model.NewClaudeSettingsAICodeOtelEnvService().ResourceMachineName())
	dryRun := c.Bool("dry-run")

	color.Yellow.Println("Looking for projects grouped by folder...")
	result, err := model.MigrateProjectRepositories(ctx, endpoint, hosts, model.NewGitRepositoryResolver(time.Hour), dryRun)
	span.SetAttributes(attribute.Int("checked", result.Checked), attribute.Int("linked", len(result.Linked)))
	if err != nil {
		var statusErr *model.HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return errors.New("the ShellTime server can't group projects by repository yet, please try again after it is updated")
		}
		return fmt.Errorf("project migration failed: %w", err)
	}

	if len(hosts) == 0 {
		return errors.New("this machine's host name is one many machines share (like localhost), so its projects can't be told apart from theirs")
	}
	if result.Checked == 0 {
		color.Green.Println("Every project of this machine is already grouped by repository.")
		return nil
	}
	if len(result.Linked) > 0 {
		w := tablewriter.NewWriter(os.Stdout)
		w.Header([]string{"FOLDER", "REPOSITORY"})
		for _, p := range result.Linked {
			w.Append([]string{p.Pwd, p.Repository})
		}
		w.Render()
	}

	fmt.Printf("%d folder projects found: %d in a git repository, %d no longer here, %d not in a repository with a remote.\n",
		result.Checked, len(result.Linked), result.Missing, result.NotRepositories)
	if result.Unchecked > 0 {
		color.Yellow.Printf("git didn't answer for %d folders; run it again to check them.\n", result.Unchecked)
	}
	if result.Truncated {
		color.Yellow.Println("This machine has more folder projects than one run goes through; run it again to check the rest.")
	}
	switch {
	case dryRun:
		color.Green.Println("Dry run: nothing was changed.")
	case len(result.Linked) > 0:
		color.Green.Printf("Moved %s sessions into their repositories' projects.\n", strconv.Itoa(result.Sessions))
	}
	return nil
}
