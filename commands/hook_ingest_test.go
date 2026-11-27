package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/malamtime/cli/agenthook"
	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

const testHookJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"

type hookIngestTest struct {
	home   string
	stdout *bytes.Buffer
	config *model.MockConfigService
}

// setupHookIngest runs hook-ingest with stdin under a temp HOME, capturing stdout, with the
// default socket pointing nowhere.
func setupHookIngest(t *testing.T, stdin io.Reader) *hookIngestTest {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)

	mc := model.NewMockConfigService(t)
	stdout := &bytes.Buffer{}
	origConfig, origIn, origOut := configService, hookIngestStdin, hookIngestStdout
	origEnv, origNow, origArgs, origSocket := hookIngestGetenv, hookIngestNow, hookIngestRawArgs, trackSocketPath
	t.Cleanup(func() {
		configService, hookIngestStdin, hookIngestStdout = origConfig, origIn, origOut
		hookIngestGetenv, hookIngestNow, hookIngestRawArgs, trackSocketPath = origEnv, origNow, origArgs, origSocket
	})
	configService = mc
	hookIngestStdin = stdin
	hookIngestStdout = stdout
	hookIngestGetenv = func(string) string { return "" }
	hookIngestNow = func() time.Time { return time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC) }
	hookIngestRawArgs = func() []string { return nil }
	trackSocketPath = filepath.Join(home, "no-daemon.sock")
	return &hookIngestTest{home: home, stdout: stdout, config: mc}
}

// run runs the command with args after `hook-ingest`. Anything urfave/cli or gookit/color
// would print to stdout lands in the same buffer.
func (h *hookIngestTest) run(t *testing.T, args ...string) {
	t.Helper()
	colorOut := captureColor(t)
	app := &cli.App{Name: "t", Commands: []*cli.Command{HookIngestCommand}, Writer: h.stdout, ErrWriter: io.Discard}
	require.NoError(t, app.Run(append([]string{"t", "hook-ingest"}, args...)))
	assert.Empty(t, colorOut.String(), "nothing printed in color")
}

// assertOnlyReply checks stdout holds exactly the reply, on one line.
func (h *hookIngestTest) assertOnlyReply(t *testing.T, reply string) {
	t.Helper()
	assert.Equal(t, reply+"\n", h.stdout.String())
	assert.True(t, json.Valid(bytes.TrimSpace(h.stdout.Bytes())))
}

func pendingHookEvents(t *testing.T) []agenthook.Event {
	t.Helper()
	data, err := os.ReadFile(model.GetAgentHooksPendingFilePath())
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var events []agenthook.Event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var ev agenthook.Event
		require.NoError(t, json.Unmarshal([]byte(line), &ev))
		events = append(events, ev)
	}
	return events
}

const cursorPromptPayload = "\xEF\xBB\xBF{\"conversation_id\":\"c1\",\"hook_event_name\":\"beforeSubmitPrompt\",\"prompt\":\"hi " + testHookJWT + "\",\"workspace_roots\":[\"/tmp/x\"]}"

func TestHookIngest_RepliesOnBadInput(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		reply string
	}{
		{"malformed stdin", "garbage", []string{"--agent", "cursor", "--event", "beforeSubmitPrompt"}, `{"continue":true}`},
		{"empty stdin", "", []string{"--agent", "agy", "--event", "Stop"}, `{"decision":"allow"}`},
		{"unknown agent", `{"x":1}`, []string{"--agent", "vim", "--event", "Stop"}, `{}`},
		{"missing flags", "{}", nil, `{}`},
		{"missing agent", cursorPromptPayload, []string{"--event", "beforeSubmitPrompt"}, `{}`},
		{"positional arguments", "{}", []string{"--agent", "cursor", "--event", "stop", "extra", "args"}, `{}`},
		{"help flag", "{}", []string{"--agent", "agy", "--event", "Stop", "--help"}, `{"decision":"allow"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := setupHookIngest(t, strings.NewReader(tt.stdin))
			h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil).Maybe()
			hookIngestRawArgs = func() []string { return append([]string{"shelltime", "hook-ingest"}, tt.args...) }

			h.run(t, tt.args...)

			h.assertOnlyReply(t, tt.reply)
		})
	}
}

func TestHookIngest_UsageErrorStillAnswers(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(`{"conversationId":"a","terminationReason":"completed"}`))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
	args := []string{"--bogus", "--agent=agy", "--event", "Stop"}
	hookIngestRawArgs = func() []string { return append([]string{"shelltime", "hook-ingest"}, args...) }

	h.run(t, args...)

	h.assertOnlyReply(t, `{"decision":"allow"}`)
	events := pendingHookEvents(t)
	require.Len(t, events, 1, "the event is recorded from the raw arguments")
	assert.Equal(t, agenthook.EventStop, events[0].EventType)
}

func TestHookIngest_NoDaemonSavesMaskedEvent(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorPromptPayload))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		DataMasking: new(true),
		SocketPath:  model.DefaultSocketPath,
	}, nil)

	h.run(t, "--agent", "cursor", "--event", "beforeSubmitPrompt")

	h.assertOnlyReply(t, `{"continue":true}`)
	events := pendingHookEvents(t)
	require.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, agenthook.AgentCursor, ev.Agent)
	assert.Equal(t, agenthook.EventPromptSubmit, ev.EventType)
	assert.Equal(t, "c1", ev.SessionID)
	assert.Equal(t, "/tmp/x", ev.Workspace)
	assert.Equal(t, "2026-11-27T09:00:00Z", ev.Timestamp)
	assert.NotEmpty(t, ev.Hostname)
	assert.Regexp(t, `^[0-9a-f]{32}$`, ev.ID)
	assert.True(t, strings.HasPrefix(ev.Prompt, "hi "))
	assert.NotContains(t, ev.Prompt, testHookJWT, "masked before it is saved")

	info, err := os.Stat(model.GetAgentHooksPendingFilePath())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestHookIngest_NoDaemonAppliesContentAndExcludeSettings(t *testing.T) {
	payload := `{"conversation_id":"c1","tool_name":"Shell","tool_input":{"command":"vault read secret/x"},"cwd":"/repo"}`
	h := setupHookIngest(t, strings.NewReader(payload))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Exclude:    []string{`^vault `},
		AgentHooks: &model.AgentHooksConfig{SendContent: new(false)},
	}, nil)

	h.run(t, "--agent", "cursor", "--event", "postToolUse")

	h.assertOnlyReply(t, `{}`)
	events := pendingHookEvents(t)
	require.Len(t, events, 1)
	assert.Empty(t, events[0].Command)
	assert.Empty(t, events[0].ToolInput)
	assert.Equal(t, true, events[0].Metadata["excluded"])
	assert.Equal(t, "Shell", events[0].ToolName)
}

func TestHookIngest_EventNameFromPayload(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorPromptPayload))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)

	h.run(t, "--agent", "cursor")

	h.assertOnlyReply(t, `{"continue":true}`)
	events := pendingHookEvents(t)
	require.Len(t, events, 1)
	assert.Equal(t, "beforeSubmitPrompt", events[0].HookEventName)
}

func TestHookIngest_ConfigErrorDropsEvent(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorPromptPayload))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, errors.New("no config"))

	h.run(t, "--agent", "cursor", "--event", "beforeSubmitPrompt")

	h.assertOnlyReply(t, `{"continue":true}`)
	assert.Empty(t, pendingHookEvents(t))
}

func TestHookIngest_PanicStillAnswersOnce(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(`{"conversationId":"a"}`))
	hookIngestNow = func() time.Time { panic("boom") }

	h.run(t, "--agent", "agy", "--event", "Stop")

	h.assertOnlyReply(t, `{"decision":"allow"}`)
}

func TestHookIngest_OversizedPayload(t *testing.T) {
	big := `{"conversation_id":"c1","workspace_roots":["/repo"],"prompt":"` + strings.Repeat("x", hookIngestStdinLimit) + `"}`
	h := setupHookIngest(t, strings.NewReader(big))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)

	h.run(t, "--agent", "cursor", "--event", "beforeSubmitPrompt")

	h.assertOnlyReply(t, `{"continue":true}`)
	events := pendingHookEvents(t)
	require.Len(t, events, 1, "the event is still recorded")
	ev := events[0]
	assert.Equal(t, agenthook.AgentCursor, ev.Agent)
	assert.Equal(t, agenthook.EventPromptSubmit, ev.EventType)
	assert.Equal(t, "beforeSubmitPrompt", ev.HookEventName)
	assert.Equal(t, "2026-11-27T09:00:00Z", ev.Timestamp)
	assert.NotEmpty(t, ev.Hostname)
	assert.Equal(t, "c1", ev.SessionID, "fields before the cut are kept")
	assert.Equal(t, "/repo", ev.Workspace)
	assert.Empty(t, ev.Prompt, "the field cut short is not")
	assert.Equal(t, true, ev.Metadata["truncated"])
}

func TestHookIngest_StdinThatNeverEnds(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		w.Close()
		r.Close()
	})
	_, _ = w.Write([]byte(`{"conversation_id":"c1"`))
	h := setupHookIngest(t, r)

	start := time.Now()
	h.run(t, "--agent", "cursor", "--event", "stop")

	assert.Less(t, time.Since(start), 3*time.Second)
	h.assertOnlyReply(t, `{}`)
}

// fakeHookDaemon listens on a unix socket and answers each agent_hook message with reply
// (none when empty), handing the messages to the returned channel.
func fakeHookDaemon(t *testing.T, reply string) (string, <-chan daemon.SocketMessage) {
	t.Helper()
	socketPath := shortSocketPath(t)
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	got := make(chan daemon.SocketMessage, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var msg daemon.SocketMessage
			if json.NewDecoder(conn).Decode(&msg) == nil {
				got <- msg
				if reply != "" {
					_, _ = conn.Write([]byte(reply + "\n"))
				}
			}
			conn.Close()
		}
	}()
	return socketPath, got
}

func TestHookIngest_SendsToDaemon(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorPromptPayload))
	socketPath, got := fakeHookDaemon(t, `{"status":"ok"}`)
	trackSocketPath = socketPath

	h.run(t, "--agent", "cursor", "--event", "beforeSubmitPrompt")

	h.assertOnlyReply(t, `{"continue":true}`)
	select {
	case msg := <-got:
		assert.Equal(t, daemon.SocketMessageTypeAgentHook, msg.Type)
		raw, err := json.Marshal(msg.Payload)
		require.NoError(t, err)
		var ev agenthook.Event
		require.NoError(t, json.Unmarshal(raw, &ev))
		assert.Equal(t, agenthook.AgentCursor, ev.Agent)
		assert.Equal(t, agenthook.EventPromptSubmit, ev.EventType)
		assert.Equal(t, "beforeSubmitPrompt", ev.HookEventName)
		assert.Equal(t, "c1", ev.SessionID)
		assert.Equal(t, "/tmp/x", ev.Workspace)
		assert.Equal(t, agenthook.SurfaceCLI, ev.Surface)
		assert.NotEmpty(t, ev.Hostname)
		assert.NotEmpty(t, ev.ID)
		assert.Contains(t, ev.Prompt, testHookJWT, "the daemon applies the privacy settings")
	case <-time.After(time.Second):
		t.Fatal("the daemon got no message")
	}
	assert.Empty(t, pendingHookEvents(t), "nothing saved locally")
}

func TestHookIngest_DaemonOnConfiguredSocket(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(`{"conversationId":"a"}`))
	socketPath, got := fakeHookDaemon(t, `{"status":"ok"}`)
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{SocketPath: socketPath}, nil)

	h.run(t, "--agent", "agy", "--event", "SessionStart")

	h.assertOnlyReply(t, `{}`)
	select {
	case msg := <-got:
		assert.Equal(t, daemon.SocketMessageTypeAgentHook, msg.Type)
	case <-time.After(time.Second):
		t.Fatal("the daemon got no message")
	}
	assert.Empty(t, pendingHookEvents(t))
}

func TestHookIngest_DaemonWithoutAckFallsBackToFile(t *testing.T) {
	for name, reply := range map[string]string{"no answer": "", "disabled": `{"status":"disabled"}`} {
		t.Run(name, func(t *testing.T) {
			h := setupHookIngest(t, strings.NewReader(cursorPromptPayload))
			socketPath, got := fakeHookDaemon(t, reply)
			trackSocketPath = socketPath
			h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{SocketPath: model.DefaultSocketPath}, nil)

			h.run(t, "--agent", "cursor", "--event", "beforeSubmitPrompt")

			h.assertOnlyReply(t, `{"continue":true}`)
			<-got
			assert.Len(t, pendingHookEvents(t), 1)
		})
	}
}

func TestHookIngestFlagsFromArgs(t *testing.T) {
	agent, event, project := hookIngestFlagsFromArgs([]string{"shelltime", "hook-ingest", "--agent", "cursor", "--event", "stop"})
	assert.Equal(t, "cursor", agent)
	assert.Equal(t, "stop", event)
	assert.False(t, project)

	agent, event, project = hookIngestFlagsFromArgs([]string{"-agent=agy", "--x", "-event=Stop"})
	assert.Equal(t, "agy", agent)
	assert.Equal(t, "Stop", event)
	assert.False(t, project)

	agent, event, project = hookIngestFlagsFromArgs([]string{"agent", "cursor", "--event"})
	assert.Empty(t, agent)
	assert.Empty(t, event)
	assert.False(t, project)

	agent, event, project = hookIngestFlagsFromArgs([]string{"--agent", "cursor", "--project", "--event", "stop"})
	assert.Equal(t, "cursor", agent)
	assert.Equal(t, "stop", event, "--project takes no value")
	assert.True(t, project)

	_, _, project = hookIngestFlagsFromArgs([]string{"--project=false"})
	assert.False(t, project)
	_, _, project = hookIngestFlagsFromArgs([]string{"-project=true"})
	assert.True(t, project)
}

// legacyHookCommand is the command older versions registered in the user-level hook files.
func legacyHookCommand(bin, agent, event string) string {
	return bin + " hook-ingest --agent " + agent + " --event " + event
}

// installUserCursorHooks writes what an older version installed in ~/.cursor/hooks.json under
// the test's HOME, with a binary that exists (the test binary) unless bin is given.
func installUserCursorHooks(t *testing.T, bin string) {
	t.Helper()
	installUserHooksFor(t, model.NewCursorHooksService().Path(), "cursor", agenthook.CursorEvents, bin)
}

func installUserHooksFor(t *testing.T, path, agent string, events []string, bin string) {
	t.Helper()
	if bin == "" {
		exe, err := os.Executable()
		require.NoError(t, err)
		bin = exe
	}
	var data []byte
	var err error
	if agent == "cursor" {
		hooks := map[string]any{}
		for _, event := range events {
			hooks[event] = []any{map[string]any{"command": legacyHookCommand(bin, agent, event)}}
		}
		data, err = json.Marshal(map[string]any{"version": 1, "hooks": hooks})
	} else {
		group := map[string]any{}
		for _, event := range events {
			group[event] = []any{map[string]any{"type": "command", "command": legacyHookCommand(bin, agent, event), "timeout": 15}}
		}
		data, err = json.Marshal(map[string]any{"shelltime": group})
	}
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, data, 0644))
}

const cursorStopPayload = `{"conversation_id":"c1","status":"completed","workspace_roots":["/tmp/x"]}`

func TestHookIngest_ProjectHookRecordsWithoutUserHooks(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorStopPayload))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)

	h.run(t, "--agent", "cursor", "--event", "stop", "--project")

	h.assertOnlyReply(t, `{}`)
	events := pendingHookEvents(t)
	require.Len(t, events, 1)
	assert.Equal(t, agenthook.EventStop, events[0].EventType)
	assert.Empty(t, events[0].Cloud)
}

func TestHookIngest_ProjectHookLeavesTheEventToUserHooks(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorStopPayload))
	installUserCursorHooks(t, "")

	h.run(t, "--agent", "cursor", "--event", "stop", "--project")

	h.assertOnlyReply(t, `{}`)
	assert.Empty(t, pendingHookEvents(t), "the user's own hook records it")
}

func TestHookIngest_UserHookRecordsNextToProjectHook(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorStopPayload))
	h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
	installUserCursorHooks(t, "")

	h.run(t, "--agent", "cursor", "--event", "stop")

	assert.Len(t, pendingHookEvents(t), 1)
}

func TestHookIngest_ProjectHookRecordsWhenUserHooksCantRun(t *testing.T) {
	t.Run("their binary is gone", func(t *testing.T) {
		h := setupHookIngest(t, strings.NewReader(cursorStopPayload))
		h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
		installUserCursorHooks(t, filepath.Join(h.home, "gone", "shelltime"))

		h.run(t, "--agent", "cursor", "--event", "stop", "--project")

		assert.Len(t, pendingHookEvents(t), 1)
	})

	t.Run("they don't register the event", func(t *testing.T) {
		h := setupHookIngest(t, strings.NewReader(cursorStopPayload))
		h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
		installUserHooksFor(t, model.NewCursorHooksService().Path(), "cursor", []string{"beforeSubmitPrompt"}, "")

		h.run(t, "--agent", "cursor", "--event", "stop", "--project")

		assert.Len(t, pendingHookEvents(t), 1)
	})

	t.Run("their file isn't valid", func(t *testing.T) {
		h := setupHookIngest(t, strings.NewReader(cursorStopPayload))
		h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
		path := model.NewCursorHooksService().Path()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(`{"hooks":`), 0644))

		h.run(t, "--agent", "cursor", "--event", "stop", "--project")

		assert.Len(t, pendingHookEvents(t), 1)
	})
}

func TestHookIngest_ProjectHookRecordsCloudEvents(t *testing.T) {
	t.Run("a Cursor Cloud Agent's session start", func(t *testing.T) {
		h := setupHookIngest(t, strings.NewReader(`{"session_id":"c1","is_background_agent":true,"composer_mode":"agent"}`))
		h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
		installUserCursorHooks(t, "") // a setup script wrote some; Cloud Agents never run them

		h.run(t, "--agent", "cursor", "--event", "sessionStart", "--project")

		events := pendingHookEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, agenthook.CloudSignalCursorBackgroundAgent, events[0].Cloud)
	})

	t.Run("SHELLTIME_CLOUD in the environment", func(t *testing.T) {
		h := setupHookIngest(t, strings.NewReader(`{"conversationId":"a1","terminationReason":"completed"}`))
		h.config.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
		installUserHooksFor(t, model.NewAntigravityHooksService().Path(), "agy", agenthook.AntigravityEvents, "")
		hookIngestGetenv = func(k string) string { return map[string]string{"SHELLTIME_CLOUD": "true"}[k] }

		h.run(t, "--agent", "agy", "--event", "Stop", "--project")

		h.assertOnlyReply(t, `{"decision":"allow"}`)
		events := pendingHookEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, agenthook.AgentAntigravity, events[0].Agent)
		assert.Equal(t, agenthook.CloudSignalEnv, events[0].Cloud)
	})
}

func TestHookIngest_ProjectFlagAfterAUsageError(t *testing.T) {
	h := setupHookIngest(t, strings.NewReader(cursorStopPayload))
	installUserCursorHooks(t, "")
	args := []string{"--bogus", "--agent", "cursor", "--event", "stop", "--project"}
	hookIngestRawArgs = func() []string { return append([]string{"shelltime", "hook-ingest"}, args...) }

	h.run(t, args...)

	h.assertOnlyReply(t, `{}`)
	assert.Empty(t, pendingHookEvents(t), "--project is read from the raw arguments too")
}

func TestUserHooksRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	assert.False(t, userHooksRecord(agenthook.AgentCursor, "stop"), "no hook file")
	assert.False(t, userHooksRecord("codex", "stop"))

	installUserCursorHooks(t, "")
	assert.True(t, userHooksRecord(agenthook.AgentCursor, "stop"))
	assert.True(t, userHooksRecord(agenthook.AgentCursor, "STOP"))
	assert.False(t, userHooksRecord(agenthook.AgentCursor, "preToolUse"), "an event ShellTime doesn't register")
	assert.False(t, userHooksRecord(agenthook.AgentCursor, ""))
	assert.False(t, userHooksRecord(agenthook.AgentAntigravity, "Stop"), "another agent's file")
}

func TestHookBinaryExists(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	assert.True(t, hookBinaryExists(exe))
	assert.False(t, hookBinaryExists(filepath.Join(t.TempDir(), "shelltime")))
	assert.False(t, hookBinaryExists(t.TempDir()), "a folder isn't a binary")
	assert.False(t, hookBinaryExists("sh"), "a project command's PATH name isn't an older version's hook")
}

func TestUserHooksRecord_ProjectCommandsInTheUserFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := model.NewCursorHooksService().Path()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	data, err := json.Marshal(map[string]any{"version": 1, "hooks": map[string]any{
		"stop": []any{map[string]any{"command": model.CursorProjectHookCommand("stop")}},
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	assert.False(t, userHooksRecord(agenthook.AgentCursor, "stop"), "the project hook must not leave the event to itself")
}
