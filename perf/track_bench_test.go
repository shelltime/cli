//go:build unix

package perf

import (
	"os/exec"
	"testing"
)

// BenchmarkStartup is the baseline the track numbers sit on.
func BenchmarkStartup(b *testing.B) {
	// floor is a trivial binary: the runner's own fork/exec cost. It does not
	// depend on the binary under test, so a base/head difference here means
	// the run was noisy (an A/A check the report uses).
	b.Run("floor", func(b *testing.B) {
		bin, err := exec.LookPath("true")
		if err != nil {
			b.Skip(err)
		}
		s := newSandbox(b, bin, "")
		measure(b, nil, func() runStat { return s.run() })
	})

	// version is process start, package init and main's config read and
	// telemetry setup, with no command logic.
	b.Run("version", func(b *testing.B) {
		s := newSandbox(b, shelltimeBin(b), baseConfig)
		measure(b, nil, func() runStat { return s.run("--version") })
		s.requireNoErrors()
	})
}

// BenchmarkTrack is `shelltime track` as the shell hooks run it, once before
// (pre) and once after (post) every command.
func BenchmarkTrack(b *testing.B) {
	// daemon: the default install. The daemon is listening, so track hands
	// it the event over the socket and returns.
	b.Run("daemon", func(b *testing.B) {
		for _, phase := range []string{"pre", "post"} {
			b.Run(phase, func(b *testing.B) {
				s := newSandbox(b, shelltimeBin(b), baseConfig)
				d := startFakeDaemon(b)
				runs := measure(b, nil, func() runStat { return s.run(trackArgs(phase)...) })
				d.expect(b, "track_"+phase, runs)
				s.requireNoErrors()
			})
		}
	})

	// direct: no daemon. track writes the txt store itself, and on post reads
	// the whole store to decide whether to sync.
	b.Run("direct", func(b *testing.B) {
		b.Run("pre", func(b *testing.B) {
			requireNoDaemon(b)
			s := newSandbox(b, shelltimeBin(b), baseConfig)
			runs := measure(b, nil, func() runStat { return s.run(trackArgs("pre")...) })
			if got := countLines(b, s.path("commands", "pre.txt")); got != runs {
				b.Fatalf("pre.txt has %d lines after %d runs; track did not take the direct path", got, runs)
			}
			s.requireNoErrors()
		})

		// post: below the flush threshold, so no sync. This is the common
		// direct-mode post and its cost grows with the store.
		b.Run("post", func(b *testing.B) {
			requireNoDaemon(b)
			s := newSandbox(b, shelltimeBin(b), baseConfig)
			f := seedStore(s, seedPairs, 0)
			ok := 0
			verify := func() {
				if grew, moved := f.check(); grew && !moved {
					ok++
				}
			}
			reset := func() { verify(); f.restore() }
			runs := measure(b, reset, func() runStat { return s.run(trackArgs("post")...) })
			verify()
			// The first reset runs before any exec, so it never counts.
			if ok != runs {
				b.Fatalf("only %d of %d runs appended a post without syncing; track did not take the direct path", ok, runs)
			}
			s.requireNoErrors()
		})

		// post-sync: the post that reaches the flush threshold and sends the
		// batch over HTTP, once every flushCount commands in direct mode.
		b.Run("post-sync", func(b *testing.B) {
			requireNoDaemon(b)
			api := startFakeAPI(b)
			s := newSandbox(b, shelltimeBin(b),
				"token: bench-token\napiEndpoint: "+api.srv.URL+"\nflushCount: 10\n")
			f := seedStore(s, seedPairs, flushCount-1)
			ok := 0
			verify := func() {
				if grew, moved := f.check(); grew && moved {
					ok++
				}
			}
			reset := func() { verify(); f.restore() }
			runs := measure(b, reset, func() runStat { return s.run(trackArgs("post")...) })
			verify()
			if ok != runs || api.syncs.Load() != int64(runs) || api.unexpects.Load() != 0 {
				b.Fatalf("%d runs: %d appended and synced, server got %d batches of %d and %d unexpected requests",
					runs, ok, api.syncs.Load(), flushCount, api.unexpects.Load())
			}
			s.requireNoErrors()
		})
	})
}
