package daemon

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CCInfoHandlerTestSuite struct {
	suite.Suite
	socketPath string
	listener   net.Listener
}

func (s *CCInfoHandlerTestSuite) SetupTest() {
	// Create temp socket path
	s.socketPath = filepath.Join(os.TempDir(), "test-cc-info-handler.sock")
	os.Remove(s.socketPath) // Clean up any existing socket
}

func (s *CCInfoHandlerTestSuite) TearDownTest() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
}

func (s *CCInfoHandlerTestSuite) TestHandleCCInfo_DefaultsToToday() {
	config := &model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	// Create socket handler
	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer ch.Close()
	handler := NewSocketHandler(config, ch)

	// Create a pipe to simulate connection
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Send message without timeRange
	msg := SocketMessage{
		Type:    SocketMessageTypeCCInfo,
		Payload: map[string]interface{}{},
	}

	go func() {
		handler.handleCCInfo(serverConn, msg)
	}()

	// Read response
	var response CCInfoResponse
	decoder := json.NewDecoder(clientConn)
	err := decoder.Decode(&response)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "today", response.TimeRange)
}

func (s *CCInfoHandlerTestSuite) TestHandleCCInfo_ParsesTimeRange() {
	config := &model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer ch.Close()
	handler := NewSocketHandler(config, ch)

	// Create a pipe
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Send message with week timeRange
	msg := SocketMessage{
		Type: SocketMessageTypeCCInfo,
		Payload: map[string]interface{}{
			"timeRange": "week",
		},
	}

	go func() {
		handler.handleCCInfo(serverConn, msg)
	}()

	var response CCInfoResponse
	decoder := json.NewDecoder(clientConn)
	err := decoder.Decode(&response)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "week", response.TimeRange)
}

func (s *CCInfoHandlerTestSuite) TestHandleCCInfo_ReturnsCorrectResponseStructure() {
	config := &model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer ch.Close()
	handler := NewSocketHandler(config, ch)

	// Pre-populate cache
	handler.ccInfoTimer.mu.Lock()
	handler.ccInfoTimer.cache[CCInfoTimeRangeToday] = CCInfoCache{
		TotalCostUSD: 12.34,
		FetchedAt:    time.Now(),
	}
	handler.ccInfoTimer.mu.Unlock()

	// Create a pipe
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	msg := SocketMessage{
		Type: SocketMessageTypeCCInfo,
		Payload: map[string]interface{}{
			"timeRange": "today",
		},
	}

	go func() {
		handler.handleCCInfo(serverConn, msg)
	}()

	var response CCInfoResponse
	decoder := json.NewDecoder(clientConn)
	err := decoder.Decode(&response)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 12.34, response.TotalCostUSD)
	assert.Equal(s.T(), "today", response.TimeRange)
	assert.False(s.T(), response.CachedAt.IsZero())
}

func (s *CCInfoHandlerTestSuite) TestHandleCCInfo_NotifiesActivity() {
	config := &model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer ch.Close()
	handler := NewSocketHandler(config, ch)
	defer handler.ccInfoTimer.Stop()

	// Create a pipe
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	msg := SocketMessage{
		Type:    SocketMessageTypeCCInfo,
		Payload: map[string]interface{}{},
	}

	before := time.Now()

	go func() {
		handler.handleCCInfo(serverConn, msg)
	}()

	// Wait for response
	var response CCInfoResponse
	json.NewDecoder(clientConn).Decode(&response)

	after := time.Now()

	// Check lastActivity was updated
	handler.ccInfoTimer.mu.RLock()
	lastActivity := handler.ccInfoTimer.lastActivity
	handler.ccInfoTimer.mu.RUnlock()

	assert.True(s.T(), lastActivity.After(before) || lastActivity.Equal(before))
	assert.True(s.T(), lastActivity.Before(after) || lastActivity.Equal(after))
}

// Test RequestCCInfo client function

type CCInfoClientTestSuite struct {
	suite.Suite
	socketPath string
	listener   net.Listener
}

func (s *CCInfoClientTestSuite) SetupTest() {
	s.socketPath = filepath.Join(os.TempDir(), "test-cc-info-client.sock")
	os.Remove(s.socketPath)
}

func (s *CCInfoClientTestSuite) TearDownTest() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
}

func (s *CCInfoClientTestSuite) TestRequestCCInfo_Success() {
	// Start mock server
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	expectedResponse := CCInfoResponse{
		TotalCostUSD: 7.89,
		TimeRange:    "today",
		CachedAt:     time.Now(),
	}

	go func() {
		conn, _ := listener.Accept()
		defer conn.Close()

		// Read request
		var msg SocketMessage
		json.NewDecoder(conn).Decode(&msg)

		// Send response
		json.NewEncoder(conn).Encode(expectedResponse)
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	response, err := RequestCCInfo(s.socketPath, CCInfoTimeRangeToday, "", 1*time.Second)

	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), response)
	assert.Equal(s.T(), expectedResponse.TotalCostUSD, response.TotalCostUSD)
	assert.Equal(s.T(), expectedResponse.TimeRange, response.TimeRange)
}

func (s *CCInfoClientTestSuite) TestRequestCCInfo_Timeout() {
	// Start mock server that doesn't respond
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	go func() {
		conn, _ := listener.Accept()
		defer conn.Close()
		// Don't respond, let it timeout
		time.Sleep(1 * time.Second)
	}()

	time.Sleep(10 * time.Millisecond)

	response, err := RequestCCInfo(s.socketPath, CCInfoTimeRangeToday, "", 50*time.Millisecond)

	assert.Error(s.T(), err)
	assert.Nil(s.T(), response)
}

func (s *CCInfoClientTestSuite) TestRequestCCInfo_SocketNotFound() {
	response, err := RequestCCInfo("/nonexistent/socket.sock", CCInfoTimeRangeToday, "", 100*time.Millisecond)

	assert.Error(s.T(), err)
	assert.Nil(s.T(), response)
}

func (s *CCInfoClientTestSuite) TestRequestCCInfo_InvalidResponse() {
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	go func() {
		conn, _ := listener.Accept()
		defer conn.Close()

		// Read request
		var msg SocketMessage
		json.NewDecoder(conn).Decode(&msg)

		// Send invalid JSON
		conn.Write([]byte("not valid json"))
	}()

	time.Sleep(10 * time.Millisecond)

	response, err := RequestCCInfo(s.socketPath, CCInfoTimeRangeToday, "", 1*time.Second)

	assert.Error(s.T(), err)
	assert.Nil(s.T(), response)
}

func (s *CCInfoClientTestSuite) TestRequestCCInfo_SendsCorrectMessage() {
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	var receivedMsg SocketMessage
	go func() {
		conn, _ := listener.Accept()
		defer conn.Close()

		// Read and capture request
		json.NewDecoder(conn).Decode(&receivedMsg)

		// Send response
		json.NewEncoder(conn).Encode(CCInfoResponse{})
	}()

	time.Sleep(10 * time.Millisecond)

	RequestCCInfo(s.socketPath, CCInfoTimeRangeWeek, "", 1*time.Second)

	assert.Equal(s.T(), SocketMessageTypeCCInfo, receivedMsg.Type)

	// Check payload
	payload, ok := receivedMsg.Payload.(map[string]interface{})
	assert.True(s.T(), ok)

	// Decode the payload properly
	payloadBytes, _ := json.Marshal(payload)
	var req CCInfoRequest
	json.NewDecoder(bytes.NewReader(payloadBytes)).Decode(&req)
	assert.Equal(s.T(), CCInfoTimeRangeWeek, req.TimeRange)
}

func (s *CCInfoHandlerTestSuite) TestHandleCCInfo_IncludesRateLimitWhenCached() {
	config := &model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer ch.Close()
	handler := NewSocketHandler(config, ch)

	// Pre-populate rate limit cache
	handler.ccInfoTimer.rateLimitCache.mu.Lock()
	handler.ccInfoTimer.rateLimitCache.usage = &AnthropicRateLimitData{
		FiveHourUtilization: 0.45,
		SevenDayUtilization: 0.23,
	}
	handler.ccInfoTimer.rateLimitCache.fetchedAt = time.Now()
	handler.ccInfoTimer.rateLimitCache.mu.Unlock()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	msg := SocketMessage{
		Type: SocketMessageTypeCCInfo,
		Payload: map[string]interface{}{
			"timeRange": "today",
		},
	}

	go func() {
		handler.handleCCInfo(serverConn, msg)
	}()

	var response CCInfoResponse
	decoder := json.NewDecoder(clientConn)
	err := decoder.Decode(&response)

	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), response.FiveHourUtilization)
	assert.NotNil(s.T(), response.SevenDayUtilization)
	assert.Equal(s.T(), 0.45, *response.FiveHourUtilization)
	assert.Equal(s.T(), 0.23, *response.SevenDayUtilization)
}

func (s *CCInfoHandlerTestSuite) TestHandleCCInfo_OmitsRateLimitWhenNotCached() {
	config := &model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	defer ch.Close()
	handler := NewSocketHandler(config, ch)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	msg := SocketMessage{
		Type: SocketMessageTypeCCInfo,
		Payload: map[string]interface{}{
			"timeRange": "today",
		},
	}

	go func() {
		handler.handleCCInfo(serverConn, msg)
	}()

	var response CCInfoResponse
	decoder := json.NewDecoder(clientConn)
	err := decoder.Decode(&response)

	assert.NoError(s.T(), err)
	assert.Nil(s.T(), response.FiveHourUtilization)
	assert.Nil(s.T(), response.SevenDayUtilization)
}

func TestCCInfoHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(CCInfoHandlerTestSuite))
}

func TestCCInfoClientTestSuite(t *testing.T) {
	suite.Run(t, new(CCInfoClientTestSuite))
}
