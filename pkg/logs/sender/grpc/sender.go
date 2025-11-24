// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpc implements gRPC-based log sender
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"

	"go.uber.org/atomic"
)

const (
	// inputChanBufferSize is the buffer size for worker input channels - may become configurable
	inputChanBufferSize = 100
)

// headerCredentials implements credentials.PerRPCCredentials to add headers to RPC calls
type headerCredentials struct {
	endpoint config.Endpoint
}

// GetRequestMetadata adds required headers to each RPC call
func (h *headerCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	headers := map[string]string{
		"dd-api-key": h.endpoint.GetAPIKey(),
	}

	// Add protocol header if specified
	if h.endpoint.Protocol != "" {
		headers["dd-protocol"] = string(h.endpoint.Protocol)
	}

	// Add origin headers if specified
	if h.endpoint.Origin != "" {
		headers["dd-evp-origin"] = string(h.endpoint.Origin)
		headers["dd-evp-origin-version"] = version.AgentVersion
	}

	if h.endpoint.UseCompression {
		headers["dd-content-encoding"] = h.endpoint.CompressionKind
	} else {
		headers["dd-content-encoding"] = "identity"
	}

	return headers, nil
}

// RequireTransportSecurity indicates whether the credentials require transport security
func (h *headerCredentials) RequireTransportSecurity() bool {
	return false // We handle TLS separately via WithTransportCredentials
}

// Sender implements PipelineComponent interface for gRPC log transmission.
// It manages multiple streamWorker instances (one per pipeline) using round-robin distribution.
// It is similar to Sender/Worker architecture
type Sender struct {
	// Configuration
	endpoint            config.Endpoint
	destinationsContext *client.DestinationsContext
	cfg                 pkgconfigmodel.Reader
	numberOfWorkers     int

	// Pipeline integration
	pipelineMonitor metrics.PipelineMonitor

	// Stream management (similar to Sender's workers and queues)
	workers []*streamWorker
	queues  []chan *message.Payload
	idx     *atomic.Uint32

	// Auditor integration
	sink sender.Sink

	// gRPC connection management (shared across all streams)
	conn   *grpc.ClientConn
	client statefulpb.StatefulLogsServiceClient
}

// NewSender creates a new gRPC sender that implements PipelineComponent
// numberOfPipelines determines how many streamWorker to create (same as number of pipelines)
func NewSender(
	numberOfPipelines int,
	cfg pkgconfigmodel.Reader,
	sink sender.Sink,
	endpoints *config.Endpoints,
	destinationsCtx *client.DestinationsContext,
	compressor logscompression.Component,
) *Sender {

	// For now, use the first reliable endpoint
	// TODO: Support multiple endpoints with failover
	var endpoint config.Endpoint
	if len(endpoints.GetReliableEndpoints()) > 0 {
		endpoint = endpoints.GetReliableEndpoints()[0]
	} else {
		log.Error("No reliable gRPC endpoints configured")
		return nil
	}

	// For the moment, we use the number of pipelines as the number of workers
	numberOfWorkers := numberOfPipelines

	// Get stream lifetime from config
	streamLifetime := config.StreamLifetime(cfg)

	// Create compressor for snapshot compression based on endpoint config
	var comp compression.Compressor
	comp = compressor.NewCompressor(compression.NoneKind, 0)
	if endpoint.UseCompression {
		comp = compressor.NewCompressor(endpoint.CompressionKind, endpoint.CompressionLevel)
	}

	sender := &Sender{
		endpoint:            endpoint,
		destinationsContext: destinationsCtx,
		cfg:                 cfg,
		numberOfWorkers:     numberOfWorkers,
		pipelineMonitor:     metrics.NewTelemetryPipelineMonitor(),
		workers:             make([]*streamWorker, 0, numberOfWorkers),
		queues:              make([]chan *message.Payload, numberOfWorkers),
		idx:                 &atomic.Uint32{},
		sink:                sink,
	}

	// Note: outputChan will be set in each streamWorker's start() method when sink.Channel() is available

	// Create gRPC connection (shared by all streams inside streamWorkers)
	if err := sender.createConnection(); err != nil {
		log.Errorf("Failed to create gRPC connection: %v", err)
		return nil
	}

	// Create multiple streamWorker instances (like Sender creates Workers)
	for i := 0; i < numberOfWorkers; i++ {
		workerID := fmt.Sprintf("worker-%d", i)

		// Create input queue for this worker (like Sender creates queues)
		sender.queues[i] = make(chan *message.Payload, inputChanBufferSize)

		// Create streamWorker instance
		worker := newStreamWorker(
			workerID,
			sender.queues[i],
			destinationsCtx,
			sender.conn,
			sender.client,
			sender.sink,
			endpoint,
			streamLifetime,
			comp,
		)

		sender.workers = append(sender.workers, worker)
	}

	log.Infof("Created gRPC sender with %d streams for endpoint %s:%d",
		numberOfWorkers, endpoint.Host, endpoint.Port)
	return sender
}

// createConnection establishes the shared gRPC connection
func (s *Sender) createConnection() error {
	log.Infof("Creating gRPC connection to %s:%d", s.endpoint.Host, s.endpoint.Port)

	// Build connection options
	var opts []grpc.DialOption

	// Configure TLS
	if s.endpoint.UseSSL() {
		tlsConfig := &tls.Config{
			ServerName: s.endpoint.Host,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Configure keepalive
	keepaliveParams := keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}
	opts = append(opts, grpc.WithKeepaliveParams(keepaliveParams))

	// Add user agent
	userAgent := fmt.Sprintf("datadog-agent/%s", version.AgentVersion)
	opts = append(opts, grpc.WithUserAgent(userAgent))

	// Add headers via per-RPC credentials
	headerCreds := &headerCredentials{endpoint: s.endpoint}
	opts = append(opts, grpc.WithPerRPCCredentials(headerCreds))

	// Add load balancing configuration, to utilize all available LB IPs
	opts = append(opts, grpc.WithDefaultServiceConfig(
		`{"loadBalancingPolicy":"round_robin"}`,
	))

	// Create connection, lazy connection establishment, does not block
	address := fmt.Sprintf("%s:%d", s.endpoint.Host, s.endpoint.Port)
	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	s.conn = conn
	s.client = statefulpb.NewStatefulLogsServiceClient(conn)

	log.Infof("Successfully created gRPC connection to %s", address)
	return nil
}

// PipelineComponent interface implementation

// In returns the input channel using round-robin distribution (same as Sender.In())
func (s *Sender) In() chan *message.Payload {
	idx := s.idx.Inc() % uint32(len(s.queues))
	return s.queues[idx]
}

// PipelineMonitor returns the pipeline monitor
func (s *Sender) PipelineMonitor() metrics.PipelineMonitor {
	return s.pipelineMonitor
}

// Start starts all streamWorker instances (same pattern as Sender.Start())
func (s *Sender) Start() {
	log.Infof("Starting gRPC sender with %d workers", len(s.workers))

	for _, worker := range s.workers {
		worker.start()
	}

	log.Info("All streamWorkers started")
}

// Stop stops all streamWorker instances and closes the connection
func (s *Sender) Stop() {
	log.Info("Stopping gRPC sender")

	// Stop all workers (same pattern as Sender.Stop())
	for _, worker := range s.workers {
		worker.stop()
	}

	// Close all queues
	for _, queue := range s.queues {
		close(queue)
	}

	// Close the shared connection
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			log.Warnf("Error closing gRPC connection: %v", err)
		}
	}

	log.Info("gRPC sender stopped")
}
