package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collmetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewAICodeOtelServer(t *testing.T) {
	proc := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "t"})
	server := NewAICodeOtelServer(54027, proc)
	require.NotNil(t, server)
	assert.Equal(t, 54027, server.port)
	assert.Same(t, proc, server.processor)
}

func TestAICodeOtelServer_StartStopLifecycle(t *testing.T) {
	proc := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "t"})
	// Port 0 -> OS assigns an ephemeral free port.
	server := NewAICodeOtelServer(0, proc)

	require.NoError(t, server.Start())
	require.NotNil(t, server.listener)

	// Stop must complete promptly without hanging.
	done := make(chan struct{})
	go func() {
		server.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete in time")
	}
}

func TestAICodeOtelServer_StopBeforeStart(t *testing.T) {
	server := NewAICodeOtelServer(0, NewAICodeOtelProcessor(model.ShellTimeConfig{}))
	// grpcServer is nil; Stop must be a safe no-op.
	assert.NotPanics(t, server.Stop)
}

func TestAICodeOtelServer_ExportRoundTrip(t *testing.T) {
	// Backend HTTP server the processor forwards to. We use empty requests so
	// no metrics/events are produced and the backend is never actually hit, but
	// the gRPC Export handlers are exercised end-to-end.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proc := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "t", APIEndpoint: backend.URL})
	server := NewAICodeOtelServer(0, proc)
	require.NoError(t, server.Start())
	defer server.Stop()

	addr := server.listener.Addr().String()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	metricsClient := collmetricsv1.NewMetricsServiceClient(conn)
	mResp, err := metricsClient.Export(ctx, &collmetricsv1.ExportMetricsServiceRequest{})
	require.NoError(t, err)
	require.NotNil(t, mResp)

	logsClient := collogsv1.NewLogsServiceClient(conn)
	lResp, err := logsClient.Export(ctx, &collogsv1.ExportLogsServiceRequest{})
	require.NoError(t, err)
	require.NotNil(t, lResp)
}
