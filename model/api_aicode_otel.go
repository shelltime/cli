package model

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// SendAICodeOtelData sends OTEL data to the backend immediately
// POST /api/v1/cc/otel
func SendAICodeOtelData(ctx context.Context, req *AICodeOtelRequest, endpoint Endpoint) (*AICodeOtelResponse, error) {
	ctx, span := modelTracer.Start(ctx, "aicode_otel.send")
	defer span.End()

	var resp AICodeOtelResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*AICodeOtelRequest, AICodeOtelResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/otel",
		Payload:  req,
		Response: &resp,
		Timeout:  30 * time.Second,
	})

	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// SplitAICodeOtelRequest splits req into requests whose JSON bodies stay within maxBytes, keeping
// events, metrics and spans in their original order. Like PackBackfillBatches it estimates each item by
// encoding it on its own; an item larger than the budget goes alone in its request.
func SplitAICodeOtelRequest(req *AICodeOtelRequest, maxBytes int) []*AICodeOtelRequest {
	if req == nil {
		return nil
	}
	envelope := AICodeOtelRequest{Host: req.Host, Project: req.Project, Source: req.Source}
	// `,"events":[]`, `,"metrics":[]` and `,"spans":[]` are not part of the empty envelope's encoding.
	budget := maxBytes - aiCodeOtelJSONSize(envelope) - len(`,"events":[],"metrics":[],"spans":[]`)

	var out []*AICodeOtelRequest
	cur := envelope
	curBytes := 0
	empty := func() bool {
		return len(cur.Events) == 0 && len(cur.Metrics) == 0 && len(cur.Spans) == 0
	}
	flush := func() {
		if !empty() {
			chunk := cur
			out = append(out, &chunk)
		}
		cur = envelope
		curBytes = 0
	}
	fits := func(size int) bool {
		return empty() || curBytes+size <= budget
	}

	for _, e := range req.Events {
		size := aiCodeOtelJSONSize(e) + 1
		if !fits(size) {
			flush()
		}
		cur.Events = append(cur.Events, e)
		curBytes += size
	}
	for _, m := range req.Metrics {
		size := aiCodeOtelJSONSize(m) + 1
		if !fits(size) {
			flush()
		}
		cur.Metrics = append(cur.Metrics, m)
		curBytes += size
	}
	for _, sp := range req.Spans {
		size := aiCodeOtelJSONSize(sp) + 1
		if !fits(size) {
			flush()
		}
		cur.Spans = append(cur.Spans, sp)
		curBytes += size
	}
	flush()
	return out
}

func aiCodeOtelJSONSize(v any) int {
	buf, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(buf)
}
