package commands

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestLsCommandFileMode exercises `shelltime ls` against the txt file store
// (no storage config => file mode, daemon not consulted).
func TestLsCommandFileMode(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	t.Setenv("HOME", t.TempDir())
	model.InitFolder("")
	require.NoError(t, os.MkdirAll(os.ExpandEnv("$HOME/"+model.COMMAND_STORAGE_FOLDER), os.ModePerm))

	store := model.NewFileStore()
	now := time.Now()
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "make", Username: "u", Hostname: "h", Time: now}
	require.NoError(t, store.SavePre(context.Background(), cmd, now))
	post := cmd
	post.Time = now.Add(time.Second)
	require.NoError(t, store.SavePost(context.Background(), post, 0, post.Time))

	cs := model.NewMockConfigService(t)
	cs.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
	configService = cs

	app := &cli.App{Name: "mtt", Commands: []*cli.Command{LsCommand}}
	require.NoError(t, app.Run([]string{"mtt", "ls", "-f", "json"}))
	require.NoError(t, app.Run([]string{"mtt", "ls", "-f", "table"}))
}
