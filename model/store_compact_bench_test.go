package model

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// BenchmarkFileStorePrune is the compaction right after a sync: what the daemon runs after
// every batch it sends, and `shelltime track` without a daemon after the batch it sends
// once every flushCount commands. synced is how many finished commands earlier syncs left
// in the store (none once every sync is followed by a compaction; hundreds in a store that
// was only compacted at shell start); ten more have just been sent.
func BenchmarkFileStorePrune(b *testing.B) {
	for _, synced := range []int{0, 500} {
		b.Run(fmt.Sprintf("synced-%d", synced), func(b *testing.B) {
			b.Setenv("HOME", b.TempDir())
			ctx := context.Background()
			store := newFileStore()

			base := time.Now().Add(-6 * time.Hour)
			var cursor time.Time
			for i := range synced + 10 {
				at := base.Add(time.Duration(i) * 3 * time.Second)
				cmd := Command{
					Shell: "zsh", SessionID: 20240601120000, Username: "u", Hostname: "h", Time: at, PPID: 4242,
					Command: fmt.Sprintf("git commit -m 'change number %d' --no-verify", i),
				}
				if err := store.SavePre(ctx, cmd, at); err != nil {
					b.Fatal(err)
				}
				cmd.Time = at.Add(800 * time.Millisecond)
				if err := store.SavePost(ctx, cmd, 0, cmd.Time); err != nil {
					b.Fatal(err)
				}
				cursor = cmd.Time
			}
			// Two commands still running
			for i := range 2 {
				cmd := Command{Shell: "zsh", SessionID: 20240601120000, Username: "u", Hostname: "h", Time: cursor.Add(time.Second), Command: fmt.Sprintf("npm run dev %d", i)}
				if err := store.SavePre(ctx, cmd, cmd.Time); err != nil {
					b.Fatal(err)
				}
			}
			if err := store.SetCursor(ctx, cursor); err != nil {
				b.Fatal(err)
			}

			files := []string{GetPreCommandFilePath(), GetPostCommandFilePath(), GetCursorFilePath()}
			seeded := make([][]byte, len(files))
			for i, path := range files {
				content, err := os.ReadFile(path)
				if err != nil {
					b.Fatal(err)
				}
				seeded[i] = content
			}

			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				for i, path := range files {
					if err := os.WriteFile(path, seeded[i], 0o600); err != nil {
						b.Fatal(err)
					}
				}
				b.StartTimer()
				if err := store.Prune(ctx, cursor); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if pres, err := store.GetPreCommands(ctx); err != nil || len(pres) != 2 {
				b.Fatalf("after the prune %d pre commands are left (err %v), want the 2 still running", len(pres), err)
			}
		})
	}
}
