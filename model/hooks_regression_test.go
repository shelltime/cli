package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmbeddedHooksDropRepeatedEmptyCommands(t *testing.T) {
	zsh := string(EmbeddedZshHook)
	assert.Contains(t, zsh, "_SHELLTIME_LAST_CMD")
	assert.Contains(t, zsh, `_SHELLTIME_LAST_CMD=""`)

	bash := string(EmbeddedBashHook)
	// Cleared before the post is recorded so an empty enter does not re-post.
	assert.Contains(t, bash, `LAST_COMMAND=""`)
	idx := strings.Index(bash, "precmd_invoke_cmd")
	if idx < 0 {
		t.Fatal("bash hook is missing precmd_invoke_cmd")
	}
	body := bash[idx:]
	clearAt := strings.Index(body, `LAST_COMMAND=""`)
	trackAt := strings.Index(body, "shelltime track")
	assert.Greater(t, clearAt, 0)
	assert.Greater(t, trackAt, clearAt, "LAST_COMMAND must be cleared before the post is sent")
}
