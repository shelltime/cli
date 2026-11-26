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

// projectMigrationMaxPages bounds one migration run: 50 pages of the server's listing (2000
// folders each).
const projectMigrationMaxPages = 50

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
	// Unchecked counts folders git couldn't answer for (it timed out or didn't start); a later
	// run asks again.
	Unchecked int
	// Truncated is true when the server had more folders than this run went through.
	Truncated bool
}

// Complete reports whether every folder of the machine got a definitive answer, so the migration
// needn't run again.
func (r ProjectMigrationResult) Complete() bool {
	return !r.Truncated && r.Unchecked == 0
}

// MigrateProjectRepositories moves this machine's folder projects into the projects of their git
// repositories: the server lists the folders it still groups sessions by, a page at a time, and
// every folder that still exists here and is a clone with a remote is sent back with its
// repository. Only this machine can answer, since the folders are on its disk. A dry run sends
// nothing back.
func MigrateProjectRepositories(ctx context.Context, endpoint Endpoint, hosts []string, repos *GitRepositoryResolver, dryRun bool) (ProjectMigrationResult, error) {
	var result ProjectMigrationResult
	if len(hosts) == 0 {
		return result, nil
	}

	var after *UnresolvedProject
	for page := 0; page < projectMigrationMaxPages; page++ {
		projects, truncated, err := FetchUnresolvedProjects(ctx, endpoint, hosts, after)
		if err != nil {
			return result, err
		}
		result.Checked += len(projects)
		result.Truncated = truncated

		var linked []ProjectRepository
		for _, p := range projects {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if info, err := os.Stat(p.Pwd); err != nil || !info.IsDir() {
				result.Missing++
				continue
			}
			repo, definitive := repos.Lookup(ctx, p.Pwd)
			switch {
			case !definitive:
				result.Unchecked++
			case repo.URL == "":
				result.NotRepositories++
			default:
				linked = append(linked, ProjectRepository{Host: p.Host, Pwd: p.Pwd, Repository: repo.URL})
			}
		}
		result.Linked = append(result.Linked, linked...)

		if !dryRun {
			for start := 0; start < len(linked); start += ProjectRepositoriesMaxProjects {
				end := min(start+ProjectRepositoriesMaxProjects, len(linked))
				updated, err := SendProjectRepositories(ctx, endpoint, linked[start:end])
				if err != nil {
					return result, err
				}
				result.Sessions += updated
			}
		}

		if !truncated || len(projects) == 0 {
			break
		}
		after = &projects[len(projects)-1]
	}
	return result, nil
}
