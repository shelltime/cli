package daemon

import (
	"os"
	"testing"
)

// BenchmarkResolveTerminal walks up from the test process, as the daemon does from the
// shell's PPID before it sends a batch of commands.
func BenchmarkResolveTerminal(b *testing.B) {
	pid := os.Getpid()
	b.ReportAllocs()
	for b.Loop() {
		ResolveTerminal(pid)
	}
}
