//go:build !windows

package commands

import (
	"os/exec"
	"syscall"
)

// applyDetachAttrs puts the child in its own session, detaching it from the
// controlling terminal and process group. Without this, Ctrl-C or the shell
// exiting would kill a binary swap midway through.
func applyDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
