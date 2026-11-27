package agenthook

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// antigravityTranscriptChunkBytes is read backwards from EOF one chunk at a time so
	// short invocations only inspect the tail of transcript.jsonl (within the 50 ms hook budget).
	antigravityTranscriptChunkBytes = 64 << 10
	// maxAntigravityTranscriptBytes bounds the maximum total bytes scanned from EOF.
	maxAntigravityTranscriptBytes = 4 << 20
)

var (
	antigravityStepIndexNeedle = []byte(`"step_index"`)
	antigravityTokensNeedle    = []byte(`"input_tokens"`)
)

type antigravityStepUsage struct {
	input         int
	output        int
	cacheRead     int
	hasCacheRead  bool
	cacheCreate   int
	hasCacheWrite bool
	reasoning     int
	hasReasoning  bool
	model         string
}

// applyAntigravityTranscriptUsage reads the current invocation's completed model steps from
// Antigravity's transcript.jsonl (all completed MODEL / PLANNER_RESPONSE steps with
// step_index >= initialNumSteps, deduplicated per step_index) and sums their token counts onto ev.
// Works across Antigravity App, IDE, and agy CLI. Missing, unreadable, non-regular, or unmatched
// transcripts are silently ignored so hook recording never fails.
func applyAntigravityTranscriptUsage(ev *Event, transcriptPath string, p payload) {
	startStep, ok := p.nonNegInt("initialNumSteps")
	if !ok || transcriptPath == "" {
		return
	}
	maxStep := -1
	if endStep, ok := firstNonNegInt(p, "finalNumSteps", "numSteps"); ok && endStep > startStep {
		maxStep = endStep - 1
	}

	path := expandHomePath(transcriptPath)
	if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return
	}
	usage, found := readAntigravityInvocationUsage(path, startStep, maxStep)
	if !found {
		return
	}

	// Antigravity's input_tokens excludes cache reads; output_tokens already includes thinking.
	ev.InputTokens = intPtr(usage.input)
	ev.OutputTokens = intPtr(usage.output)
	if usage.hasCacheRead {
		ev.CacheReadTokens = intPtr(usage.cacheRead)
	}
	if usage.hasCacheWrite {
		ev.CacheCreationTokens = intPtr(usage.cacheCreate)
	}
	if usage.hasReasoning {
		ev.ReasoningTokens = intPtr(usage.reasoning)
	}
	if ev.Model == "" && usage.model != "" {
		ev.Model = usage.model
	}
}

// readAntigravityInvocationUsage scans path backwards in bounded chunks from EOF, collecting
// the latest completed model row for each step_index >= startStep (and <= maxStep when maxStep >= 0),
// and stops as soon as a row with step_index < startStep is reached.
func readAntigravityInvocationUsage(path string, startStep, maxStep int) (antigravityStepUsage, bool) {
	f, err := os.Open(path)
	if err != nil {
		return antigravityStepUsage{}, false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return antigravityStepUsage{}, false
	}

	steps := map[int]antigravityStepUsage{}
	latestStep := -1
	latestModel := ""

	pos := info.Size()
	var scanned int64
	var carry []byte

	processLine := func(line []byte) (stop bool) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.Contains(line, antigravityStepIndexNeedle) {
			return false
		}
		// Fast check for non-usage lines that still carry step_index: we only need to stop
		// when step_index falls below startStep, so skip full unmarshal when a line has no
		// token fields unless we haven't checked its step_index yet.
		var row payload
		if err := json.Unmarshal(line, &row); err != nil || row == nil {
			return false
		}
		idx, ok := row.nonNegInt("step_index")
		if !ok {
			return false
		}
		if idx < startStep {
			return true
		}
		if maxStep >= 0 && idx > maxStep {
			return false
		}
		if !bytes.Contains(line, antigravityTokensNeedle) || !isCompletedAntigravityModelStep(row) {
			return false
		}
		// Scanning backwards: the first completed row seen for a given step_index is the newest.
		if _, exists := steps[idx]; exists {
			return false
		}
		input, hasInput := row.nonNegInt("input_tokens")
		output, hasOutput := row.nonNegInt("output_tokens")
		if !hasInput || !hasOutput {
			return false
		}
		u := antigravityStepUsage{
			input:  input,
			output: output,
		}
		if cacheRead, ok := row.nonNegInt("cache_read_tokens"); ok {
			u.cacheRead = cacheRead
			u.hasCacheRead = true
		}
		if cacheWrite, ok := firstNonNegInt(row, "cache_creation_input_tokens", "cache_write_tokens"); ok {
			u.cacheCreate = cacheWrite
			u.hasCacheWrite = true
		}
		if thinking, ok := firstNonNegInt(row, "thinking_tokens", "reasoning_tokens"); ok {
			u.reasoning = thinking
			u.hasReasoning = true
		}
		if m := row.str("model_name"); m != "" {
			u.model = m
		} else if m := row.str("modelName"); m != "" {
			u.model = m
		}
		steps[idx] = u
		if u.model != "" && idx > latestStep {
			latestStep = idx
			latestModel = u.model
		}
		return false
	}

	stop := false
	for pos > 0 && scanned < maxAntigravityTranscriptBytes && !stop {
		chunkSize := int64(antigravityTranscriptChunkBytes)
		if chunkSize > pos {
			chunkSize = pos
		}
		if remaining := int64(maxAntigravityTranscriptBytes) - scanned; chunkSize > remaining {
			chunkSize = remaining
		}
		pos -= chunkSize
		buf := make([]byte, chunkSize)
		if _, err := f.ReadAt(buf, pos); err != nil && err != io.EOF {
			break
		}
		scanned += chunkSize

		if len(carry) > 0 {
			buf = append(buf, carry...)
			carry = nil
		}

		// Split lines from the end of buf; keep the leading fragment in carry when pos > 0.
		end := len(buf)
		for {
			nl := bytes.LastIndexByte(buf[:end], '\n')
			if nl < 0 {
				if pos == 0 {
					stop = processLine(buf[:end])
				} else {
					carry = append([]byte(nil), buf[:end]...)
				}
				break
			}
			if processLine(buf[nl+1 : end]) {
				stop = true
				break
			}
			end = nl
		}
	}

	if len(steps) == 0 {
		return antigravityStepUsage{}, false
	}

	var total antigravityStepUsage
	total.model = latestModel
	for _, u := range steps {
		total.input += u.input
		total.output += u.output
		if u.hasCacheRead {
			total.cacheRead += u.cacheRead
			total.hasCacheRead = true
		}
		if u.hasCacheWrite {
			total.cacheCreate += u.cacheCreate
			total.hasCacheWrite = true
		}
		if u.hasReasoning {
			total.reasoning += u.reasoning
			total.hasReasoning = true
		}
	}
	return total, true
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
