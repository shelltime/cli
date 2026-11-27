package commands

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/malamtime/cli/agenthook"
	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

const (
	// hookIngestStdinTimeout bounds reading the hook payload: the agent waits on the hook.
	hookIngestStdinTimeout = 500 * time.Millisecond
	// hookIngestStdinLimit caps the hook payload; a larger one gives no event.
	hookIngestStdinLimit = 4 << 20
	// hookIngestDaemonTimeout bounds handing the event to the daemon, acknowledgment included.
	hookIngestDaemonTimeout = 50 * time.Millisecond
)

// Seams for tests.
var (
	hookIngestStdin  io.Reader = os.Stdin
	hookIngestStdout io.Writer = os.Stdout
	hookIngestGetenv           = os.Getenv
	hookIngestNow              = time.Now
	// hookIngestRawArgs are the command line arguments, read when urfave/cli couldn't parse
	// them.
	hookIngestRawArgs = func() []string { return os.Args }
)

// HookIngestCommand is what the Cursor and Antigravity hooks run (`shelltime cursor install`,
// `shelltime agy install`). It must never get in the agent's way: whatever happens, stdout
// holds exactly the reply that lets the agent carry on, and the command exits 0.
var HookIngestCommand = &cli.Command{
	Name:     "hook-ingest",
	Usage:    "record a Cursor or Antigravity hook event (run by the agent's hooks)",
	Hidden:   true,
	HideHelp: true,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "agent", Usage: "the agent that runs the hook: cursor, or agy (antigravity)"},
		&cli.StringFlag{Name: "event", Usage: "the hook event"},
	},
	Action: func(c *cli.Context) error {
		runHookIngest(c.Context, c.String("agent"), c.String("event"))
		return nil
	},
	OnUsageError: func(c *cli.Context, err error, isSubcommand bool) error {
		// The hook still answers. Read --agent and --event from the arguments as they are.
		agent, event := hookIngestFlagsFromArgs(hookIngestRawArgs())
		runHookIngest(c.Context, agent, event)
		return nil
	},
}

// runHookIngest answers the hook on stdout, then records its event: through the daemon when
// one takes it, else in the pending file the daemon resends from. Nothing else ever reaches
// stdout, and a panic still answers.
func runHookIngest(ctx context.Context, agent, event string) {
	if ctx == nil {
		ctx = context.Background()
	}
	answered := false
	answer := func(event string) {
		if answered {
			return
		}
		answered = true
		_, _ = hookIngestStdout.Write(append(agenthook.PassThrough(agent, event), '\n'))
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("hook-ingest failed", slog.Any("panic", r))
		}
		answer(event)
	}()

	// The log goes to a file (or stderr), never stdout
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	readCtx, cancel := context.WithTimeout(ctx, hookIngestStdinTimeout)
	stdin, readErr := readWithTimeout(readCtx, hookIngestStdin, hookIngestStdinLimit)
	cancel()
	answer(agenthook.ResolveEventName(event, stdin))

	if readErr != nil {
		slog.Warn("hook-ingest: couldn't read the hook payload", slog.String("agent", agent), slog.String("event", event), slog.Any("err", readErr))
		return
	}
	ev, err := agenthook.Normalize(agent, event, stdin, hookIngestNow(), hookIngestGetenv)
	if err != nil {
		slog.Warn("hook-ingest: no event recorded", slog.String("agent", agent), slog.String("event", event), slog.Any("err", err))
		return
	}
	ev.Hostname, _ = os.Hostname()

	// Fast path, before any config is read: the daemon applies the privacy settings, adds the
	// repository and sends in batches.
	if sendHookEventToDaemon(trackSocketPath, ev) {
		return
	}

	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("hook-ingest: failed to read config file, event dropped", slog.Any("err", err))
		return
	}
	// A daemon may listen on a socket of its own. The default one was tried already (tests
	// point trackSocketPath elsewhere and must never reach a real daemon).
	if config.SocketPath != trackSocketPath && config.SocketPath != model.DefaultSocketPath {
		if sendHookEventToDaemon(config.SocketPath, ev) {
			return
		}
	}

	// No daemon: keep the event, with the privacy settings applied, for the daemon to send
	model.ApplyAgentHookPrivacy(&ev, config)
	if err := model.AppendAgentHookPending(ev); err != nil {
		slog.Error("hook-ingest: failed to save the event", slog.Any("err", err))
		return
	}
	slog.Debug("hook-ingest: no daemon, event saved", slog.String("agent", ev.Agent), slog.String("eventType", ev.EventType))
}

// sendHookEventToDaemon reports whether a daemon on socketPath acknowledged the event. A
// socket file left by a dead daemon refuses the connection, which counts as not taken.
func sendHookEventToDaemon(socketPath string, ev agenthook.Event) bool {
	if socketPath == "" {
		return false
	}
	if _, err := os.Stat(socketPath); err != nil {
		return false
	}
	if err := daemon.SendAgentHookEvent(socketPath, ev, hookIngestDaemonTimeout); err != nil {
		slog.Warn("hook-ingest: the daemon didn't take the event; saving it locally", slog.String("socket", socketPath), slog.Any("err", err))
		return false
	}
	return true
}

// hookIngestFlagsFromArgs finds --agent and --event (or -agent, --agent=x) in raw arguments.
func hookIngestFlagsFromArgs(args []string) (agent, event string) {
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
		if !strings.HasPrefix(args[i], "-") || (name != "agent" && name != "event") {
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				continue
			}
			i++
			value = args[i]
		}
		if name == "agent" {
			agent = value
		} else {
			event = value
		}
	}
	return agent, event
}
