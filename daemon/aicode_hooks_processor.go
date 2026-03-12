package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
)

const AICodeHooksTopic = "aicode_hooks"

// AICodeHooksProcessor handles batching and forwarding hook events to the backend
type AICodeHooksProcessor struct {
	config   model.ShellTimeConfig
	endpoint model.Endpoint
	hostname string
	debug    bool
	events   []model.AICodeHooksEventData
	mu       sync.Mutex
	timer    *time.Timer
	stopChan chan struct{}

	batchSize     int
	batchInterval time.Duration
}

// NewAICodeHooksProcessor creates a new AICodeHooks processor
func NewAICodeHooksProcessor(config model.ShellTimeConfig) *AICodeHooksProcessor {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	debug := config.AICodeHooks != nil && config.AICodeHooks.Debug != nil && *config.AICodeHooks.Debug

	batchSize := 50
	batchInterval := 5 * time.Second
	if config.AICodeHooks != nil {
		if config.AICodeHooks.BatchSize > 0 {
			batchSize = config.AICodeHooks.BatchSize
		}
		if config.AICodeHooks.BatchIntervalSeconds > 0 {
			batchInterval = time.Duration(config.AICodeHooks.BatchIntervalSeconds) * time.Second
		}
	}

	return &AICodeHooksProcessor{
		config: config,
		endpoint: model.Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		hostname:      hostname,
		debug:         debug,
		events:        make([]model.AICodeHooksEventData, 0),
		stopChan:      make(chan struct{}),
		batchSize:     batchSize,
		batchInterval: batchInterval,
	}
}

// Start subscribes to the AICodeHooks topic and begins processing messages
func (p *AICodeHooksProcessor) Start(goChannel *GoChannel) {
	msgChan, err := goChannel.Subscribe(context.Background(), AICodeHooksTopic)
	if err != nil {
		slog.Error("AICodeHooks: Failed to subscribe to topic", slog.Any("err", err))
		return
	}

	slog.Info("AICodeHooks: Processor started", slog.Int("batchSize", p.batchSize), slog.Duration("batchInterval", p.batchInterval))

	go func() {
		for {
			select {
			case msg, ok := <-msgChan:
				if !ok {
					slog.Info("AICodeHooks: Message channel closed")
					return
				}
				p.processMessage(msg)
				msg.Ack()
			case <-p.stopChan:
				slog.Info("AICodeHooks: Processor stopping")
				return
			}
		}
	}()
}

// Stop flushes remaining events and stops the processor
func (p *AICodeHooksProcessor) Stop() {
	close(p.stopChan)

	p.mu.Lock()
	if p.timer != nil {
		p.timer.Stop()
	}
	p.mu.Unlock()

	// Flush remaining events
	p.flush()
	slog.Info("AICodeHooks: Processor stopped")
}

// processMessage parses a socket message and adds the event to the batch
func (p *AICodeHooksProcessor) processMessage(msg *message.Message) {
	var socketMsg SocketMessage
	if err := json.Unmarshal(msg.Payload, &socketMsg); err != nil {
		slog.Error("AICodeHooks: Failed to parse socket message", slog.Any("err", err))
		return
	}

	// Extract the event data from the socket message payload
	payloadBytes, err := json.Marshal(socketMsg.Payload)
	if err != nil {
		slog.Error("AICodeHooks: Failed to marshal payload", slog.Any("err", err))
		return
	}

	var eventData model.AICodeHooksEventData
	if err := json.Unmarshal(payloadBytes, &eventData); err != nil {
		slog.Error("AICodeHooks: Failed to parse event data", slog.Any("err", err))
		return
	}

	if p.debug {
		p.writeDebugFile("aicode-hooks-debug-events.txt", eventData)
	}

	slog.Debug("AICodeHooks: Received event",
		slog.String("eventId", eventData.EventID),
		slog.String("hookEventName", eventData.HookEventName),
		slog.String("clientType", eventData.ClientType),
	)

	p.mu.Lock()
	p.events = append(p.events, eventData)

	// Reset the flush timer
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(p.batchInterval, func() {
		p.flush()
	})

	// Flush immediately if batch is full
	if len(p.events) >= p.batchSize {
		events := make([]model.AICodeHooksEventData, len(p.events))
		copy(events, p.events)
		p.events = p.events[:0]
		p.timer.Stop()
		p.mu.Unlock()
		p.sendEvents(events)
		return
	}
	p.mu.Unlock()
}

// flush sends all pending events to the backend
func (p *AICodeHooksProcessor) flush() {
	p.mu.Lock()
	if len(p.events) == 0 {
		p.mu.Unlock()
		return
	}
	events := make([]model.AICodeHooksEventData, len(p.events))
	copy(events, p.events)
	p.events = p.events[:0]
	p.mu.Unlock()

	p.sendEvents(events)
}

// sendEvents sends a batch of events to the backend
func (p *AICodeHooksProcessor) sendEvents(events []model.AICodeHooksEventData) {
	if len(events) == 0 {
		return
	}

	req := &model.AICodeHooksRequest{
		Host:   p.hostname,
		Events: events,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := model.SendAICodeHooksData(ctx, req, p.endpoint)
	if err != nil {
		slog.Error("AICodeHooks: Failed to send events to backend", slog.Any("err", err), slog.Int("eventCount", len(events)))
		return
	}

	slog.Debug("AICodeHooks: Events sent to backend",
		slog.Int("eventsProcessed", resp.EventsProcessed),
		slog.Bool("success", resp.Success),
	)
}

// writeDebugFile appends JSON-formatted data to a debug file
func (p *AICodeHooksProcessor) writeDebugFile(filename string, data interface{}) {
	debugDir := filepath.Join(os.TempDir(), "shelltime")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		slog.Error("AICodeHooks: Failed to create debug directory", "error", err)
		return
	}

	filePath := filepath.Join(debugDir, filename)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("AICodeHooks: Failed to open debug file", "error", err, "path", filePath)
		return
	}
	defer f.Close()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		slog.Error("AICodeHooks: Failed to marshal debug data", "error", err)
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	if _, err := f.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", timestamp, jsonData)); err != nil {
		slog.Error("AICodeHooks: Failed to write debug data", "error", err)
	}
	slog.Debug("AICodeHooks: Wrote debug data", "path", filePath)
}
