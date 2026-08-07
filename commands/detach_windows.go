//go:build windows

package commands

import "os/exec"

// applyDetachAttrs is a no-op on Windows: there is no daemon to repair there, so
// this path is never reached.
func applyDetachAttrs(cmd *exec.Cmd) {}
