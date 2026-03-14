package commands

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

var AICodeHooksCommand = &cli.Command{
	Name:  "aicode-hooks",
	Usage: "Track AI coding tool hook events",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "source",
			Value: "claude-code",
			Usage: "Source of the hook event (claude-code, codex, cursor)",
		},
	},
	Action: commandAICodeHooks,
	Subcommands: []*cli.Command{
		AICodeHooksInstallCommand,
		AICodeHooksUninstallCommand,
	},
}

func commandAICodeHooks(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "aicode-hooks", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	// Read JSON from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		slog.Error("Failed to read stdin", slog.Any("err", err))
		return err
	}

	if len(input) == 0 {
		slog.Debug("No input received from stdin")
		return nil
	}

	// Parse raw JSON payload
	var rawPayload map[string]any
	if err := json.Unmarshal(input, &rawPayload); err != nil {
		slog.Error("Failed to parse JSON input", slog.Any("err", err))
		return err
	}

	// Detect source
	source := c.String("source")
	if !c.IsSet("source") {
		// Auto-detect: if JSON has hook_event_name, it's claude-code (default)
		if _, ok := rawPayload["hook_event_name"]; ok {
			source = model.AICodeHooksSourceClaudeCode
		}
	}

	// Map source to client type
	clientType := mapSourceToClientType(source)

	// Build event data
	eventData := model.AICodeHooksEventData{
		EventID:    uuid.New().String(),
		ClientType: clientType,
		Timestamp:  time.Now().Unix(),
		RawPayload: rawPayload,
	}

	// Parse common fields from raw payload
	if v, ok := rawPayload["hook_event_name"].(string); ok {
		eventData.HookEventName = v
	}
	if v, ok := rawPayload["session_id"].(string); ok {
		eventData.SessionID = v
	}
	if v, ok := rawPayload["cwd"].(string); ok {
		eventData.Cwd = v
	}
	if v, ok := rawPayload["permission_mode"].(string); ok {
		eventData.PermissionMode = v
	}
	if v, ok := rawPayload["model"].(string); ok {
		eventData.Model = v
	}
	if v, ok := rawPayload["tool_name"].(string); ok {
		eventData.ToolName = v
	}
	if v, ok := rawPayload["tool_input"].(map[string]any); ok {
		eventData.ToolInput = v
	}
	if v, ok := rawPayload["tool_response"].(map[string]any); ok {
		eventData.ToolResponse = v
	}
	if v, ok := rawPayload["tool_use_id"].(string); ok {
		eventData.ToolUseID = v
	}
	if v, ok := rawPayload["prompt"].(string); ok {
		eventData.Prompt = v
	}
	if v, ok := rawPayload["error"].(string); ok {
		eventData.Error = v
	}
	if v, ok := rawPayload["is_interrupt"].(bool); ok {
		eventData.IsInterrupt = v
	}
	if v, ok := rawPayload["agent_id"].(string); ok {
		eventData.AgentID = v
	}
	if v, ok := rawPayload["agent_type"].(string); ok {
		eventData.AgentType = v
	}
	if v, ok := rawPayload["last_assistant_message"].(string); ok {
		eventData.LastMessage = v
	}
	if v, ok := rawPayload["stop_hook_active"].(bool); ok {
		eventData.StopHookActive = v
	}
	if v, ok := rawPayload["notification_type"].(string); ok {
		eventData.NotificationType = v
	}
	if v, ok := rawPayload["notification_message"].(string); ok {
		eventData.NotificationMessage = v
	}
	if v, ok := rawPayload["session_end_reason"].(string); ok {
		eventData.SessionEndReason = v
	}
	if v, ok := rawPayload["transcript_path"].(string); ok {
		eventData.TranscriptPath = v
	}

	// Try sending to daemon socket first
	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to read config", slog.Any("err", err))
		return err
	}

	socketPath := config.SocketPath
	if daemon.IsSocketReady(ctx, socketPath) {
		err := sendAICodeHooksToSocket(ctx, socketPath, eventData)
		if err != nil {
			slog.Error("Failed to send to daemon socket, trying direct HTTP", slog.Any("err", err))
			sendAICodeHooksDirect(ctx, config, eventData)
		}
	} else {
		slog.Debug("Daemon socket not available, sending direct HTTP")
		sendAICodeHooksDirect(ctx, config, eventData)
	}

	return nil
}

func mapSourceToClientType(source string) string {
	switch source {
	case model.AICodeHooksSourceCodex:
		return model.AICodeHooksClientCodex
	case model.AICodeHooksSourceCursor:
		return model.AICodeHooksClientCursor
	default:
		return model.AICodeHooksClientClaudeCode
	}
}

func sendAICodeHooksToSocket(ctx context.Context, socketPath string, eventData model.AICodeHooksEventData) error {
	return daemon.SendAICodeHooksToSocket(socketPath, eventData)
}

// sendAICodeHooksDirect sends event data directly via HTTP (fire-and-forget)
func sendAICodeHooksDirect(ctx context.Context, config model.ShellTimeConfig, eventData model.AICodeHooksEventData) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	req := &model.AICodeHooksRequest{
		Host:   hostname,
		Events: []model.AICodeHooksEventData{eventData},
	}

	endpoint := model.Endpoint{
		Token:       config.Token,
		APIEndpoint: config.APIEndpoint,
	}

	// Fire-and-forget
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := model.SendAICodeHooksData(sendCtx, req, endpoint)
		if err != nil {
			slog.Error("AICodeHooks: Direct HTTP send failed", slog.Any("err", err))
		}
	}()
}
