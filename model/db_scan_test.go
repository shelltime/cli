package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetPostCommands_CopiesLinesBeyondScannerBuffer locks the scanner-buffer
// aliasing bug: once post.txt is larger than MAX_BUFFER_SIZE, Scan shifts its
// buffer and every previously stored slice is overwritten unless copied.
func TestGetPostCommands_CopiesLinesBeyondScannerBuffer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("HOME"), ".shelltime", "commands")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	const lineCount = 12000
	lines := make([]string, lineCount)
	var b strings.Builder
	for i := 0; i < lineCount; i++ {
		lines[i] = fmt.Sprintf("line-%05d-%s", i, strings.Repeat("x", 40))
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}
	require.Greater(t, b.Len(), MAX_BUFFER_SIZE)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "post.txt"), []byte(b.String()), 0o600))

	got, n, err := GetPostCommands(context.Background())
	require.NoError(t, err)
	require.Equal(t, lineCount, n)
	require.Len(t, got, lineCount)
	for i, line := range got {
		if string(line) != lines[i] {
			t.Fatalf("line %d corrupted: got %q want %q", i, line, lines[i])
		}
	}
}
