package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v2"
)

func TestDaemonRestartIsReinstall(t *testing.T) {
	for _, name := range []string{"reinstall", "restart"} {
		var matches []*cli.Command
		for _, sub := range DaemonCommand.Subcommands {
			if sub.HasName(name) {
				matches = append(matches, sub)
			}
		}
		if assert.Len(t, matches, 1, "daemon %s", name) {
			assert.Same(t, DaemonReinstallCommand, matches[0], "daemon %s", name)
		}
	}
}
