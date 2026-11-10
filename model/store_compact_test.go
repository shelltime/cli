package model

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The file compaction decides whether a pre command is finished and synced with one lookup
// in latestPostTimes. That has to be the answer preHasSyncedPost gives, which compares the
// pre with every post and is still what the bolt store uses.
func TestLatestPostTimes_AnswersLikePreHasSyncedPost(t *testing.T) {
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	commands := []string{"ls", "git status", "make build", ""}

	for seed := int64(1); seed <= 300; seed++ {
		rng := rand.New(rand.NewSource(seed))
		random := func() *Command {
			at := base.Add(time.Duration(rng.Intn(120)) * time.Second)
			return &Command{
				Shell:         []string{"zsh", "bash"}[rng.Intn(2)],
				SessionID:     int64(1 + rng.Intn(2)),
				Command:       commands[rng.Intn(len(commands))],
				Username:      "u",
				Time:          at,
				RecordingTime: at.Add(time.Duration(rng.Intn(3)-1) * time.Second),
			}
		}
		var pres, posts []*Command
		for range rng.Intn(30) {
			pres = append(pres, random())
		}
		for range rng.Intn(30) {
			posts = append(posts, random())
		}
		// Somewhere among the recording times, often exactly on one
		cursor := base.Add(time.Duration(rng.Intn(124)-2) * time.Second)

		latestSynced := latestPostTimes(posts, &cursor)
		latestAny := latestPostTimes(posts, nil)
		for _, pre := range pres {
			synced, ok := latestSynced[pre.GetUniqueKey()]
			got := ok && !synced.Before(pre.Time)
			require.Equal(t, preHasSyncedPost(pre, posts, cursor), got, "seed %d, pre %+v, cursor %s", seed, pre, cursor)

			// And without a cursor: is there any post of its key at or after its start
			any, ok := latestAny[pre.GetUniqueKey()]
			want := false
			for _, post := range posts {
				if post.GetUniqueKey() == pre.GetUniqueKey() && !post.Time.Before(pre.Time) {
					want = true
				}
			}
			require.Equal(t, want, ok && !any.Before(pre.Time), "seed %d, pre %+v", seed, pre)
		}
	}
}
