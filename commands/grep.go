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

var GrepCommand *cli.Command = &cli.Command{
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
		&cli.StringFlag{
			Name:  "since",
			Usage: "filter commands since date (2024, 2024-01, or 2024-01-15)",
		},
		&cli.StringFlag{
			Name:  "until",
			Usage: "filter commands until date (2024, 2024-01, or 2024-01-15)",
		},
	},
	Action: commandGrep,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

func commandGrep(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "grep", trace.WithSpanKind(trace.SpanKindClient))
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
		return fmt.Errorf("search text is required. Usage: shelltime grep <search-text>")
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
	filter, err := buildGrepFilter(c, searchText)
	if err != nil {
		return err
	}

	// Build pagination
	pagination := &model.SearchCommandsPagination{
		Limit:  c.Int("limit"),
		Offset: c.Int("offset"),
	}

	// Fetch commands from server
	result, err := model.FetchCommandsFromServer(ctx, endpoint, filter, pagination)
	if err != nil {
		return fmt.Errorf("failed to fetch commands: %w", err)
	}

	if len(result.Edges) == 0 {
		color.Yellow.Println("No commands found matching your search")
		return nil
	}

	// Output based on format
	if format == "json" {
		return outputGrepJSON(result.Edges, result.Count)
	}
	return outputGrepTable(result.Edges, result.Count, c.Int("limit"), c.Int("offset"))
}

func buildGrepFilter(c *cli.Context, searchText string) (*model.SearchCommandsFilter, error) {
	filter := &model.SearchCommandsFilter{
		Shell:       []string{},
		MainCommand: []string{},
		Hostname:    []string{},
		Username:    []string{},
		IP:          []string{},
		Result:      []int{},
		Time:        []float64{},
		SessionID:   []float64{},
		Command:     searchText,
	}

	// Add optional filters if provided
	if shell := c.String("shell"); shell != "" {
		filter.Shell = []string{shell}
	}

	if hostname := c.String("hostname"); hostname != "" {
		filter.Hostname = []string{hostname}
	}

	if username := c.String("username"); username != "" {
		filter.Username = []string{username}
	}

	if result := c.Int("result"); result >= 0 {
		filter.Result = []int{result}
	}

	if mainCmd := c.String("main-command"); mainCmd != "" {
		filter.MainCommand = []string{mainCmd}
	}

	// Handle time filters with flexible date parsing
	var timeFilters []float64
	if since := c.String("since"); since != "" {
		t, err := parseFlexibleDate(since, false)
		if err != nil {
			return nil, fmt.Errorf("invalid --since date: %w", err)
		}
		timeFilters = append(timeFilters, float64(t.UnixMilli()))
	}
	if until := c.String("until"); until != "" {
		t, err := parseFlexibleDate(until, true)
		if err != nil {
			return nil, fmt.Errorf("invalid --until date: %w", err)
		}
		timeFilters = append(timeFilters, float64(t.UnixMilli()))
	}
	if len(timeFilters) > 0 {
		filter.Time = timeFilters
	}

	return filter, nil
}

// parseFlexibleDate parses dates in formats: 2024, 2024-01, 2024-01-15
// If isEndOfPeriod is true, returns end of the period (for --until)
func parseFlexibleDate(s string, isEndOfPeriod bool) (time.Time, error) {
	// Try year only: 2024
	if t, err := time.Parse("2006", s); err == nil {
		if isEndOfPeriod {
			return time.Date(t.Year(), 12, 31, 23, 59, 59, 0, time.UTC), nil
		}
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC), nil
	}

	// Try year-month: 2024-01
	if t, err := time.Parse("2006-01", s); err == nil {
		if isEndOfPeriod {
			// End of month: go to next month, then subtract 1 second
			return t.AddDate(0, 1, 0).Add(-time.Second), nil
		}
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}

	// Try year-month-day: 2024-01-15
	if t, err := time.Parse("2006-01-02", s); err == nil {
		if isEndOfPeriod {
			return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC), nil
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	}

	return time.Time{}, fmt.Errorf("use format: 2024, 2024-01, or 2024-01-15")
}

func outputGrepJSON(commands []model.SearchCommandEdge, totalCount int) error {
	output := struct {
		TotalCount int                       `json:"totalCount"`
		Commands   []model.SearchCommandEdge `json:"commands"`
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

func outputGrepTable(commands []model.SearchCommandEdge, totalCount, limit, offset int) error {
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
