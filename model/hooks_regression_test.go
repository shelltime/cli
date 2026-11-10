package model

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stands in for bash-preexec.sh, which returns early when it is sourced again,
// so the hook arrays keep what an earlier source of the hook added to them.
const bashPreexecStub = `if [[ -n "${bash_preexec_imported:-}" ]]; then
    return 0
fi
bash_preexec_imported="defined"
declare -a preexec_functions
declare -a precmd_functions
`

// Records each call in $SHELLTIME_LOG instead of tracking anything.
const shelltimeStub = `#!/bin/sh
printf '%s\n' "$*" >> "$SHELLTIME_LOG"
`

// Sources the hook twice, as `source ~/.bashrc` does in a shell that already
// loaded it, lists the hook arrays, then runs one command through them the way
// bash-preexec does.
const bashSourceTwiceScript = `source "$1" || exit 1
source "$1" || exit 1
for f in "${preexec_functions[@]}"; do echo "preexec_functions $f"; done
for f in "${precmd_functions[@]}"; do echo "precmd_functions $f"; done
for f in "${preexec_functions[@]}"; do "$f" "ls -la"; done
for f in "${precmd_functions[@]}"; do "$f"; done
`

func TestEmbeddedBashHookRegistersOnceWhenSourcedAgain(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}

	hooksDir := t.TempDir()
	hookPath := filepath.Join(hooksDir, "bash.bash")
	require.NoError(t, os.WriteFile(hookPath, EmbeddedBashHook, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash-preexec.sh"), []byte(bashPreexecStub), 0644))

	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "shelltime"), []byte(shelltimeStub), 0755))
	logPath := filepath.Join(t.TempDir(), "shelltime.log")

	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", bashSourceTwiceScript, "bash", hookPath)
	cmd.Dir = hooksDir
	// No BASH_ENV, and the stub comes first: the hook must never reach a real
	// shelltime or its daemon.
	cmd.Env = []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"SHELLTIME_LOG=" + logPath,
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	assert.Equal(t, 1, countLines(lines, "preexec_functions preexec_invoke_cmd"), string(out))
	assert.Equal(t, 1, countLines(lines, "precmd_functions precmd_invoke_cmd"), string(out))

	logged, err := os.ReadFile(logPath)
	require.NoError(t, err)
	var pre, post int
	for _, call := range strings.Split(string(logged), "\n") {
		if !strings.HasPrefix(call, "track ") {
			continue
		}
		if strings.Contains(call, " -p=pre ") {
			pre++
		}
		if strings.Contains(call, " -p=post ") {
			post++
		}
	}
	assert.Equal(t, 1, pre, "one command must send one pre:\n%s", logged)
	assert.Equal(t, 1, post, "one command must send one post:\n%s", logged)
}

func countLines(lines []string, want string) int {
	n := 0
	for _, line := range lines {
		if line == want {
			n++
		}
	}
	return n
}

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
