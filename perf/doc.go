// Package perf benchmarks the latency of the real `shelltime` binary.
//
// `shelltime track` runs in the foreground of every shell prompt (see
// model/hooks), so what users feel is the whole process: exec, Go runtime and
// package init, config read, then the track logic. The benchmarks here exec a
// built binary per iteration, in an isolated $HOME, the same way the hooks do.
//
// This is a separate module so its tooling dependencies (golang.org/x/perf)
// never take part in version selection for the shipped CLI and daemon.
//
// Run against the current tree:
//
//	go -C perf test -run '^$' -bench . -benchtime 50x -count 5
//
// Compare two commits the way CI does:
//
//	perf/compare.sh origin/main
//
// See perf/README.md for the scenarios and how the report is read.
package perf
