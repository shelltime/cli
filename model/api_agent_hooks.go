package model

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/malamtime/cli/agenthook"
)

const (
	// AgentHookMaxEventsPerRequest caps the events of one POST /api/v1/agent-hooks.
	AgentHookMaxEventsPerRequest = 500
	// AgentHookMaxRequestBytes keeps a request's JSON body well under the server's 8 MB limit.
	AgentHookMaxRequestBytes = 4 << 20
)

// AgentHookRequest is the body of POST /api/v1/agent-hooks.
type AgentHookRequest struct {
	Host   string            `json:"host"`
	Events []agenthook.Event `json:"events"`
}

// AgentHookResponse is the server's answer to POST /api/v1/agent-hooks.
type AgentHookResponse struct {
	Success         bool `json:"success"`
	EventsProcessed int  `json:"eventsProcessed"`
	EventsSkipped   int  `json:"eventsSkipped"`
}

// SendAgentHookEvents posts Cursor and Antigravity hook events to the server, split into
// requests of at most AgentHookMaxEventsPerRequest events and AgentHookMaxRequestBytes. It
// stops at the first request that fails and returns its error; the server ignores event ids
// it already has, so sending the whole batch again is safe.
func SendAgentHookEvents(ctx context.Context, cfg ShellTimeConfig, events []agenthook.Event) error {
	ctx, span := modelTracer.Start(ctx, "agent_hooks.send")
	defer span.End()

	host, _ := os.Hostname()
	if host == "" && len(events) > 0 {
		host = events[0].Hostname
	}
	endpoint := Endpoint{APIEndpoint: cfg.APIEndpoint, Token: cfg.Token}
	for _, chunk := range SplitAgentHookEvents(events, AgentHookMaxEventsPerRequest, AgentHookMaxRequestBytes) {
		var resp AgentHookResponse
		err := SendHTTPRequestJSON(HTTPRequestOptions[AgentHookRequest, AgentHookResponse]{
			Context:  ctx,
			Endpoint: endpoint,
			Method:   http.MethodPost,
			Path:     "/api/v1/agent-hooks",
			Payload:  AgentHookRequest{Host: host, Events: chunk},
			Response: &resp,
			Timeout:  30 * time.Second,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// SplitAgentHookEvents splits events, in order, into chunks of at most maxEvents events whose
// encoded size stays within maxBytes. An event larger than maxBytes goes alone in its chunk
// (its text fields are capped, so it stays far below the server's limit).
func SplitAgentHookEvents(events []agenthook.Event, maxEvents, maxBytes int) [][]agenthook.Event {
	if maxEvents <= 0 {
		maxEvents = AgentHookMaxEventsPerRequest
	}
	if maxBytes <= 0 {
		maxBytes = AgentHookMaxRequestBytes
	}
	// The envelope: {"host":"…","events":[…]}
	budget := maxBytes - 256

	var chunks [][]agenthook.Event
	var cur []agenthook.Event
	curBytes := 0
	for _, ev := range events {
		size := agentHookEventSize(ev) + 1
		if len(cur) > 0 && (len(cur) >= maxEvents || curBytes+size > budget) {
			chunks = append(chunks, cur)
			cur, curBytes = nil, 0
		}
		cur = append(cur, ev)
		curBytes += size
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

func agentHookEventSize(ev agenthook.Event) int {
	b, err := json.Marshal(ev)
	if err != nil {
		return 0
	}
	return len(b)
}
