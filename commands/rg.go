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
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

// CommandEdge represents a single command from the server
type CommandEdge struct {
	ID              int     `json:"id"`
	Shell           string  `json:"shell"`
	SessionID       float64 `json:"sessionId"`
	Command         string  `json:"command"`
	MainCommand     string  `json:"mainCommand"`
	Hostname        string  `json:"hostname"`
	Username        string  `json:"username"`
	Time            float64 `json:"time"`
	EndTime         float64 `json:"endTime"`
	Result          int     `json:"result"`
	IsEncrypted     bool    `json:"isEncrypted"`
	OriginalCommand string  `json:"originalCommand"`
}

// FetchCommandsData wraps the GraphQL data response
type FetchCommandsData struct {
	FetchCommands struct {
		Count int           `json:"count"`
		Edges []CommandEdge `json:"edges"`
	} `json:"fetchCommands"`
}

// FetchCommandsResponse is the complete GraphQL response
type FetchCommandsResponse = model.GraphQLResponse[FetchCommandsData]

var RgCommand *cli.Command = &cli.Command{
	Name:      "rg",
	Aliases:   []string{"grep"},
	Usage:     "Search server-synced commands",
	ArgsUsage: "<search-text>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Value:   "table",
			Usage:   "output format (table/json)",
		},
		&cli.IntFlag{
			Name:    "limit",
			Aliases: []string{"l"},
			Value:   50,
			Usage:   "maximum number of results",
		},
		&cli.IntFlag{
			Name:    "offset",
			Aliases: []string{"o"},
			Value:   0,
			Usage:   "skip this many results (pagination)",
		},
		&cli.StringFlag{
			Name:    "shell",
			Aliases: []string{"s"},
			Usage:   "filter by shell (bash, zsh, fish)",
		},
		&cli.StringFlag{
			Name:    "hostname",
			Aliases: []string{"H"},
			Usage:   "filter by hostname",
		},
		&cli.StringFlag{
			Name:    "username",
			Aliases: []string{"u"},
			Usage:   "filter by username",
		},
		&cli.IntFlag{
			Name:    "result",
			Aliases: []string{"r"},
			Value:   -1,
			Usage:   "filter by exit code (-1 means any)",
		},
		&cli.StringFlag{
			Name:    "main-command",
			Aliases: []string{"m"},
			Usage:   "filter by main command (e.g., git, npm)",
		},
		&cli.Int64Flag{
			Name:  "session",
			Value: -1,
			Usage: "filter by session ID (-1 means any)",
		},
		&cli.StringFlag{
			Name:  "since",
			Usage: "filter commands since time (RFC3339 format)",
		},
		&cli.StringFlag{
			Name:  "until",
			Usage: "filter commands until time (RFC3339 format)",
		},
	},
	Action: commandRg,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

func commandRg(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "rg", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	// Validate format
	format := c.String("format")
	if format != "table" && format != "json" {
		return fmt.Errorf("unsupported format: %s. Use 'table' or 'json'", format)
	}

	// Get search text from args
	searchText := c.Args().First()
	if searchText == "" {
		return fmt.Errorf("search text is required. Usage: shelltime rg <search-text>")
	}

	// Read config to get endpoint and token
	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if cfg.Token == "" {
		return fmt.Errorf("not authenticated. Please run 'shelltime auth' first")
	}

	endpoint := model.Endpoint{
		APIEndpoint: cfg.APIEndpoint,
		Token:       cfg.Token,
	}

	// Build filter
	filter := buildCommandFilter(c, searchText)

	// Build pagination
	pagination := map[string]interface{}{
		"limit":  c.Int("limit"),
		"offset": c.Int("offset"),
	}

	// Build variables
	variables := map[string]interface{}{
		"pagination": pagination,
		"filter":     filter,
	}

	// GraphQL query
	query := `query fetchCommands($pagination: InputPagination!, $filter: CommandFilter!) {
		fetchCommands(pagination: $pagination, filter: $filter) {
			count
			edges {
				id
				shell
				sessionId
				command
				mainCommand
				hostname
				username
				time
				endTime
				result
				isEncrypted
				originalCommand
			}
		}
	}`

	var result FetchCommandsResponse
	err = model.SendGraphQLRequest(model.GraphQLRequestOptions[FetchCommandsResponse]{
		Context:   ctx,
		Endpoint:  endpoint,
		Query:     query,
		Variables: variables,
		Response:  &result,
		Timeout:   time.Second * 30,
	})
	if err != nil {
		return fmt.Errorf("failed to fetch commands: %w", err)
	}

	commands := result.Data.FetchCommands.Edges
	totalCount := result.Data.FetchCommands.Count

	if len(commands) == 0 {
		color.Yellow.Println("No commands found matching your search")
		return nil
	}

	// Output based on format
	if format == "json" {
		return outputRgJSON(commands, totalCount)
	}
	return outputRgTable(commands, totalCount, c.Int("limit"), c.Int("offset"))
}

func buildCommandFilter(c *cli.Context, searchText string) map[string]interface{} {
	filter := map[string]interface{}{
		"shell":       []string{},
		"sessionId":   []float64{},
		"mainCommand": []string{},
		"hostname":    []string{},
		"username":    []string{},
		"ip":          []string{},
		"result":      []int{},
		"time":        []float64{},
		"command":     searchText,
	}

	// Add optional filters if provided
	if shell := c.String("shell"); shell != "" {
		filter["shell"] = []string{shell}
	}

	if hostname := c.String("hostname"); hostname != "" {
		filter["hostname"] = []string{hostname}
	}

	if username := c.String("username"); username != "" {
		filter["username"] = []string{username}
	}

	if result := c.Int("result"); result >= 0 {
		filter["result"] = []int{result}
	}

	if mainCmd := c.String("main-command"); mainCmd != "" {
		filter["mainCommand"] = []string{mainCmd}
	}

	if session := c.Int64("session"); session >= 0 {
		filter["sessionId"] = []float64{float64(session)}
	}

	// Handle time filters
	var timeFilters []float64
	if since := c.String("since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err == nil {
			timeFilters = append(timeFilters, float64(t.UnixMilli()))
		}
	}
	if until := c.String("until"); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err == nil {
			timeFilters = append(timeFilters, float64(t.UnixMilli()))
		}
	}
	if len(timeFilters) > 0 {
		filter["time"] = timeFilters
	}

	return filter
}

func outputRgJSON(commands []CommandEdge, totalCount int) error {
	output := struct {
		TotalCount int           `json:"totalCount"`
		Commands   []CommandEdge `json:"commands"`
	}{
		TotalCount: totalCount,
		Commands:   commands,
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

func outputRgTable(commands []CommandEdge, totalCount, limit, offset int) error {
	w := tablewriter.NewWriter(os.Stdout)
	w.Header([]string{"COMMAND", "SHELL", "TIME", "END TIME", "DURATION(ms)", "STATUS", "USER", "HOST"})

	for _, cmd := range commands {
		// Use originalCommand if encrypted and available
		displayCommand := cmd.Command
		if cmd.IsEncrypted && cmd.OriginalCommand != "" {
			displayCommand = cmd.OriginalCommand
		}

		// Convert milliseconds to time
		startTime := time.UnixMilli(int64(cmd.Time))
		endTime := time.UnixMilli(int64(cmd.EndTime))
		duration := int64(cmd.EndTime - cmd.Time)

		w.Append([]string{
			displayCommand,
			cmd.Shell,
			startTime.Format(time.RFC3339),
			endTime.Format(time.RFC3339),
			strconv.FormatInt(duration, 10),
			strconv.Itoa(cmd.Result),
			cmd.Username,
			cmd.Hostname,
		})
	}

	w.Render()

	// Show result count summary
	showing := len(commands)
	if totalCount > showing {
		color.Gray.Printf("\nShowing %d of %d total results (offset: %d)\n", showing, totalCount, offset)
		if offset+limit < totalCount {
			color.Gray.Printf("Use --offset %d to see more results\n", offset+limit)
		}
	}

	return nil
}
