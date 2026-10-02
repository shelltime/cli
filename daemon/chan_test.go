// cli/daemon/chan_test.go
package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type chanTestSuite struct {
	suite.Suite
}

func (s *chanTestSuite) TestNewGoChannel() {
	pubSub := NewGoChannel(PubSubConfig{}, nil)
	assert.NotNil(s.T(), pubSub)
	assert.NotNil(s.T(), pubSub.subscribers)
	assert.NotNil(s.T(), pubSub.persistedMessages)
	assert.NotNil(s.T(), pubSub.closing)

	pubSub.Close()
}
func (s *chanTestSuite) TestSimplePublishSubscribe() {
	pubSub := NewGoChannel(PubSubConfig{
		OutputChannelBuffer: 10,
	}, nil)
	defer pubSub.Close()

	msg := message.NewMessage("1", []byte("test"))
	topic := "test-topic"

	// Create subscriber first
	messages, err := pubSub.Subscribe(context.Background(), topic)
	assert.NoError(s.T(), err)

	// Publish message
	err = pubSub.Publish(topic, msg)
	assert.NoError(s.T(), err)

	// Receive message
	received := <-messages
	assert.Equal(s.T(), msg.UUID, received.UUID)
	assert.Equal(s.T(), msg.Payload, received.Payload)
}

func (s *chanTestSuite) TestPersistentMessages() {
	pubSub := NewGoChannel(PubSubConfig{
		OutputChannelBuffer: 10,
		Persistent:          true,
	}, nil)
	defer pubSub.Close()

	msg := message.NewMessage("1", []byte("test"))
	topic := "test-topic"

	// Publish before subscribe
	err := pubSub.Publish(topic, msg)
	assert.NoError(s.T(), err)

	// Subscribe after publish
	messages, err := pubSub.Subscribe(context.Background(), topic)
	assert.NoError(s.T(), err)

	// Should receive persisted message
	received := <-messages
	assert.Equal(s.T(), msg.UUID, received.UUID)
	assert.Equal(s.T(), msg.Payload, received.Payload)
}

func (s *chanTestSuite) TestMultipleSubscribers() {
	pubSub := NewGoChannel(PubSubConfig{
		OutputChannelBuffer: 10,
	}, nil)
	defer pubSub.Close()

	msg := message.NewMessage("1", []byte("test"))
	topic := "test-topic"

	// Create two subscribers
	messages1, err := pubSub.Subscribe(context.Background(), topic)
	assert.NoError(s.T(), err)
	messages2, err := pubSub.Subscribe(context.Background(), topic)
	assert.NoError(s.T(), err)

	// Publish message
	err = pubSub.Publish(topic, msg)
	assert.NoError(s.T(), err)

	// Both subscribers should receive the message
	received1 := <-messages1
	received2 := <-messages2
	assert.Equal(s.T(), msg.UUID, received1.UUID)
	assert.Equal(s.T(), msg.UUID, received2.UUID)
}

func (s *chanTestSuite) TestSubscriberContextCancellation() {
	pubSub := NewGoChannel(PubSubConfig{
		OutputChannelBuffer: 10,
	}, nil)
	defer pubSub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	topic := "test-topic"

	_, err := pubSub.Subscribe(ctx, topic)
	assert.NoError(s.T(), err)

	// Cancel context
	cancel()

	// Wait a bit for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify subscriber was removed
	pubSub.subscribersLock.RLock()
	assert.Empty(s.T(), pubSub.subscribers[topic])
	pubSub.subscribersLock.RUnlock()
}

func (s *chanTestSuite) TestCloseWithActiveSubscribers() {
	pubSub := NewGoChannel(PubSubConfig{
		OutputChannelBuffer: 10,
	}, nil)

	topic := "test-topic"
	_, err := pubSub.Subscribe(context.Background(), topic)
	assert.NoError(s.T(), err)

	err = pubSub.Close()
	assert.NoError(s.T(), err)

	// Verify closed state
	assert.True(s.T(), pubSub.isClosed())
	assert.Nil(s.T(), pubSub.persistedMessages)
}

func (s *chanTestSuite) TestPublishToClosedChannel() {
	pubSub := NewGoChannel(PubSubConfig{}, nil)
	pubSub.Close()

	msg := message.NewMessage("1", []byte("test"))
	err := pubSub.Publish("topic", msg)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "Pub/Sub closed")
}

func (s *chanTestSuite) TestSubscribeToClosedChannel() {
	pubSub := NewGoChannel(PubSubConfig{}, nil)
	pubSub.Close()

	_, err := pubSub.Subscribe(context.Background(), "topic")
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "Pub/Sub closed")
}

func TestChanTestSuite(t *testing.T) {
	suite.Run(t, new(chanTestSuite))
}
