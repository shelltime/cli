package model

import (
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffMergeService_PrettyPrint(t *testing.T) {
	s := NewDiffMergeService()

	t.Run("no additions returns info message", func(t *testing.T) {
		out := s.PrettyPrint([]diffmatchpatch.Diff{
			{Type: diffmatchpatch.DiffEqual, Text: "unchanged"},
			{Type: diffmatchpatch.DiffDelete, Text: "gone"},
		})
		assert.Contains(t, out, "No additions detected")
	})

	t.Run("empty diff slice returns info message", func(t *testing.T) {
		out := s.PrettyPrint(nil)
		assert.Contains(t, out, "No additions detected")
	})

	t.Run("renders added lines and summary count", func(t *testing.T) {
		out := s.PrettyPrint([]diffmatchpatch.Diff{
			{Type: diffmatchpatch.DiffEqual, Text: "context\n"},
			{Type: diffmatchpatch.DiffInsert, Text: "new line one\nnew line two\n"},
		})
		require.NotEmpty(t, out)
		assert.Contains(t, out, "Added Lines")
		assert.Contains(t, out, "new line one")
		assert.Contains(t, out, "new line two")
		assert.Contains(t, out, "Summary")
		// Two inserted lines (each ending in \n => 2 newlines counted).
		assert.Contains(t, out, "Total lines added: 2")
	})

	t.Run("single insertion without trailing newline counts as one", func(t *testing.T) {
		out := s.PrettyPrint([]diffmatchpatch.Diff{
			{Type: diffmatchpatch.DiffInsert, Text: "solo"},
		})
		assert.Contains(t, out, "solo")
		assert.Contains(t, out, "Total lines added: 1")
	})
}
