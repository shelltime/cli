package daemon

import (
	"context"
	"encoding/json"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
)

// DaemonCircuitBreaker defines the interface for daemon-specific circuit breaker operations
type DaemonCircuitBreaker interface {
	IsOpen() bool
	RecordSuccess()
	RecordFailure()
	SaveForRetry(ctx context.Context, payload interface{}) error
}

// Global instance
var syncCircuitBreaker DaemonCircuitBreaker

// SyncCircuitBreakerWrapper wraps model.CircuitBreakerService with daemon-specific logic
type SyncCircuitBreakerWrapper struct {
	*model.CircuitBreakerService
}

// NewSyncCircuitBreakerService creates a new daemon-specific circuit breaker service
func NewSyncCircuitBreakerService(publisher message.Publisher) *SyncCircuitBreakerWrapper {
	republishFn := func(data []byte) error {
		msg := message.NewMessage(watermill.NewUUID(), data)
		return publisher.Publish(PubSubTopic, msg)
	}

	svc := model.NewCircuitBreakerService(model.CircuitBreakerConfig{}, republishFn)
	wrapper := &SyncCircuitBreakerWrapper{
		CircuitBreakerService: svc,
	}
	syncCircuitBreaker = wrapper
	return wrapper
}

// SaveForRetry wraps payload in SocketMessage before saving
func (w *SyncCircuitBreakerWrapper) SaveForRetry(ctx context.Context, payload interface{}) error {
	socketMsg := SocketMessage{
		Type:    SocketMessageTypeSync,
		Payload: payload,
	}
	jsonData, err := json.Marshal(socketMsg)
	if err != nil {
		return err
	}
	return w.CircuitBreakerService.SaveForRetry(ctx, jsonData)
}
