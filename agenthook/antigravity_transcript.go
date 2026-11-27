package agenthook

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxAntigravityTranscriptBytes bounds how far back from the end of transcript.jsonl we
// scan during PostInvocation so long conversations never slow down the hook.
const maxAntigravityTranscriptBytes = 4 << 20

// applyAntigravityTranscriptUsage reads the current invocation's completed model step from
// Antigravity's transcript.jsonl (matching initialNumSteps to step_index) and fills token
// counts onto ev. Works across Antigravity App, IDE, and agy CLI. Missing, unreadable, or
// unmatched transcripts are silently ignored so hook recording never fails.
func applyAntigravityTranscriptUsage(ev *Event, transcriptPath string, p payload) {
	stepIndex, ok := p.nonNegInt("initialNumSteps")
	if !ok || transcriptPath == "" {
		return
	}
	path := expandHomePath(transcriptPath)
	if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return
	}
	row, found := readAntigravityStep(path, stepIndex)
	if !found {
		return
	}
	input, hasInput := row.nonNegInt("input_tokens")
	output, hasOutput := row.nonNegInt("output_tokens")
	if !hasInput || !hasOutput {
		return
	}

	// Antigravity's input_tokens excludes cache reads; output_tokens already includes thinking.
	ev.InputTokens = intPtr(input)
	ev.OutputTokens = intPtr(output)
	if cacheRead, ok := row.nonNegInt("cache_read_tokens"); ok {
		ev.CacheReadTokens = intPtr(cacheRead)
	}
	if cacheWrite, ok := firstNonNegInt(row, "cache_creation_input_tokens", "cache_write_tokens"); ok {
		ev.CacheCreationTokens = intPtr(cacheWrite)
	}
	if thinking, ok := firstNonNegInt(row, "thinking_tokens", "reasoning_tokens"); ok {
		ev.ReasoningTokens = intPtr(thinking)
	}
	if ev.Model == "" {
		if m := row.str("model_name"); m != "" {
			ev.Model = m
		} else if m := row.str("modelName"); m != "" {
			ev.Model = m
		}
	}
}

func readAntigravityStep(path string, stepIndex int) (payload, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() || info.Size() == 0 {
		return nil, false
	}

	offset := info.Size() - maxAntigravityTranscriptBytes
	if offset < 0 {
		offset = 0
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, false
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, maxAntigravityTranscriptBytes))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	if offset > 0 {
		// Discard the partial first line when seeking into the middle of the file.
		if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
			data = data[idx+1:]
		} else {
			return nil, false
		}
	}

	var matched payload
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var row payload
		if err := json.Unmarshal(line, &row); err != nil || row == nil {
			continue
		}
		idx, ok := row.nonNegInt("step_index")
		if !ok || idx != stepIndex {
			continue
		}
		if !isCompletedAntigravityModelStep(row) {
			continue
		}
		if _, hasIn := row.nonNegInt("input_tokens"); !hasIn {
			continue
		}
		if _, hasOut := row.nonNegInt("output_tokens"); !hasOut {
			continue
		}
		matched = row
	}
	return matched, matched != nil
}

func isCompletedAntigravityModelStep(row payload) bool {
	if status := row.str("status"); status != "" && !strings.EqualFold(status, "DONE") {
		return false
	}
	source := row.str("source")
	stepType := row.str("step_type")
	if source != "" {
		return strings.EqualFold(source, "MODEL")
	}
	if stepType != "" {
		return strings.EqualFold(stepType, "PLANNER_RESPONSE")
	}
	return true
}

func expandHomePath(p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
