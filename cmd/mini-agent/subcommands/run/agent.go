// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements the mini-agent run subcommand.
package run

import (
	"context"
	"fmt"
	"net"
	"time"

	grpcauth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaserver "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// miniAgent implements the minimal agent functionality
type miniAgent struct {
	config       coreconfig.Component
	log          log.Component
	serializer   serializer.MetricSerializer
	hostname     hostnameinterface.Component
	tagger       tagger.Component
	workloadmeta workloadmeta.Component
	ipc          ipc.Component
	telemetry    telemetry.Component

	// gRPC server
	grpcServer *grpc.Server
	serverPort int
	listener   net.Listener

	// Metric submission goroutine management
	metricCtx    context.Context
	metricCancel context.CancelFunc
}

// newMiniAgent creates a new mini-agent instance
func newMiniAgent(
	config coreconfig.Component,
	log log.Component,
	serializer serializer.MetricSerializer,
	hostname hostnameinterface.Component,
	tagger tagger.Component,
	workloadmeta workloadmeta.Component,
	ipc ipc.Component,
	telemetry telemetry.Component,
) *miniAgent {
	return &miniAgent{
		config:       config,
		log:          log,
		serializer:   serializer,
		hostname:     hostname,
		tagger:       tagger,
		workloadmeta: workloadmeta,
		ipc:          ipc,
		telemetry:    telemetry,
	}
}

// start starts the mini-agent components
func (m *miniAgent) start() error {
	m.log.Info("Starting mini-agent")

	// Start gRPC server if enabled
	enableServer := m.config.GetBool("mini_agent.server.enabled")
	if enableServer {
		if err := m.startGRPCServer(); err != nil {
			m.log.Errorf("Failed to start gRPC server: %v", err)
			return err
		}
	}

	// Start periodic metric submission if enabled (default: true)
	submitRunningMetric := true
	if m.config.IsSet("mini_agent.submit_running_metric") {
		submitRunningMetric = m.config.GetBool("mini_agent.submit_running_metric")
	}
	if submitRunningMetric {
		m.startMetricSubmission()
	} else {
		m.log.Info("Running metric submission disabled")
	}

	m.log.Infof("mini-agent started successfully (grpc_port=%d)", m.serverPort)

	// Block forever
	select {}
}

// startGRPCServer starts the gRPC server with tagger and workloadmeta services
func (m *miniAgent) startGRPCServer() error {
	addr := m.config.GetString("mini_agent.server.address")
	if addr == "" {
		addr = "localhost"
	}
	port := m.config.GetInt("mini_agent.server.port")
	if port == 0 {
		port = 5002
	}

	// 1. Create listener
	listenAddr := fmt.Sprintf("%s:%d", addr, port)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to create listener on %s: %w", listenAddr, err)
	}
	m.listener = lis

	// Store the actual port (useful when auto-assigning with port 0)
	m.serverPort = lis.Addr().(*net.TCPAddr).Port

	// 2. Setup gRPC server with authentication
	var grpcOpts []grpc.ServerOption

	// Get TLS credentials from IPC component
	tlsConf := m.ipc.GetTLSServerConfig()
	if tlsConf != nil {
		creds := credentials.NewTLS(tlsConf)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
		m.log.Debug("gRPC server: TLS enabled")
	} else {
		m.log.Warn("gRPC server: TLS not configured, running without TLS")
	}

	// Add auth interceptor from IPC
	authInterceptor := grpcauth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(m.ipc.GetAuthToken()))
	grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(authInterceptor))

	// Add stream auth interceptor
	streamAuthInterceptor := grpcauth.StreamServerInterceptor(grpcutil.StaticAuthInterceptor(m.ipc.GetAuthToken()))
	grpcOpts = append(grpcOpts, grpc.StreamInterceptor(streamAuthInterceptor))

	// Set max message size
	maxMessageSize := m.config.GetInt("mini_agent.server.max_message_size")
	if maxMessageSize == 0 {
		maxMessageSize = 50 * 1024 * 1024 // 50MB default
	}
	grpcOpts = append(grpcOpts,
		grpc.MaxRecvMsgSize(maxMessageSize),
		grpc.MaxSendMsgSize(maxMessageSize),
	)

	// 3. Create gRPC server
	m.grpcServer = grpc.NewServer(grpcOpts...)

	// 4. Create tagger and workloadmeta servers using existing implementations
	maxEventSize := maxMessageSize / 2
	maxParallelSync := m.config.GetInt("mini_agent.server.max_concurrent_sync")
	if maxParallelSync == 0 {
		maxParallelSync = 4
	}

	taggerSrv := taggerserver.NewServer(m.tagger, m.telemetry, maxEventSize, maxParallelSync)
	workloadmetaSrv := workloadmetaserver.NewServer(m.workloadmeta)

	// 5. Register services using a combined server wrapper
	pb.RegisterAgentSecureServer(m.grpcServer, &agentSecureServer{
		taggerServer:       taggerSrv,
		workloadmetaServer: workloadmetaSrv,
	})

	// 6. Start serving in goroutine
	go func() {
		m.log.Infof("Starting gRPC server on %s (port %d)", listenAddr, m.serverPort)
		if err := m.grpcServer.Serve(lis); err != nil {
			m.log.Errorf("gRPC server error: %v", err)
		}
	}()

	return nil
}

// startMetricSubmission starts the goroutine that periodically submits the running metric
func (m *miniAgent) startMetricSubmission() {
	metadataInterval := m.config.GetInt("mini_agent.metadata_interval")
	if metadataInterval <= 0 {
		metadataInterval = 60 // Default to 60 seconds
	}

	m.metricCtx, m.metricCancel = context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(time.Duration(metadataInterval) * time.Second)
		defer ticker.Stop()

		// Submit immediately on start
		m.submitRunningMetric()

		for {
			select {
			case <-ticker.C:
				m.submitRunningMetric()
			case <-m.metricCtx.Done():
				m.log.Debug("Metric submission goroutine stopped")
				return
			}
		}
	}()

	m.log.Infof("Started periodic metric submission (interval: %ds)", metadataInterval)
}

// submitRunningMetric submits the mini_agent.running metric
func (m *miniAgent) submitRunningMetric() {
	hostname, err := m.hostname.Get(context.Background())
	if err != nil {
		m.log.Warnf("Failed to get hostname for running metric: %v", err)
		hostname = "unknown"
	}

	// Create the metric series
	series := metrics.NewIterableSeries(func(*metrics.Serie) {}, 1, 1)
	series.Append(&metrics.Serie{
		Name:   "mini_agent.running",
		Points: []metrics.Point{{Ts: float64(time.Now().Unix()), Value: 1.0}},
		Tags:   tagset.CompositeTagsFromSlice([]string{"host:" + hostname}),
		MType:  metrics.APIGaugeType,
		Host:   hostname,
		Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
	})

	// Send the metric
	if err := m.serializer.SendIterableSeries(series); err != nil {
		m.log.Warnf("Failed to submit running metric: %v", err)
	}
}

// agentSecureServer wraps the tagger and workloadmeta servers to implement pb.AgentSecureServer
type agentSecureServer struct {
	pb.UnimplementedAgentSecureServer
	taggerServer       *taggerserver.Server
	workloadmetaServer *workloadmetaserver.Server
}

// TaggerStreamEntities delegates to the tagger server
func (s *agentSecureServer) TaggerStreamEntities(in *pb.StreamTagsRequest, out pb.AgentSecure_TaggerStreamEntitiesServer) error {
	return s.taggerServer.TaggerStreamEntities(in, out)
}

// TaggerFetchEntity delegates to the tagger server
func (s *agentSecureServer) TaggerFetchEntity(ctx context.Context, in *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	return s.taggerServer.TaggerFetchEntity(ctx, in)
}

// TaggerGenerateContainerIDFromOriginInfo delegates to the tagger server
func (s *agentSecureServer) TaggerGenerateContainerIDFromOriginInfo(ctx context.Context, in *pb.GenerateContainerIDFromOriginInfoRequest) (*pb.GenerateContainerIDFromOriginInfoResponse, error) {
	return s.taggerServer.TaggerGenerateContainerIDFromOriginInfo(ctx, in)
}

// WorkloadmetaStreamEntities delegates to the workloadmeta server
func (s *agentSecureServer) WorkloadmetaStreamEntities(in *pb.WorkloadmetaStreamRequest, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	return s.workloadmetaServer.StreamEntities(in, out)
}
