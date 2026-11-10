package model

import (
	"context"
	"fmt"
	"testing"
	"time"
)

var benchExcludePatterns = []string{`^export .*(SECRET|TOKEN)`, `^mysql .* -p\S+`, `password=`}

func BenchmarkMaskSensitiveTokens(b *testing.B) {
	const command = `kubectl --context prod get pods -n payments -o wide --sort-by=.status.startTime`
	b.ReportAllocs()
	for b.Loop() {
		MaskSensitiveTokens(command)
	}
}

func BenchmarkShouldExcludeCommand(b *testing.B) {
	const command = `kubectl --context prod get pods -n payments -o wide --sort-by=.status.startTime`
	b.ReportAllocs()
	for b.Loop() {
		ShouldExcludeCommand(command, benchExcludePatterns)
	}
}

// BenchmarkBuildTrackingData is what every post command costs once it reaches the
// store: `shelltime track` without a daemon, and the daemon's track_post handler.
// The file store holds 500 synced pairs behind the cursor and pending pairs in
// front of it, as the perf module's direct post scenario does.
func BenchmarkBuildTrackingData(b *testing.B) {
	for _, pending := range []int{1, 9} {
		b.Run(fmt.Sprintf("file/pending-%d", pending), func(b *testing.B) {
			b.Setenv("HOME", b.TempDir())
			ctx := context.Background()
			store := newFileStore()

			base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
			save := func(i int) time.Time {
				at := base.Add(time.Duration(i) * time.Second)
				cmd := Command{
					Shell: "zsh", SessionID: 20240601120000, Username: "u", Hostname: "h", Time: at, PPID: 4242,
					Command: fmt.Sprintf("git commit -m 'change number %d' --no-verify", i),
				}
				if err := store.SavePre(ctx, cmd, at); err != nil {
					b.Fatal(err)
				}
				cmd.Time = at.Add(500 * time.Millisecond)
				if err := store.SavePost(ctx, cmd, 0, cmd.Time); err != nil {
					b.Fatal(err)
				}
				return cmd.Time
			}
			var cursor time.Time
			for i := range 500 {
				cursor = save(i)
			}
			if err := store.SetCursor(ctx, cursor); err != nil {
				b.Fatal(err)
			}
			for i := range pending {
				save(500 + i)
			}

			truthy := true
			config := ShellTimeConfig{DataMasking: &truthy, Exclude: benchExcludePatterns}
			b.ReportAllocs()
			for b.Loop() {
				res, err := BuildTrackingData(ctx, store, config)
				if err != nil {
					b.Fatal(err)
				}
				if len(res.Data) != pending {
					b.Fatalf("built %d commands, want %d", len(res.Data), pending)
				}
			}
		})
	}
}
