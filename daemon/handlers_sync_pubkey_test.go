package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestSendTrackArgsToServer_PublicKeyFetchErrorFailsClosed is a regression test
// for a nil-pointer panic: when encryption is enabled but GetOpenTokenPublicKey
// fails, sendTrackArgsToServer must fail closed (return the error) instead of
// dereferencing the nil public-key result or silently sending the data
// unencrypted against the user's configured intent.
func TestSendTrackArgsToServer_PublicKeyFetchErrorFailsClosed(t *testing.T) {
	var syncHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/opentoken/publickey") {
			// Make the public-key fetch fail -> GetOpenTokenPublicKey returns (nil, err).
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		syncHit = true
		w.WriteHeader(http.StatusNoContent)
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

	prevCB := syncCircuitBreaker
	syncCircuitBreaker = nil
	t.Cleanup(func() { syncCircuitBreaker = prevCB })

	msg := model.PostTrackArgs{
		CursorID: 1234567890,
		Data:     []model.TrackingData{{Command: "super-secret-command", Result: 0}},
		Meta:     model.TrackingMetaData{OS: "linux", Shell: "bash"},
	}

	// Must return an error without panicking, and must NOT reach the sync
	// endpoint (no unencrypted send).
	require.Error(t, sendTrackArgsToServer(context.Background(), msg))
	require.False(t, syncHit, "must not send data when the encryption public key cannot be fetched")
}
