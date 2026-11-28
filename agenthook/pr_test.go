package agenthook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize_CursorPullRequests(t *testing.T) {
	common := `"conversation_id":"conv-1","model":"claude-4.5-sonnet","workspace_roots":["/repo"]`

	t.Run("extracts PR URL from gh pr create plain tool_output without keeping tool_output", func(t *testing.T) {
		body := `{` + common + `,"tool_name":"Shell","tool_input":{"command":"git push && gh pr create --fill"},"tool_output":"Creating pull request for feat into main\nSECRET_LINE\nhttps://GitHub.com/acme/web/pull/42/files?w=1\nhttps://github.com/acme/web/pull/42\n"}`
		ev, err := Normalize("cursor", CursorPostToolUse, []byte(body), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://github.com/acme/web/pull/42"}, ev.PullRequestURLs)
		assert.NotContains(t, mustJSON(t, ev), "SECRET_LINE", "raw tool_output must never be stored")
	})

	t.Run("handles bash -lc quoted gh pr create", func(t *testing.T) {
		body := `{` + common + `,"tool_name":"Shell","tool_input":{"command":"bash -lc \"gh pr create --title test\""},"tool_output":"https://github.com/acme/cli/pull/101"}`
		ev, err := Normalize("cursor", CursorPostToolUse, []byte(body), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://github.com/acme/cli/pull/101"}, ev.PullRequestURLs)
	})

	t.Run("structured output scans stdout only and ignores stderr existing-PR notice", func(t *testing.T) {
		body := `{` + common + `,"tool_name":"Shell","tool_input":{"command":"gh pr create --fill"},"tool_output":{"stdout":"https://github.com/acme/web/pull/99\n","stderr":"a pull request for branch already exists: https://github.com/acme/web/pull/10","exit_code":0}}`
		ev, err := Normalize("cursor", CursorPostToolUse, []byte(body), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://github.com/acme/web/pull/99"}, ev.PullRequestURLs)
	})

	t.Run("structured output with non-zero exit_code or isError extracts nothing", func(t *testing.T) {
		for _, out := range []string{
			`{"stdout":"https://github.com/acme/web/pull/99","exit_code":1}`,
			`{"isError":true,"content":[{"type":"text","text":"https://github.com/acme/web/pull/99"}]}`,
			`{"error":"already exists","output":"https://github.com/acme/web/pull/99"}`,
		} {
			body := `{` + common + `,"tool_name":"Shell","tool_input":{"command":"gh pr create --fill"},"tool_output":` + out + `}`
			ev, err := Normalize("cursor", CursorPostToolUse, []byte(body), testNow, nil)
			require.NoError(t, err)
			assert.Empty(t, ev.PullRequestURLs, out)
		}
	})

	t.Run("extracts PR URL from GitHub MCP create_pull_request", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			toolName string
			output   string
		}{
			{
				name:     "stringified JSON tool_output",
				toolName: "mcp__github__create_pull_request",
				output:   `"{\"id\":1,\"html_url\":\"https://github.com/acme/server/pull/55\"}"`,
			},
			{
				name:     "MCP content array tool_output",
				toolName: "MCP:github:create_pull_request",
				output:   `{"content":[{"type":"text","text":"Created https://github.com/acme/server/pull/55"}]}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body := `{` + common + `,"tool_name":"` + tc.toolName + `","tool_input":{"owner":"acme","repo":"server","head":"feat"},"tool_output":` + tc.output + `}`
				ev, err := Normalize("cursor", CursorPostToolUse, []byte(body), testNow, nil)
				require.NoError(t, err)
				assert.Equal(t, []string{"https://github.com/acme/server/pull/55"}, ev.PullRequestURLs)
			})
		}
	})

	t.Run("structured MCP response prefers canonical html_url and ignores PRs referenced in body", func(t *testing.T) {
		body := `{` + common + `,"tool_name":"mcp__github__create_pull_request","tool_input":{"owner":"acme","repo":"server"},"tool_output":{"html_url":"https://github.com/acme/server/pull/42","body":"Follow-up to https://github.com/acme/server/pull/10","title":"Fix https://github.com/acme/server/pull/9"}}`
		ev, err := Normalize("cursor", CursorPostToolUse, []byte(body), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://github.com/acme/server/pull/42"}, ev.PullRequestURLs)
	})

	t.Run("ignores non-create commands, quoted text mentions, PR URLs in command body, and postToolUseFailure", func(t *testing.T) {
		cases := []struct {
			event string
			body  string
		}{
			{
				event: CursorPostToolUse,
				body:  `{` + common + `,"tool_name":"Shell","tool_input":{"command":"gh pr view 12"},"tool_output":"https://github.com/acme/web/pull/12"}`,
			},
			{
				event: CursorPostToolUse,
				body:  `{` + common + `,"tool_name":"Shell","tool_input":{"command":"echo \"gh pr create\""},"tool_output":"https://github.com/acme/web/pull/12"}`,
			},
			{
				event: CursorPostToolUse,
				body:  `{` + common + `,"tool_name":"Shell","tool_input":{"command":"grep -rn \"gh pr create\" docs/"},"tool_output":"https://github.com/acme/web/pull/12"}`,
			},
			{
				event: CursorPostToolUse,
				body:  `{` + common + `,"tool_name":"Shell","tool_input":{"command":"gh pr create --body \"see https://github.com/acme/web/pull/1\""},"tool_output":"ok"}`,
			},
			{
				event: CursorPostToolUse,
				body:  `{` + common + `,"tool_name":"Read","tool_input":{"file_path":"/repo/README.md"},"tool_output":"https://github.com/acme/web/pull/12"}`,
			},
			{
				event: CursorPostToolUseFailure,
				body:  `{` + common + `,"tool_name":"Shell","tool_input":{"command":"gh pr create --fill"},"tool_output":"https://github.com/acme/web/pull/12","error_message":"exit 1"}`,
			},
		}
		for _, tc := range cases {
			ev, err := Normalize("cursor", tc.event, []byte(tc.body), testNow, nil)
			require.NoError(t, err)
			assert.Empty(t, ev.PullRequestURLs)
		}
	})
}

func TestNormalize_AntigravityPullRequests(t *testing.T) {
	t.Run("reads PR URL from transcript.jsonl for matching stepIdx without reading args or stderr", func(t *testing.T) {
		dir := t.TempDir()
		transcript := filepath.Join(dir, "transcript.jsonl")
		lines := "" +
			`{"step_index":3,"source":"TOOL","status":"DONE","output":"https://github.com/acme/web/pull/1"}` + "\n" +
			`{"step_index":5,"source":"TOOL","status":"DONE","args":{"CommandLine":"gh pr create --body https://github.com/acme/web/pull/2"},"stderr":"https://github.com/acme/web/pull/3","output":"Creating pull request...\nhttps://github.com/acme/web/pull/77\n"}` + "\n"
		require.NoError(t, os.WriteFile(transcript, []byte(lines), 0o600))

		body := `{"conversationId":"agy-1","workspacePaths":["/repo"],"transcriptPath":"` + filepath.ToSlash(transcript) + `","stepIdx":5,"toolCall":{"name":"run_command","args":{"CommandLine":"gh pr create --fill"}}}`
		ev, err := Normalize("agy", AntigravityPostToolUse, []byte(body), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://github.com/acme/web/pull/77"}, ev.PullRequestURLs)
	})

	t.Run("reads direct toolOutput when present on stdin payload", func(t *testing.T) {
		body := `{"conversationId":"agy-1","workspacePaths":["/repo"],"stepIdx":2,"toolCall":{"name":"mcp__github__create_pull_request","args":{"owner":"acme","repo":"api"}},"toolOutput":{"html_url":"https://github.com/acme/api/pull/88"}}`
		ev, err := Normalize("agy", AntigravityPostToolUse, []byte(body), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://github.com/acme/api/pull/88"}, ev.PullRequestURLs)
	})

	t.Run("ignores failed PostToolUse and failed transcript rows", func(t *testing.T) {
		dir := t.TempDir()
		transcript := filepath.Join(dir, "transcript.jsonl")
		lines := `{"step_index":5,"source":"TOOL","status":"DONE","exit_code":1,"output":"https://github.com/acme/web/pull/77"}` + "\n"
		require.NoError(t, os.WriteFile(transcript, []byte(lines), 0o600))

		bodyFailedRow := `{"conversationId":"agy-1","workspacePaths":["/repo"],"transcriptPath":"` + filepath.ToSlash(transcript) + `","stepIdx":5,"toolCall":{"name":"run_command","args":{"CommandLine":"gh pr create --fill"}}}`
		ev, err := Normalize("agy", AntigravityPostToolUse, []byte(bodyFailedRow), testNow, nil)
		require.NoError(t, err)
		assert.Empty(t, ev.PullRequestURLs)

		bodyHookErr := `{"conversationId":"agy-1","workspacePaths":["/repo"],"stepIdx":5,"error":"exit status 1","toolCall":{"name":"run_command","args":{"CommandLine":"gh pr create --fill"}},"toolOutput":"https://github.com/acme/web/pull/77"}`
		ev, err = Normalize("agy", AntigravityPostToolUse, []byte(bodyHookErr), testNow, nil)
		require.NoError(t, err)
		assert.Empty(t, ev.PullRequestURLs)
	})

	t.Run("missing or unflushed transcript returns no URLs and never fails", func(t *testing.T) {
		body := `{"conversationId":"agy-1","workspacePaths":["/repo"],"transcriptPath":"/no/such/transcript.jsonl","stepIdx":5,"toolCall":{"name":"run_command","args":{"CommandLine":"gh pr create --fill"}}}`
		ev, err := Normalize("agy", AntigravityPostToolUse, []byte(body), testNow, nil)
		require.NoError(t, err)
		assert.Empty(t, ev.PullRequestURLs)
	})
}
