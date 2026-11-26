package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// ProjectMigrationMarkerFile, in ~/.shelltime, records that the daemon moved this machine's folder
// projects into their repositories' projects, so it does it once.
const ProjectMigrationMarkerFile = "project-repositories.v1"

// ProjectMigrationMarkerPath is where the daemon's one-time project migration leaves its marker.
func ProjectMigrationMarkerPath() string {
	return filepath.Join(os.ExpandEnv("$HOME"), COMMAND_BASE_STORAGE_FOLDER, ProjectMigrationMarkerFile)
}

// genericHostNames are host names many machines share (a fresh install, a container, a CI
// runner). Another machine's folder project under one of them may exist here as a clone of
// something else, so the migration never claims them.
var genericHostNames = map[string]bool{
	"localhost":             true,
	"localhost.localdomain": true,
	"ubuntu":                true,
	"debian":                true,
	"fedora":                true,
	"archlinux":             true,
	"raspberrypi":           true,
	"runner":                true,
	"buildkitsandbox":       true,
	"docker-desktop":        true,
	"(none)":                true,
}

// ProjectMigrationHosts returns the names this machine may have reported its coding agent
// sessions under: its host name, the host name without its domain (macOS adds and drops .local),
// and the machine.name `cc install` wrote into Claude Code's settings, which keeps the name the
// machine had then. Generic names many machines share are left out.
func ProjectMigrationHosts(settingsMachineName string) []string {
	var hosts []string
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" || genericHostNames[strings.ToLower(h)] || len(hosts) >= ProjectRepositoriesMaxHosts {
			return
		}
		for _, existing := range hosts {
			if existing == h {
				return
			}
		}
		hosts = append(hosts, h)
	}
	hostname, _ := os.Hostname()
	add(hostname)
	if short, _, ok := strings.Cut(hostname, "."); ok {
		add(short)
	}
	add(settingsMachineName)
	return hosts
}

// ProjectMigrationResult is what a project migration found and did.
type ProjectMigrationResult struct {
	// Checked counts the folder projects the server listed for this machine.
	Checked int
	// Linked lists the folder projects that are clones of a repository with a remote. Without a
	// dry run they were moved into their repositories' projects.
	Linked []ProjectRepository
	// Sessions counts the sessions the server moved.
	Sessions int
	// Missing counts folders that no longer exist here (renamed, moved or deleted).
	Missing int
	// NotRepositories counts folders outside a git work tree, or in one without a remote.
	NotRepositories int
	// Truncated is true when the server matched more folders than it listed; a later run sees
	// the rest, once the folders this one moved are gone from the list.
	Truncated bool
}

// MigrateProjectRepositories moves this machine's folder projects into the projects of their git
// repositories: the server lists the folders it still groups sessions by, and every folder that
// still exists here and is a clone with a remote is sent back with its repository. Only this
// machine can answer, since the folders are on its disk. A dry run sends nothing back.
func MigrateProjectRepositories(ctx context.Context, endpoint Endpoint, hosts []string, repos *GitRepositoryResolver, dryRun bool) (ProjectMigrationResult, error) {
	var result ProjectMigrationResult
	if len(hosts) == 0 {
		return result, nil
	}
	projects, truncated, err := FetchUnresolvedProjects(ctx, endpoint, hosts)
	if err != nil {
		return result, err
	}
	result.Checked = len(projects)
	result.Truncated = truncated

	for _, p := range projects {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if info, err := os.Stat(p.Pwd); err != nil || !info.IsDir() {
			result.Missing++
			continue
		}
		repo := repos.Resolve(ctx, p.Pwd)
		if repo.URL == "" {
			result.NotRepositories++
			continue
		}
		result.Linked = append(result.Linked, ProjectRepository{Host: p.Host, Pwd: p.Pwd, Repository: repo.URL})
	}
	if dryRun {
		return result, nil
	}

	for start := 0; start < len(result.Linked); start += ProjectRepositoriesMaxProjects {
		end := min(start+ProjectRepositoriesMaxProjects, len(result.Linked))
		updated, err := SendProjectRepositories(ctx, endpoint, result.Linked[start:end])
		if err != nil {
			return result, err
		}
		result.Sessions += updated
	}
	return result, nil
}
