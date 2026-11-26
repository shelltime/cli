package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/malamtime/cli/model"
)

// projectMigrationDelay keeps the daemon's one-time project migration out of its startup.
const projectMigrationDelay = 2 * time.Minute

// RunProjectMigrationOnce moves this machine's folder projects into their git repositories'
// projects, once per machine (see model.MigrateProjectRepositories): sessions recorded before
// the daemon named repositories are grouped by folder on the server, and only this machine can
// tell which repository a folder is a clone of. It runs in the background a while after start
// and leaves a marker when it is done; a server that doesn't support it yet, or a failure,
// leaves none, so the next start tries again. `shelltime cc projects` runs it by hand.
func RunProjectMigrationOnce(ctx context.Context, cfg model.ShellTimeConfig) {
	runProjectMigrationOnce(ctx, cfg, projectMigrationDelay, model.ProjectMigrationMarkerPath())
}

type projectMigrationMarker struct {
	MigratedAt time.Time `json:"migratedAt"`
	Checked    int       `json:"checked"`
	Linked     int       `json:"linked"`
	Sessions   int       `json:"sessions"`
}

func runProjectMigrationOnce(ctx context.Context, cfg model.ShellTimeConfig, delay time.Duration, markerPath string) {
	if cfg.Token == "" {
		return
	}
	if _, err := os.Stat(markerPath); err == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	endpoint := model.Endpoint{Token: cfg.Token, APIEndpoint: cfg.APIEndpoint}
	hosts := model.ProjectMigrationHosts(model.NewClaudeSettingsAICodeOtelEnvService().ResourceMachineName())
	result, err := model.MigrateProjectRepositories(ctx, endpoint, hosts, model.NewGitRepositoryResolver(time.Hour), false)
	if err != nil {
		var statusErr *model.HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			slog.Debug("Project migration skipped: the server can't group projects by repository yet")
			return
		}
		slog.Warn("Project migration failed", slog.Any("err", err))
		return
	}

	marker, _ := json.Marshal(projectMigrationMarker{
		MigratedAt: time.Now().UTC(),
		Checked:    result.Checked,
		Linked:     len(result.Linked),
		Sessions:   result.Sessions,
	})
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err == nil {
		err = os.WriteFile(markerPath, marker, 0o600)
	}
	if err != nil {
		slog.Warn("Failed to write the project migration marker", slog.Any("err", err))
	}
	slog.Info("Project migration finished",
		slog.Int("checked", result.Checked),
		slog.Int("linked", len(result.Linked)),
		slog.Int("sessions", result.Sessions),
		slog.Int("missing", result.Missing),
		slog.Int("notRepositories", result.NotRepositories))
}
