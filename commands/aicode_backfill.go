package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var CCBackfillCommand = &cli.Command{
	Name:   "backfill",
	Usage:  "Upload Claude Code usage from local transcripts that live tracking missed",
	Flags:  aiCodeBackfillFlags(),
	Action: commandCCBackfill,
}

var CodexBackfillCommand = &cli.Command{
	Name:   "backfill",
	Usage:  "Upload Codex usage from local session files that live tracking missed",
	Flags:  aiCodeBackfillFlags(),
	Action: commandCodexBackfill,
}

// aiCodeBackfillSource describes where a client keeps its transcripts and
// how to read them.
type aiCodeBackfillSource struct {
	clientType string
	name       string
	roots      func() []string
	parse      func(roots []string, opts model.BackfillOptions) (*model.BackfillParseResult, error)
}

func commandCCBackfill(c *cli.Context) error {
	return runAICodeBackfill(c, aiCodeBackfillSource{
		clientType: model.AICodeClientClaudeCode,
		name:       "Claude Code",
		roots:      model.ClaudeProjectRoots,
		parse:      model.ParseClaudeTranscripts,
	})
}

func commandCodexBackfill(c *cli.Context) error {
	return runAICodeBackfill(c, aiCodeBackfillSource{
		clientType: model.AICodeClientCodex,
		name:       "Codex",
		roots:      model.CodexSessionRoots,
		parse:      model.ParseCodexRollouts,
	})
}

func aiCodeBackfillFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "since",
			Usage: "only upload sessions started on or after this day (YYYY-MM-DD, local time)",
		},
		&cli.StringFlag{
			Name:  "until",
			Usage: "only upload sessions started on or before this day (YYYY-MM-DD, local time)",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be uploaded without uploading anything",
		},
		&cli.BoolFlag{
			Name:  "no-prompts",
			Usage: "upload prompt lengths but not the prompt text",
		},
		&cli.BoolFlag{
			Name:  "ai-summary",
			Usage: "also generate AI summaries for uploaded sessions (uses your monthly AI credits)",
		},
	}
}

// backfillBackoff is how long to wait before each retry of a failed upload.
var backfillBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}

// backfillPlan sorts local sessions by what the server already has.
type backfillPlan struct {
	upload   []*model.BackfillSession
	done     []*model.BackfillSession
	live     []*model.BackfillSession
	archived []*model.BackfillSession
}

func runAICodeBackfill(c *cli.Context, src aiCodeBackfillSource) error {
	ctx, span := commandTracer.Start(c.Context, "aicode.backfill", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(attribute.String("clientType", src.clientType))

	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	opts, err := backfillOptionsFromFlags(c)
	if err != nil {
		return err
	}

	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("failed to read config file", slog.Any("err", err))
		return err
	}
	if cfg.Token == "" {
		return errors.New("no ShellTime token configured, run `shelltime init` first")
	}
	endpoint := model.Endpoint{Token: cfg.Token, APIEndpoint: cfg.APIEndpoint}

	color.Yellow.Printf("Scanning %s transcripts...\n", src.name)
	parsed, err := src.parse(src.roots(), opts)
	if err != nil {
		return fmt.Errorf("failed to read %s transcripts: %w", src.name, err)
	}
	sessions, active := model.SelectBackfillSessions(parsed.Sessions, opts)
	span.SetAttributes(attribute.Int("files", parsed.Files), attribute.Int("sessions", len(sessions)))
	if parsed.BadLines > 0 {
		slog.Warn("skipped unreadable transcript lines", slog.Int("count", parsed.BadLines))
	}
	if len(sessions) == 0 {
		fmt.Printf("No finished %s sessions found in %d files.\n", src.name, parsed.Files)
		return nil
	}

	statuses, err := fetchBackfillStatuses(ctx, endpoint, src.clientType, sessions)
	if err != nil {
		return backfillServerError(err)
	}
	plan := classifyBackfillSessions(sessions, statuses)
	printBackfillPlan(plan, active)

	if c.Bool("dry-run") {
		printBackfillDays(src.clientType, plan.upload)
		color.Green.Println("Dry run: nothing was uploaded.")
		return nil
	}
	if len(plan.upload) == 0 {
		color.Green.Println("Nothing new to upload.")
		return nil
	}

	batches := model.PackBackfillBatches(src.clientType, plan.upload,
		model.AICodeBackfillMaxEvents, model.AICodeBackfillMaxBatchBytes, model.AICodeBackfillMaxCompleted)
	accepted := 0
	skippedByServer := map[string]bool{}
	for i := range batches {
		batch := batches[i]
		batch.AISummary = c.Bool("ai-summary")
		var resp *model.AICodeBackfillResponse
		err := sendWithRetry(ctx, func() error {
			var err error
			resp, err = model.SendAICodeBackfill(ctx, endpoint, batch)
			return err
		})
		if err != nil {
			fmt.Println()
			return backfillServerError(err)
		}
		accepted += resp.Accepted
		for _, s := range resp.SkippedSessions {
			skippedByServer[s.SessionID] = true
		}
		fmt.Printf("\rUploading... batch %d/%d", i+1, len(batches))
	}
	fmt.Println()

	from, to := backfillRange(plan.upload)
	err = sendWithRetry(ctx, func() error {
		return model.CompleteAICodeBackfill(ctx, endpoint, model.AICodeBackfillCompleteRequest{
			ClientType: src.clientType,
			From:       from,
			To:         to,
		})
	})
	if err != nil {
		// The data is stored; only the activity refresh and cache flush failed.
		color.Yellow.Printf("Uploaded, but refreshing dashboards failed: %v\n", err)
	}

	color.Green.Printf("Uploaded %d events from %d sessions.\n", accepted, len(plan.upload)-len(skippedByServer))
	if len(skippedByServer) > 0 {
		fmt.Printf("%d sessions were skipped because the server already tracks them.\n", len(skippedByServer))
	}
	if c.Bool("ai-summary") {
		fmt.Println("AI summaries are being generated in the background.")
	}
	fmt.Println("Note: free plans show the last 7 days of AI coding history.")
	return nil
}

func backfillOptionsFromFlags(c *cli.Context) (model.BackfillOptions, error) {
	opts := model.BackfillOptions{NoPrompts: c.Bool("no-prompts"), Now: time.Now()}
	if v := c.String("since"); v != "" {
		day, err := time.ParseInLocation(time.DateOnly, v, time.Local)
		if err != nil {
			return opts, fmt.Errorf("invalid --since %q, expected YYYY-MM-DD", v)
		}
		opts.Since = day
	}
	if v := c.String("until"); v != "" {
		day, err := time.ParseInLocation(time.DateOnly, v, time.Local)
		if err != nil {
			return opts, fmt.Errorf("invalid --until %q, expected YYYY-MM-DD", v)
		}
		// Inclusive of the whole day.
		opts.Until = day.AddDate(0, 0, 1)
	}
	if !opts.Since.IsZero() && !opts.Until.IsZero() && !opts.Since.Before(opts.Until) {
		return opts, errors.New("--since must not be after --until")
	}
	return opts, nil
}

func fetchBackfillStatuses(ctx context.Context, endpoint model.Endpoint, clientType string, sessions []*model.BackfillSession) (map[string]model.AICodeBackfillSessionStatus, error) {
	statuses := map[string]model.AICodeBackfillSessionStatus{}
	for start := 0; start < len(sessions); start += model.AICodeBackfillMaxSessionIDs {
		end := min(start+model.AICodeBackfillMaxSessionIDs, len(sessions))
		ids := make([]string, 0, end-start)
		for _, s := range sessions[start:end] {
			ids = append(ids, s.ID)
		}
		var chunk []model.AICodeBackfillSessionStatus
		err := sendWithRetry(ctx, func() error {
			var err error
			chunk, err = model.FetchAICodeBackfillStatus(ctx, endpoint, clientType, ids)
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, st := range chunk {
			statuses[st.SessionID] = st
		}
	}
	return statuses, nil
}

func classifyBackfillSessions(sessions []*model.BackfillSession, statuses map[string]model.AICodeBackfillSessionStatus) backfillPlan {
	var plan backfillPlan
	for _, s := range sessions {
		st, known := statuses[s.ID]
		switch {
		case !known:
			plan.upload = append(plan.upload, s)
		case st.Status == model.AICodeBackfillStatusLive:
			plan.live = append(plan.live, s)
		case st.Status == model.AICodeBackfillStatusArchived:
			plan.archived = append(plan.archived, s)
		case st.Status == model.AICodeBackfillStatusBackfilled && st.EventCount >= len(s.Events):
			plan.done = append(plan.done, s)
		default:
			// Partly uploaded before; re-sending is idempotent.
			plan.upload = append(plan.upload, s)
		}
	}
	return plan
}

func sendWithRetry(ctx context.Context, fn func() error) error {
	err := fn()
	for _, wait := range backfillBackoff {
		if err == nil || !isRetryableBackfillError(err) {
			return err
		}
		slog.Warn("backfill request failed, retrying", slog.Any("err", err), slog.Duration("wait", wait))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		err = fn()
	}
	return err
}

func isRetryableBackfillError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var statusErr *model.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode >= http.StatusInternalServerError || statusErr.StatusCode == http.StatusTooManyRequests
	}
	// Network errors and timeouts.
	return true
}

func backfillServerError(err error) error {
	var statusErr *model.HTTPStatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
		return errors.New("the ShellTime server does not support backfill yet, please try again after it is updated")
	}
	return fmt.Errorf("backfill failed: %w", err)
}

func backfillRange(sessions []*model.BackfillSession) (time.Time, time.Time) {
	var from, to time.Time
	for _, s := range sessions {
		if from.IsZero() || s.Start.Before(from) {
			from = s.Start
		}
		if s.End.After(to) {
			to = s.End
		}
	}
	return from, to
}

func countBackfillEvents(sessions []*model.BackfillSession) int {
	n := 0
	for _, s := range sessions {
		n += len(s.Events)
	}
	return n
}

func printBackfillPlan(plan backfillPlan, active int) {
	w := tablewriter.NewWriter(os.Stdout)
	w.Header([]string{"SESSIONS", "COUNT", "EVENTS"})
	rows := []struct {
		label    string
		sessions []*model.BackfillSession
	}{
		{"to upload", plan.upload},
		{"already uploaded", plan.done},
		{"tracked live (skipped)", plan.live},
		{"archived on server (skipped)", plan.archived},
	}
	for _, r := range rows {
		w.Append([]string{r.label, strconv.Itoa(len(r.sessions)), strconv.Itoa(countBackfillEvents(r.sessions))})
	}
	if active > 0 {
		w.Append([]string{"still active (skipped)", strconv.Itoa(active), "-"})
	}
	w.Render()
}

type backfillDay struct {
	sessions, prompts, requests, tokens int
}

// printBackfillDays shows per-day totals of what would be uploaded. Tokens
// add up input, output and cache tokens; Codex input already includes cached
// tokens, so cache reads are only added for Claude Code.
func printBackfillDays(clientType string, sessions []*model.BackfillSession) {
	if len(sessions) == 0 {
		return
	}
	days := map[string]*backfillDay{}
	for _, s := range sessions {
		key := s.Start.Local().Format(time.DateOnly)
		d := days[key]
		if d == nil {
			d = &backfillDay{}
			days[key] = d
		}
		d.sessions++
		for _, e := range s.Events {
			switch e.EventType {
			case model.AICodeEventUserPrompt:
				d.prompts++
			case model.AICodeEventApiRequest, model.AICodeEventSSEEvent:
				d.requests++
				d.tokens += derefInt(e.InputTokens) + derefInt(e.OutputTokens) + derefInt(e.CacheCreationTokens)
				if clientType == model.AICodeClientClaudeCode {
					d.tokens += derefInt(e.CacheReadTokens)
				}
			}
		}
	}

	keys := make([]string, 0, len(days))
	for k := range days {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	w := tablewriter.NewWriter(os.Stdout)
	w.Header([]string{"DAY", "SESSIONS", "PROMPTS", "REQUESTS", "TOKENS"})
	for _, k := range keys {
		d := days[k]
		w.Append([]string{k, strconv.Itoa(d.sessions), strconv.Itoa(d.prompts), strconv.Itoa(d.requests), strconv.Itoa(d.tokens)})
	}
	w.Render()
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
