package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// x3SwapStConfig swaps stConfig (and restores it) for a daemon handler test.
func x3SwapStConfig(t *testing.T, cs model.ConfigService) {
	t.Helper()
	prev := stConfig
	stConfig = cs
	t.Cleanup(func() { stConfig = prev })
}

// TestX3SendTrackArgsToServer_EncryptedHappyPath drives the encryption branch of
// sendTrackArgsToServer end-to-end. A real RSA public key (generated locally) is
// served by the publickey endpoint, so the AES key is RSA-wrapped, the payload
// is AES-GCM encrypted, and the final POST carries the encrypted envelope (not
// the plaintext command).
func TestX3SendTrackArgsToServer_EncryptedHappyPath(t *testing.T) {
	// Generate a valid PEM-encoded RSA public key the model crypto can parse.
	pub, _, err := model.NewRSAService().GenerateKeys()
	require.NoError(t, err)

	var sentBody string
	var publicKeyHit, syncHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/opentoken/publickey"):
			publicKeyHit = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(model.OpenTokenPublicKeyResponse{
				Data: model.OpenTokenPublicKey{ID: 1, PublicKey: string(pub)},
			})
		default:
			syncHit = true
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			sentBody = string(b)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)

	enabled := true
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		Encrypted:   &enabled,
	}, nil)
	x3SwapStConfig(t, mc)

	// No circuit breaker for this test.
	prevCB := syncCircuitBreaker
	syncCircuitBreaker = nil
	t.Cleanup(func() { syncCircuitBreaker = prevCB })

	msg := model.PostTrackArgs{
		CursorID: 1234567890,
		Data:     []model.TrackingData{{Command: "super-secret-command", Result: 0}},
		Meta:     model.TrackingMetaData{OS: "linux", Shell: "bash"},
	}
	require.NoError(t, sendTrackArgsToServer(context.Background(), msg))

	assert.True(t, publicKeyHit, "public key endpoint should be queried for encryption")
	assert.True(t, syncHit, "sync endpoint should receive the payload")
	assert.NotContains(t, sentBody, "super-secret-command", "plaintext command must not be sent when encrypted")
	assert.Contains(t, sentBody, "encrypted", "payload should carry the encrypted envelope")
}

// The public-key-fetch-failure branch is covered by
// TestSendTrackArgsToServer_PublicKeyFetchErrorFailsClosed in
// handlers_sync_pubkey_test.go (it previously panicked on a nil pointer; the
// handler now fails closed and returns the error).
