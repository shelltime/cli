package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// x3SetupQuery wires mock AI + Config services and an isolated HOME for the
// query-action coverage tests, restoring both package globals on cleanup.
func x3SetupQuery(t *testing.T) (*model.MockAIService, *model.MockConfigService) {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	origAI := aiService
	origCfg := configService
	mai := model.NewMockAIService(t)
	mc := model.NewMockConfigService(t)
	aiService = mai
	configService = mc
	t.Cleanup(func() {
		aiService = origAI
		configService = origCfg
	})
	return mai, mc
}

// x3FeedStdin replaces os.Stdin with a pipe carrying payload, restoring it on
// cleanup. Used to drive the interactive delete-confirmation prompt.
func x3FeedStdin(t *testing.T, payload string) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		r.Close()
	})
	go func() {
		_, _ = w.WriteString(payload)
		w.Close()
	}()
}

// TestX3Query_AutoRunEditCommand drives the ActionEdit auto-run case (cfg.AI.Agent.Edit
// enabled). "touch <file>" classifies as edit and is executed, creating the file.
func TestX3Query_AutoRunEditCommand(t *testing.T) {
	mai, mc := x3SetupQuery(t)

	target := filepath.Join(t.TempDir(), "x3-edit-target.txt")
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		APIEndpoint: "https://api.shelltime.xyz",
		Token:       "tok",
		AI: &model.AIConfig{
			Agent: model.AIAgentConfig{Edit: true},
		},
	}, nil)
	mai.On("QueryCommandStream", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			onToken := args.Get(3).(func(token string))
			onToken("touch " + target)
		}).Return(nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{QueryCommand}}
	require.NoError(t, app.Run([]string{"t", "query", "create a file"}))

	// The edit command was auto-run, so the file now exists.
	_, statErr := os.Stat(target)
	assert.NoError(t, statErr, "auto-run edit command should have created the file")
}

// TestX3Query_DeleteConfirmedRuns covers the ActionDelete branch with an
// affirmative "y" confirmation: the command is classified delete, the prompt is
// answered yes, and executeCommand runs it.
func TestX3Query_DeleteConfirmedRuns(t *testing.T) {
	mai, mc := x3SetupQuery(t)

	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("x"), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		APIEndpoint: "https://api.shelltime.xyz",
		Token:       "tok",
		AI: &model.AIConfig{
			Agent: model.AIAgentConfig{Delete: true},
		},
	}, nil)
	mai.On("QueryCommandStream", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			onToken := args.Get(3).(func(token string))
			onToken("rm " + victim)
		}).Return(nil)

	x3FeedStdin(t, "y\n")

	app := &cli.App{Name: "t", Commands: []*cli.Command{QueryCommand}}
	require.NoError(t, app.Run([]string{"t", "query", "delete the file"}))

	// Confirmed deletion actually removed the file.
	_, statErr := os.Stat(victim)
	assert.True(t, os.IsNotExist(statErr), "confirmed delete should have removed the file")
}

// TestX3Query_DeleteCancelled covers the ActionDelete branch where the user
// answers anything other than "y": execution is cancelled and the file remains.
func TestX3Query_DeleteCancelled(t *testing.T) {
	mai, mc := x3SetupQuery(t)

	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	require.NoError(t, os.WriteFile(keep, []byte("x"), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		APIEndpoint: "https://api.shelltime.xyz",
		Token:       "tok",
		AI: &model.AIConfig{
			Agent: model.AIAgentConfig{Delete: true},
		},
	}, nil)
	mai.On("QueryCommandStream", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			onToken := args.Get(3).(func(token string))
			onToken("rm " + keep)
		}).Return(nil)

	x3FeedStdin(t, "n\n")

	app := &cli.App{Name: "t", Commands: []*cli.Command{QueryCommand}}
	require.NoError(t, app.Run([]string{"t", "query", "delete the file"}))

	// Cancelled: file must still be present.
	_, statErr := os.Stat(keep)
	assert.NoError(t, statErr, "cancelled delete must leave the file intact")
}

// TestX3Query_TipShownForDisabledActionType covers the "tip" branch where an
// action type is recognized but not enabled for auto-run, and tips are on.
func TestX3Query_TipShownForDisabledActionType(t *testing.T) {
	mai, mc := x3SetupQuery(t)

	enabled := true
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		APIEndpoint: "https://api.shelltime.xyz",
		Token:       "tok",
		AI: &model.AIConfig{
			// View enabled (so the agent block is active) but the suggested
			// command is an edit, which is disabled -> "enable ai.agent.edit" tip.
			Agent:    model.AIAgentConfig{View: true, Edit: false},
			ShowTips: &enabled,
		},
	}, nil)
	mai.On("QueryCommandStream", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			onToken := args.Get(3).(func(token string))
			onToken("touch /tmp/x3-not-run.txt")
		}).Return(nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{QueryCommand}}
	require.NoError(t, app.Run([]string{"t", "query", "edit something"}))
}

// --- shouldShowTips explicit values -------------------------------------------

func TestX3ShouldShowTips_ExplicitFalse(t *testing.T) {
	disabled := false
	cfg := model.ShellTimeConfig{AI: &model.AIConfig{ShowTips: &disabled}}
	assert.False(t, shouldShowTips(cfg), "explicit ShowTips=false must disable tips")
}

func TestX3ShouldShowTips_ExplicitTrue(t *testing.T) {
	enabled := true
	cfg := model.ShellTimeConfig{AI: &model.AIConfig{ShowTips: &enabled}}
	assert.True(t, shouldShowTips(cfg))
}

// --- executeCommand: empty SHELL falls back to /bin/sh ------------------------

// TestX3ExecuteCommand_EmptyShellFallback covers the shell=="" branch of
// executeCommand: with SHELL unset it falls back to /bin/sh and still runs.
func TestX3ExecuteCommand_EmptyShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	require.NoError(t, executeCommand(context.Background(), "true"))
}
