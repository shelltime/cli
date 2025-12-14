package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	collogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collmetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
)

// CCOtelServer is the gRPC server for receiving OTEL data from Claude Code
type CCOtelServer struct {
	port       int
	processor  *CCOtelProcessor
	grpcServer *grpc.Server
	listener   net.Listener
}

// NewCCOtelServer creates a new CCOtel gRPC server
func NewCCOtelServer(port int, processor *CCOtelProcessor) *CCOtelServer {
	return &CCOtelServer{
		port:      port,
		processor: processor,
	}
}

// Start starts the gRPC server
func (s *CCOtelServer) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	s.grpcServer = grpc.NewServer()

	// Register OTEL collector services
	collmetricsv1.RegisterMetricsServiceServer(s.grpcServer, &metricsServiceServer{processor: s.processor})
	collogsv1.RegisterLogsServiceServer(s.grpcServer, &logsServiceServer{processor: s.processor})

	slog.Info("CCOtel gRPC server starting", "port", s.port)

	// Start serving in a goroutine
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			slog.Error("CCOtel gRPC server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the gRPC server
func (s *CCOtelServer) Stop() {
	if s.grpcServer != nil {
		slog.Info("CCOtel gRPC server stopping")
		s.grpcServer.GracefulStop()
	}
}

// metricsServiceServer implements the OTEL MetricsService
type metricsServiceServer struct {
	collmetricsv1.UnimplementedMetricsServiceServer
	processor *CCOtelProcessor
}

// Export handles incoming metrics export requests
func (s *metricsServiceServer) Export(ctx context.Context, req *collmetricsv1.ExportMetricsServiceRequest) (*collmetricsv1.ExportMetricsServiceResponse, error) {
	return s.processor.ProcessMetrics(ctx, req)
}

// logsServiceServer implements the OTEL LogsService
type logsServiceServer struct {
	collogsv1.UnimplementedLogsServiceServer
	processor *CCOtelProcessor
}

// Export handles incoming logs export requests
func (s *logsServiceServer) Export(ctx context.Context, req *collogsv1.ExportLogsServiceRequest) (*collogsv1.ExportLogsServiceResponse, error) {
	return s.processor.ProcessLogs(ctx, req)
}
