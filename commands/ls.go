// cli/commands/ls.go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/olekukonko/tablewriter"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

var LsCommand *cli.Command = &cli.Command{
	Name:  "ls",
	Usage: "list locally saved commands",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Value:   "table",
			Usage:   "output format (table/json)",
		},
	},
	Action: commandList,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

func commandList(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "ls", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	format := c.String("format")
	if format != "table" && format != "json" {
		return fmt.Errorf("unsupported format: %s. Use 'table' or 'json'", format)
	}

	// TODO: add un-sync data list here
	if format == "table" {
		color.Yellow.Println("⚠️ Note: Unsaved commands are not included in this list")
	}

	if format == "table" {
		color.Yellow.Println("⚠️ Note: Local data will be cleaned periodically for performance and disk efficiency. To view all of your commands, please run 'shelltime web'")
	}

	// Get post commands
	postFileContent, _, err := model.GetPostCommands(ctx)
	if err != nil {
		return err
	}

	// Get pre commands tree for reference
	preFileTree, err := model.GetPreCommandsTree(ctx)
	if err != nil {
		return err
	}

	// Process commands
	var commands []struct {
		Command   string    `json:"command"`
		Shell     string    `json:"shell"`
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		Result    int       `json:"result"`
		Username  string    `json:"username"`
		Hostname  string    `json:"hostname"`
	}

	for _, line := range postFileContent {
		postCommand := new(model.Command)
		_, err := postCommand.FromLineBytes(line)
		if err != nil {
			logrus.Errorln("Failed to parse post command: ", err, string(line))
			continue
		}

		key := postCommand.GetUniqueKey()
		preCommands, ok := preFileTree[key]
		if !ok {
			continue
		}

		closestPreCommand := postCommand.FindClosestCommand(preCommands, false)
		startTime := postCommand.Time
		if closestPreCommand != nil {
			startTime = closestPreCommand.Time
		}

		commands = append(commands, struct {
			Command   string    `json:"command"`
			Shell     string    `json:"shell"`
			StartTime time.Time `json:"start_time"`
			EndTime   time.Time `json:"end_time"`
			Result    int       `json:"result"`
			Username  string    `json:"username"`
			Hostname  string    `json:"hostname"`
		}{
			Command:   postCommand.Command,
			Shell:     postCommand.Shell,
			StartTime: startTime,
			EndTime:   postCommand.Time,
			Result:    postCommand.Result,
			Username:  postCommand.Username,
			Hostname:  postCommand.Hostname,
		})
	}

	// Output based on format
	if format == "json" {
		return outputJSON(commands)
	}
	return outputTable(commands)
}

func outputJSON(commands interface{}) error {
	jsonData, err := json.MarshalIndent(commands, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

func outputTable(commands []struct {
	Command   string    `json:"command"`
	Shell     string    `json:"shell"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Result    int       `json:"result"`
	Username  string    `json:"username"`
	Hostname  string    `json:"hostname"`
}) error {
	w := tablewriter.NewWriter(os.Stdout)
	w.Header([]string{"COMMAND", "SHELL", "START TIME", "END TIME", "DURATION(ms)", "STATUS", "USER", "HOST"})

	for _, cmd := range commands {
		duration := cmd.EndTime.Sub(cmd.StartTime).Milliseconds()
		w.Append([]string{
			cmd.Command,
			cmd.Shell,
			cmd.StartTime.Format(time.RFC3339),
			cmd.EndTime.Format(time.RFC3339),
			strconv.Itoa(int(duration)),
			strconv.Itoa(cmd.Result),
			cmd.Username,
			cmd.Hostname,
		})
	}

	w.Render()
	return nil
}
