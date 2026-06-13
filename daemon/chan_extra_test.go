package daemon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublish_BlockUntilSubscriberAck(t *testing.T) {
	pubSub := NewGoChannel(PubSubConfig{
		OutputChannelBuffer:            10,
		BlockPublishUntilSubscriberAck: true,
	}, nil)
	defer pubSub.Close()

	topic := "ack-topic"
	msgs, err := pubSub.Subscribe(context.Background(), topic)
	require.NoError(t, err)

	// Consumer acks as soon as it receives the message.
	go func() {
		m := <-msgs
		m.Ack()
	}()

	msg := message.NewMessage("ack-1", []byte("payload"))
	// Publish blocks in waitForAckFromSubscribers until the consumer acks.
	done := make(chan error, 1)
	go func() { done <- pubSub.Publish(topic, msg) }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Publish did not return after subscriber ack")
	}
}

func TestPublish_BlockUntilAck_UnblocksOnClose(t *testing.T) {
	pubSub := NewGoChannel(PubSubConfig{
		OutputChannelBuffer:            10,
		BlockPublishUntilSubscriberAck: true,
	}, nil)

	topic := "ack-close-topic"
	msgs, err := pubSub.Subscribe(context.Background(), topic)
	require.NoError(t, err)

	// Receive but never ack; closing the pubsub must unblock the waiter.
	go func() {
		<-msgs
		// no ack/nack
	}()

	msg := message.NewMessage("ack-2", []byte("payload"))
	done := make(chan error, 1)
	go func() { done <- pubSub.Publish(topic, msg) }()

	// Give the publish a moment to enter the wait, then close.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, pubSub.Close())

	select {
	case <-done:
		// Publish returned (unblocked by closing).
	case <-time.After(2 * time.Second):
		t.Fatal("Publish did not unblock on Close")
	}
}

func TestSubscriber_NackTriggersRedelivery(t *testing.T) {
	pubSub := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer pubSub.Close()

	topic := "nack-topic"
	msgs, err := pubSub.Subscribe(context.Background(), topic)
	require.NoError(t, err)

	var deliveries atomic.Int32
	// Nack the first delivery, ack the redelivered copy. This drives the
	// retry/backoff branch in sendMessageToSubscriber.
	go func() {
		for m := range msgs {
			n := deliveries.Add(1)
			if n == 1 {
				m.Nack()
			} else {
				m.Ack()
				return
			}
		}
	}()

	msg := message.NewMessage("nack-1", []byte("payload"))
	require.NoError(t, pubSub.Publish(topic, msg))

	require.Eventually(t, func() bool {
		return deliveries.Load() >= 2
	}, 3*time.Second, 20*time.Millisecond, "message should be redelivered after nack")
}

func TestSubscriber_NackExceedsMaxRetriesDropsMessage(t *testing.T) {
	pubSub := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer pubSub.Close()

	topic := "nack-drop-topic"
	msgs, err := pubSub.Subscribe(context.Background(), topic)
	require.NoError(t, err)

	var deliveries atomic.Int32
	// Always nack: after maxRetries (3) the message is dropped and no further
	// redelivery occurs (total deliveries == 1 + maxRetries = 4).
	go func() {
		for m := range msgs {
			deliveries.Add(1)
			m.Nack()
		}
	}()

	msg := message.NewMessage("nack-drop", []byte("payload"))
	require.NoError(t, pubSub.Publish(topic, msg))

	// Backoff is 100ms,200ms,400ms; total ~700ms before drop. Wait it out and
	// confirm the delivery count settles at 4 (initial + 3 retries).
	require.Eventually(t, func() bool {
		return deliveries.Load() == 4
	}, 4*time.Second, 20*time.Millisecond)

	// Stays at 4 (no further redelivery after drop).
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(4), deliveries.Load())
}
