// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main contains the logic for the remote-agent example client
package main

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// StatusServiceName is the service name for remote agent status provider
const StatusServiceName = "datadog.remoteagent.status.v1.StatusProvider"

// FlareServiceName is the service name for remote agent flare provider
const FlareServiceName = "datadog.remoteagent.flare.v1.FlareProvider"

// TelemetryServiceName is the service name for remote agent telemetry provider
const TelemetryServiceName = "datadog.remoteagent.telemetry.v1.TelemetryProvider"

type remoteAgentServer struct {
	started time.Time
	pbcore.UnimplementedStatusProviderServer
	pbcore.UnimplementedFlareProviderServer
	pbcore.UnimplementedTelemetryProviderServer
}

func (s *remoteAgentServer) GetStatusDetails(_ context.Context, req *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	log.Printf("Got request for status details: %v", req)

	fields := make(map[string]string)
	fields["Started"] = s.started.Format(time.RFC3339)
	fields["Version"] = "1.0.0"

	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{
			Fields: fields,
		},
		NamedSections: make(map[string]*pbcore.StatusSection),
	}, nil
}

func (s *remoteAgentServer) GetFlareFiles(_ context.Context, req *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	log.Printf("Got request for flare files: %v", req)

	files := make(map[string][]byte, 0)
	files["example.txt"] = []byte("Hello, world!\n")

	return &pbcore.GetFlareFilesResponse{
		Files: files,
	}, nil
}

func (s *remoteAgentServer) GetTelemetry(_ context.Context, req *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	log.Printf("Got request for telemetry: %v", req)

	// Testing histogram support in RAR telemetry service
	// This includes multiple scenarios to test bucket mismatch behavior:
	//
	// Scenario 1: Unique histogram name (should work fine)
	// Scenario 2: Same name as internal Agent histogram (remote_agent_registry_action_duration_seconds)
	//             but with DIFFERENT bucket boundaries - tests for potential conflicts
	// Scenario 3: Histogram with labels that might match internal histogram labels
	var prometheusText = `
# TYPE remote_agent_test_foo counter
remote_agent_test_foo 62
# TYPE remote_agent_test_bar gauge
remote_agent_test_bar{tag_one="1",tag_two="two"} 3
# HELP my_custom_histogram A unique histogram from remote agent (Scenario 1)
# TYPE my_custom_histogram histogram
my_custom_histogram_bucket{le="0.1"} 10
my_custom_histogram_bucket{le="0.5"} 25
my_custom_histogram_bucket{le="1.0"} 30
my_custom_histogram_bucket{le="+Inf"} 35
my_custom_histogram_sum 15.5
my_custom_histogram_count 35
# HELP remote_agent_registry_action_duration_seconds Conflicting histogram - same name as internal Agent metric but different buckets (Scenario 2)
# TYPE remote_agent_registry_action_duration_seconds histogram
remote_agent_registry_action_duration_seconds_bucket{le="1"} 5
remote_agent_registry_action_duration_seconds_bucket{le="10"} 15
remote_agent_registry_action_duration_seconds_bucket{le="+Inf"} 20
remote_agent_registry_action_duration_seconds_sum 50.0
remote_agent_registry_action_duration_seconds_count 20
# HELP remote_agent_registry_action_duration_seconds_with_labels Histogram with labels matching internal metric (Scenario 3)
# TYPE remote_agent_registry_action_duration_seconds_with_labels histogram
remote_agent_registry_action_duration_seconds_with_labels_bucket{name="test-agent",action="query",le="0.5"} 10
remote_agent_registry_action_duration_seconds_with_labels_bucket{name="test-agent",action="query",le="2.0"} 25
remote_agent_registry_action_duration_seconds_with_labels_bucket{name="test-agent",action="query",le="+Inf"} 30
remote_agent_registry_action_duration_seconds_with_labels_sum{name="test-agent",action="query"} 12.5
remote_agent_registry_action_duration_seconds_with_labels_count{name="test-agent",action="query"} 30
`
	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: prometheusText,
		},
	}, nil
}

func newRemoteAgentServer() *remoteAgentServer {
	return &remoteAgentServer{
		started: time.Now(),
	}
}

// registerWithAgent handles the registration logic with the Core Agent
func registerWithAgent(agentIpcAddress, agentAuthToken, agentFlavor, displayName, listenAddr string, clientTLSConfig *tls.Config, refreshTicker *time.Ticker) (string, pbcore.AgentSecureClient, error) {
	log.Println("Session ID is empty, entering registration loop")

	agentClient, err := newAgentSecureClient(agentIpcAddress, agentAuthToken, clientTLSConfig)
	if err != nil {
		log.Printf("failed to create agent client: %v", err)
		return "", nil, err
	}

	registerReq := &pbcore.RegisterRemoteAgentRequest{
		Flavor:         agentFlavor,
		DisplayName:    displayName,
		ApiEndpointUri: listenAddr,
		Services:       []string{StatusServiceName, FlareServiceName, TelemetryServiceName},
	}

	log.Printf("Registering with Core Agent at %s...", agentIpcAddress)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := agentClient.RegisterRemoteAgent(ctx, registerReq)
	if err != nil {
		log.Printf("failed to register remote agent: %v", err)
		return "", nil, err
	}

	// Store the session ID for use in the session ID interceptor
	sessionID := resp.SessionId
	refreshTicker.Reset(time.Duration(resp.RecommendedRefreshIntervalSecs) * time.Second)
	log.Printf("Registered with Core Agent. Recommended refresh interval of %d seconds.", resp.RecommendedRefreshIntervalSecs)

	return sessionID, agentClient, nil
}

// refreshRegistration handles the refresh logic with the Core Agent
func refreshRegistration(agentClient pbcore.AgentSecureClient, sessionID string) error {
	_, err := agentClient.RefreshRemoteAgent(context.Background(), &pbcore.RefreshRemoteAgentRequest{SessionId: sessionID})
	if err != nil {
		return err
	}

	log.Println("Refreshed registration with Core Agent.")
	return nil
}

func main() {
	// Read in all of the necessary configuration for this remote agent.
	var agentFlavor string
	var displayName string
	var listenAddr string
	var agentIpcAddress string
	var agentAuthTokenFilePath string
	var agentIPCCertFilePath string
	var sessionID string
	var agentClient pbcore.AgentSecureClient

	flag.StringVar(&agentFlavor, "agent-flavor", "", "Agent Flavor")
	flag.StringVar(&displayName, "display-name", "", "Display name to register with")
	flag.StringVar(&listenAddr, "listen-addr", "", "Address to listen on")
	flag.StringVar(&agentIpcAddress, "agent-ipc-address", "", "Agent IPC server address")
	flag.StringVar(&agentAuthTokenFilePath, "agent-auth-token-file", "", "Path to Agent authentication token file")
	flag.StringVar(&agentIPCCertFilePath, "agent-cert-file", "", "Path to Agent IPC certificate file")
	flag.Parse()

	if flag.NFlag() != 6 {
		flag.Usage()
		os.Exit(1)
	}

	// Now we'll register with the Core Agent, pointing it to our gRPC server.
	rawAgentAuthToken, err := os.ReadFile(agentAuthTokenFilePath)
	if err != nil {
		log.Fatalf("failed to read agent auth token file: %v", err)
	}

	agentAuthToken := string(rawAgentAuthToken)

	// Read the IPC certificate from the agent (for our gRPC server and for client connection to agent IPC)
	tlsCert, err := getAgentCert(agentIPCCertFilePath)
	if err != nil {
		log.Fatalf("failed to get agent IPC cert: %v", err)
	}
	clientTLSConfig, err := cert.LoadIPCClientTLSConfigFromFile(agentIPCCertFilePath)
	if err != nil {
		log.Fatalf("failed to load IPC client TLS config: %v", err)
	}

	// Build and spawn our gRPC server.
	err = buildAndSpawnGrpcServer(listenAddr, newRemoteAgentServer(), agentAuthToken, &tlsCert, &sessionID)
	if err != nil {
		log.Fatalf("failed to build/spawn gRPC server: %v", err)
	}

	log.Printf("Spawned remote agent gRPC server on %s.", listenAddr)

	// Wait forever, periodically refreshing our registration.
	refreshTicker := time.NewTicker(500 * time.Millisecond)
	for range refreshTicker.C {
		if sessionID == "" {
			var err error
			sessionID, agentClient, err = registerWithAgent(agentIpcAddress, agentAuthToken, agentFlavor, displayName, listenAddr, clientTLSConfig, refreshTicker)
			if err != nil {
				continue
			}
		} else {
			err := refreshRegistration(agentClient, sessionID)
			if err != nil {
				log.Printf("failed to refresh registration with Core Agent: %v, entering registration loop", err)
				sessionID = ""
				continue
			}
		}
	}
}

func getAgentCert(path string) (tls.Certificate, error) {
	cert := tls.Certificate{}

	// Getting the IPC certificate from the agent
	rawFile, err := os.ReadFile(path)
	if err != nil {
		return cert, fmt.Errorf("error while creating or fetching IPC cert: %w", err)
	}

	// Decode the certificate
	block, rest := pem.Decode(rawFile)

	if block == nil || block.Type != "CERTIFICATE" {
		return cert, fmt.Errorf("failed to decode PEM block containing certificate")
	}
	rawCert := pem.EncodeToMemory(block)

	block, _ = pem.Decode(rest)

	if block == nil || block.Type != "EC PRIVATE KEY" {
		return cert, fmt.Errorf("failed to decode PEM block containing key")
	}

	rawKey := pem.EncodeToMemory(block)

	tlsCert, err := tls.X509KeyPair(rawCert, rawKey)
	if err != nil {
		return cert, fmt.Errorf("Unable to generate x509 cert from PERM IPC cert and key")
	}
	return tlsCert, nil
}

func buildAndSpawnGrpcServer(listenAddr string, server *remoteAgentServer, authToken string, cert *tls.Certificate, sessionID *string) error {
	// Make sure we can listen on the intended address.
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	chainedInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// first apply auth interceptor
		authHandler := grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(authToken))
		return authHandler(ctx, req, info, func(ctx context.Context, req any) (any, error) {
			// then apply session ID interceptor
			return sessionIDInterceptor(sessionID)(ctx, req, info, handler)
		})
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewServerTLSFromCert(cert)),
		grpc.UnaryInterceptor(chainedInterceptor),
	}

	grpcServer := grpc.NewServer(serverOpts...)
	pbcore.RegisterStatusProviderServer(grpcServer, server)
	pbcore.RegisterFlareProviderServer(grpcServer, server)
	pbcore.RegisterTelemetryProviderServer(grpcServer, server)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	return nil
}

func newAgentSecureClient(ipcAddress string, agentAuthToken string, tlsConfig *tls.Config) (pbcore.AgentSecureClient, error) {
	tlsCreds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.NewClient(ipcAddress,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(agentAuthToken)),
	)
	if err != nil {
		return nil, err
	}

	return pbcore.NewAgentSecureClient(conn), nil
}

// Create session ID interceptor that adds session_id to response metadata
func sessionIDInterceptor(sessionID *string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		_ = grpc.SetHeader(ctx, metadata.New(map[string]string{"session_id": *sessionID}))
		return handler(ctx, req)
	}
}
