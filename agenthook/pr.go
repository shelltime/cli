package agenthook

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const maxPullRequestURLsPerEvent = 20

var (
	// ghPRCreatePattern finds `gh pr create` in a shell command, including when chained
	// or invoked via a shell `-c` wrapper (`bash -lc "gh pr create ..."`), without matching
	// arbitrary quoted text such as `echo "gh pr create"`.
	ghPRCreatePattern = regexp.MustCompile(`(?:^|[\s;&|(]|-[a-zA-Z]*c\s+["'])gh\s+pr\s+create\b`)
	// prURLPattern finds pull request URLs in tool output text.
	prURLPattern = regexp.MustCompile(`https?://[^\s"'<>)\]]+/[^\s"'<>)\]]+/[^\s"'<>)\]]+/pull/\d+`)
)

// prCanonicalURLKeys name fields on a structured PR-creation response (such as GitHub MCP
// `create_pull_request`) that carry the created pull request's canonical URL directly.
// When any of these holds a PR URL, sibling prose fields like `body` are never scanned.
var prCanonicalURLKeys = []string{
	"html_url",
	"htmlUrl",
	"pull_request_url",
	"pullRequestUrl",
	"pr_url",
	"prUrl",
	"url",
}

// prOutputIgnoredKeys are JSON object keys that hold tool inputs, stderr, or prose fields
// rather than created-PR output, so URLs mentioned in PR bodies or `gh` stderr notices are ignored.
var prOutputIgnoredKeys = map[string]bool{
	"stderr":      true,
	"error":       true,
	"command":     true,
	"commandline": true,
	"args":        true,
	"input":       true,
	"tool_input":  true,
	"toolinput":   true,
	"tool_call":   true,
	"toolcall":    true,
	"body":        true,
	"description": true,
	"title":       true,
}

// antigravityTranscriptOutputKeys name the fields on a transcript.jsonl tool step row
// that may hold tool output (never command args or stderr).
var antigravityTranscriptOutputKeys = []string{
	"output",
	"result",
	"response",
	"tool_output",
	"toolOutput",
	"tool_result",
	"toolResult",
	"tool_response",
	"toolResponse",
	"command_output",
	"commandOutput",
	"combined_output",
	"combinedOutput",
	"run_command_output",
	"runCommandOutput",
	"content",
}

// isPRCreateToolCall reports whether a completed tool call ran `gh pr create` or called a
// GitHub MCP `create_pull_request` tool.
func isPRCreateToolCall(toolName, command string) bool {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(toolName)), "create_pull_request") {
		return true
	}
	return command != "" && ghPRCreatePattern.MatchString(command)
}

func extractCursorToolPRURLs(p payload) []string {
	for _, key := range []string{"tool_output", "tool_response"} {
		if raw, ok := p[key]; ok {
			if urls := extractPullRequestURLsFromRaw(raw); len(urls) > 0 {
				return urls
			}
		}
	}
	return nil
}

func extractAntigravityToolPRURLs(p payload, transcriptPath string) []string {
	for _, key := range []string{"toolOutput", "toolResult", "toolResponse", "result", "output"} {
		if raw, ok := p[key]; ok {
			if urls := extractPullRequestURLsFromRaw(raw); len(urls) > 0 {
				return urls
			}
		}
	}
	if transcriptPath == "" {
		return nil
	}
	stepIdx, ok := p.nonNegInt("stepIdx")
	if !ok {
		return nil
	}
	return extractAntigravityTranscriptPRURLs(transcriptPath, stepIdx)
}

// extractPullRequestURLsFromRaw extracts distinct normalized pull request URLs from a
// JSON tool output value (string, object, or array). A structured object that reports an
// error or non-zero exit code yields none; one with a canonical PR URL field (`html_url`,
// `url`, ...) uses that alone; one with a `stdout` field scans `stdout` alone so `gh`
// stderr notices about existing PRs are never linked.
func extractPullRequestURLsFromRaw(raw json.RawMessage) []string {
	var out []string
	seen := map[string]bool{}
	collectPullRequestURLsFromRaw(raw, seen, &out)
	return out
}

func collectPullRequestURLsFromRaw(raw json.RawMessage, seen map[string]bool, out *[]string) {
	if len(*out) >= maxPullRequestURLsPerEvent {
		return
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return
		}
		trimmed := strings.TrimSpace(s)
		if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Valid([]byte(trimmed)) {
			collectPullRequestURLsFromRaw(json.RawMessage(trimmed), seen, out)
			return
		}
		appendPullRequestURLs(s, seen, out)
	case '{':
		var obj payload
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return
		}
		if payloadIndicatesToolError(obj) {
			return
		}
		// Prefer the canonical URL of a structured PR response (e.g. GitHub MCP
		// `create_pull_request`) so URLs referenced in sibling fields like `body` are ignored.
		for _, key := range prCanonicalURLKeys {
			if s := obj.str(key); s != "" {
				if norm, ok := normalizePullRequestURL(s); ok {
					if !seen[norm] {
						seen[norm] = true
						*out = append(*out, norm)
					}
					return
				}
			}
		}
		if stdout, ok := obj["stdout"]; ok {
			collectPullRequestURLsFromRaw(stdout, seen, out)
			return
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			if prOutputIgnoredKeys[strings.ToLower(k)] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			collectPullRequestURLsFromRaw(obj[k], seen, out)
			if len(*out) >= maxPullRequestURLsPerEvent {
				return
			}
		}
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return
		}
		for _, item := range items {
			collectPullRequestURLsFromRaw(item, seen, out)
			if len(*out) >= maxPullRequestURLsPerEvent {
				return
			}
		}
	}
}

func payloadIndicatesToolError(obj payload) bool {
	for _, key := range []string{"isError", "is_error"} {
		if b, ok := obj.boolean(key); ok && b {
			return true
		}
	}
	for _, key := range []string{"exitCode", "exit_code"} {
		if n, ok := obj.number(key); ok && n != 0 {
			return true
		}
	}
	if errText := obj.text("error"); errText != "" {
		return true
	}
	return false
}

func appendPullRequestURLs(text string, seen map[string]bool, out *[]string) {
	if text == "" || len(*out) >= maxPullRequestURLsPerEvent {
		return
	}
	for _, raw := range prURLPattern.FindAllString(text, -1) {
		norm, ok := normalizePullRequestURL(raw)
		if !ok || seen[norm] {
			continue
		}
		seen[norm] = true
		*out = append(*out, norm)
		if len(*out) >= maxPullRequestURLsPerEvent {
			return
		}
	}
}

// normalizePullRequestURL normalizes a pull request URL to scheme://host/owner/repo/pull/number,
// matching the server's ParsePullRequestURL.
func normalizePullRequestURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] != "pull" {
		return "", false
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", false
	}
	return u.Scheme + "://" + strings.ToLower(u.Host) + "/" + parts[0] + "/" + parts[1] + "/pull/" + strconv.Itoa(number), true
}

// extractAntigravityTranscriptPRURLs scans Antigravity's transcript.jsonl backwards from EOF
// for step_index == stepIdx and extracts pull request URLs from its output fields (never its
// command arguments or stderr). Missing, unreadable, non-regular, or unflushed transcripts
// return nil so hook recording never fails or blocks.
func extractAntigravityTranscriptPRURLs(transcriptPath string, stepIdx int) []string {
	consecutiveBelow := 0
	var found []string

	scanJSONLBackwards(transcriptPath, func(line []byte) (stop bool) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			return false
		}
		idx, ok := extractJSONNonNegInt(line, antigravityStepIndexNeedle)
		if !ok {
			return false
		}
		if idx < stepIdx {
			consecutiveBelow++
			return consecutiveBelow >= antigravityLowerStepThreshold
		}
		consecutiveBelow = 0
		if idx != stepIdx {
			return false
		}
		var row payload
		if err := json.Unmarshal(line, &row); err != nil || row == nil {
			return false
		}
		if payloadIndicatesToolError(row) {
			return true
		}
		if stdout, ok := row["stdout"]; ok {
			found = extractPullRequestURLsFromRaw(stdout)
			return true
		}
		var urls []string
		seen := map[string]bool{}
		for _, key := range antigravityTranscriptOutputKeys {
			if raw, ok := row[key]; ok {
				collectPullRequestURLsFromRaw(raw, seen, &urls)
			}
		}
		if len(urls) > 0 {
			found = urls
			return true
		}
		return false
	})

	return found
}
