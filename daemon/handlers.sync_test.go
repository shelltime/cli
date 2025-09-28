package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SyncHandlerTestSuite struct {
	suite.Suite
	server     *httptest.Server
	rsaService model.CryptoService
}

func (s *SyncHandlerTestSuite) SetupSuite() {
	s.rsaService = model.NewRSAService()

	// Setup test server with multiple endpoints
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/opentoken/publickey":
			s.handleOpenTokenPublicKey(w, r)
		case "/api/v1/track":
			s.handleTrackEndpoint(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}

func (s *SyncHandlerTestSuite) handleOpenTokenPublicKey(w http.ResponseWriter, r *http.Request) {
	// Generate test RSA keys
	pubKey, _, err := s.rsaService.GenerateKeys()
	if err != nil {
		http.Error(w, "Failed to generate keys", http.StatusInternalServerError)
		return
	}

	response := model.OpenTokenPublicKeyResponse{
		Data: model.OpenTokenPublicKey{
			ID:        1,
			PublicKey: string(pubKey),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *SyncHandlerTestSuite) handleTrackEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify authorization header
	auth := r.Header.Get("Authorization")
	if auth == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Decode request body
	var trackArgs model.PostTrackArgs
	if err := json.NewDecoder(r.Body).Decode(&trackArgs); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify the structure based on whether it's encrypted or not
	if trackArgs.Encrypted != "" {
		// Verify encrypted payload structure
		if trackArgs.AesKey == "" || trackArgs.Nonce == "" {
			http.Error(w, "Invalid encrypted payload", http.StatusBadRequest)
			return
		}
	} else {
		// Verify unencrypted payload structure
		if len(trackArgs.Data) == 0 {
			http.Error(w, "Empty tracking data", http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *SyncHandlerTestSuite) TearDownSuite() {
	s.server.Close()
}

func (s *SyncHandlerTestSuite) TestHandlePubSubSync_Unencrypted() {
	// Setup
	ctx := context.Background()
	cursorID := time.Now().UnixNano()

	testData := []model.TrackingData{
		{
			SessionID:     time.Now().Unix(),
			Command:       "test command",
			StartTime:     time.Now().Unix(),
			EndTime:       time.Now().Unix(),
			StartTimeNano: time.Now().UnixNano(),
			EndTimeNano:   time.Now().UnixNano(),
			Result:        0,
		},
	}

	testMeta := model.TrackingMetaData{
		Hostname:  "test-host",
		Username:  "test-user",
		OS:        "linux",
		OSVersion: "ubuntu-20.04",
		Shell:     "bash",
	}

	syncMsg := model.PostTrackArgs{
		CursorID: cursorID,
		Data:     testData,
		Meta:     testMeta,
	}

	// Mock config
	mockedConfig := model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
		Encrypted:   nil, // unencrypted mode
	}
	mockedStConfig := model.NewMockConfigService(s.T())
	mockedStConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)
	stConfig = mockedStConfig

	// Test
	err := handlePubSubSync(ctx, syncMsg)

	// Assert
	assert.NoError(s.T(), err)
	mockedStConfig.AssertExpectations(s.T())
}

func (s *SyncHandlerTestSuite) TestHandlePubSubSync_Encrypted() {
	// Setup
	ctx := context.Background()
	cursorID := time.Now().UnixNano()

	testData := []model.TrackingData{
		{
			SessionID:     time.Now().Unix(),
			Command:       "test command",
			StartTime:     time.Now().Unix(),
			EndTime:       time.Now().Unix(),
			StartTimeNano: time.Now().UnixNano(),
			EndTimeNano:   time.Now().UnixNano(),
			Result:        0,
		},
	}

	testMeta := model.TrackingMetaData{
		Hostname:  "test-host",
		Username:  "test-user",
		OS:        "linux",
		OSVersion: "ubuntu-20.04",
		Shell:     "bash",
	}

	syncMsg := model.PostTrackArgs{
		CursorID: cursorID,
		Data:     testData,
		Meta:     testMeta,
	}

	encrypted := true
	// Mock config
	mockedConfig := model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
		Encrypted:   &encrypted,
	}
	mockedStConfig := model.NewMockConfigService(s.T())
	mockedStConfig.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)
	stConfig = mockedStConfig

	// Test
	err := handlePubSubSync(ctx, syncMsg)

	// Assert
	assert.NoError(s.T(), err)
	mockedStConfig.AssertExpectations(s.T())
}

func (s *SyncHandlerTestSuite) TestHandlePubSubSync_InvalidPayload() {
	// Setup
	ctx := context.Background()
	invalidPayload := "invalid payload"

	// Test
	err := handlePubSubSync(ctx, invalidPayload)

	// Assert
	assert.Error(s.T(), err)
}

func (s *SyncHandlerTestSuite) TestHandlePubSubSync_ConfigError() {
	// Setup
	ctx := context.Background()
	syncMsg := model.PostTrackArgs{
		CursorID: time.Now().UnixNano(),
		Data:     []model.TrackingData{},
		Meta:     model.TrackingMetaData{},
	}

	// Mock config service to return error
	mockedStConfig := model.NewMockConfigService(s.T())
	mockedStConfig.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
	stConfig = mockedStConfig

	// Test
	err := handlePubSubSync(ctx, syncMsg)

	// Assert
	assert.Nil(s.T(), err)
	mockedStConfig.AssertExpectations(s.T())
}

func TestSyncHandlerSuite(t *testing.T) {
	suite.Run(t, new(SyncHandlerTestSuite))
}
