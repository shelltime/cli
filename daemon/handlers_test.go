// cli/daemon/handlers_test.go
package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/vmihailenco/msgpack/v5"
)

type handlersTestSuite struct {
	suite.Suite
}

func (s *handlersTestSuite) SetupSuite() {
	mockedST := model.NewMockConfigService(s.T())

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	mockedConfig := model.ShellTimeConfig{
		APIEndpoint: ts.URL,
	}
	mockedST.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)
	model.UserShellTimeConfig = mockedConfig
	Init(mockedST, "")
}

func (s *handlersTestSuite) TestSocketTopicProcessorValidSync() {
	msgChan := make(chan *message.Message)

	socketMsg := SocketMessage{
		Type: SocketMessageTypeSync,
		Payload: model.PostTrackArgs{
			CursorID: 9999,
			Data:     []model.TrackingData{},
			Meta: model.TrackingMetaData{
				OS:    "windows",
				Shell: "cmd",
			},
		},
	}
	payload, err := msgpack.Marshal(socketMsg)
	assert.NoError(s.T(), err)

	msg := message.NewMessage("test-uuid", payload)

	go SocketTopicProccessor(msgChan)

	msgChan <- msg

	time.Sleep(100 * time.Millisecond)

	close(msgChan)
}

func (s *handlersTestSuite) TestSocketTopicProcessorInvalidFormat() {
	msgChan := make(chan *message.Message)

	msg := message.NewMessage("test-uuid", []byte("invalid"))

	go SocketTopicProccessor(msgChan)

	msgChan <- msg

	time.Sleep(100 * time.Millisecond)

	close(msgChan)
}

func (s *handlersTestSuite) TestSocketTopicProcessorNonSync() {
	msgChan := make(chan *message.Message)

	socketMsg := SocketMessage{
		Type: SocketMessageTypeSync,
		Payload: model.PostTrackArgs{
			CursorID: 12345,
			Data:     []model.TrackingData{},
			Meta: model.TrackingMetaData{
				OS:    "linux",
				Shell: "bash",
			},
		},
	}
	payload, err := msgpack.Marshal(socketMsg)
	assert.NoError(s.T(), err)

	msg := message.NewMessage("test-uuid", payload)

	go SocketTopicProccessor(msgChan)

	msgChan <- msg

	time.Sleep(100 * time.Millisecond)

	close(msgChan)
}

func (s *handlersTestSuite) TestSocketTopicProcessorInvalidPayload() {
	msgChan := make(chan *message.Message)

	socketMsg := SocketMessage{
		Type:    "sync",
		Payload: []byte(`invalid json`),
	}
	payload, err := msgpack.Marshal(socketMsg)
	assert.NoError(s.T(), err)

	msg := message.NewMessage("test-uuid", payload)

	go SocketTopicProccessor(msgChan)

	msgChan <- msg

	time.Sleep(100 * time.Millisecond)

	close(msgChan)
}

func (s *handlersTestSuite) TestSocketTopicProcessorMultipleMessages() {
	msgChan := make(chan *message.Message, 2)

	socketMsg1 := SocketMessage{
		Type: SocketMessageTypeSync,
		Payload: model.PostTrackArgs{
			CursorID: 2222,
			Data:     []model.TrackingData{},
			Meta: model.TrackingMetaData{
				OS:    "mac",
				Shell: "fish",
			},
		},
	}
	payload1, err := msgpack.Marshal(socketMsg1)
	assert.NoError(s.T(), err)

	socketMsg2 := SocketMessage{
		Type: SocketMessageTypeSync,
		Payload: model.PostTrackArgs{
			CursorID: 111111,
			Data:     []model.TrackingData{},
			Meta: model.TrackingMetaData{
				OS:    "mac",
				Shell: "fish",
			},
		},
	}
	payload2, err := msgpack.Marshal(socketMsg2)
	assert.NoError(s.T(), err)

	msg1 := message.NewMessage("test-uuid-1", payload1)
	msg2 := message.NewMessage("test-uuid-2", payload2)

	go SocketTopicProccessor(msgChan)

	msgChan <- msg1
	msgChan <- msg2

	time.Sleep(100 * time.Millisecond)

	close(msgChan)
}

func TestHandlersTestSuite(t *testing.T) {
	suite.Run(t, new(handlersTestSuite))
}
