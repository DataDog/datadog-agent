// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// systemProbeGrpcServer implements the RemoteAgent gRPC service for system-probe
type systemProbeGrpcServer struct {
	telemetryComp telemetry.Component
	pbcore.UnimplementedRemoteAgentServer
}

// GetTelemetry implements the RemoteAgent.GetTelemetry gRPC method
func (s *systemProbeGrpcServer) GetTelemetry(ctx context.Context, req *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	log.Debugf("Received telemetry request: %v", req)

	// Get telemetry from the existing telemetry component
	// We need to convert the HTTP handler to return Prometheus text format
	handler := s.telemetryComp.Handler()

	// Create a fake HTTP request to get the telemetry data
	fakeReq, err := http.NewRequestWithContext(ctx, "GET", "/telemetry", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create telemetry request: %w", err)
	}

	// Create a response recorder to capture the telemetry output
	recorder := &responseRecorder{}
	handler.ServeHTTP(recorder, fakeReq)

	// HTTP handlers that don't call WriteHeader implicitly return 200
	if recorder.code == 0 {
		recorder.code = http.StatusOK
	}

	if recorder.code != http.StatusOK {

		return nil, fmt.Errorf("telemetry handler returned status %d", recorder.code)
	}

	// Filter to only COAT metrics - avoid sending duplicates
	coatMetrics := filterCOATMetrics(recorder.body)

	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: coatMetrics,
		},
	}, nil
}

// GetStatusDetails implements the RemoteAgent.GetStatusDetails gRPC method
// TODO(mb) this is a placeholder for now
func (s *systemProbeGrpcServer) GetStatusDetails(_ context.Context, req *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	log.Debugf("Received status details request: %v", req)

	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{
			Fields: map[string]string{
				"status": "System Probe is running",
			},
		},
		NamedSections: make(map[string]*pbcore.StatusSection),
	}, nil
}

// GetFlareFiles implements the RemoteAgent.GetFlareFiles gRPC method
// TODO(mb) this is a placeholder for now
func (s *systemProbeGrpcServer) GetFlareFiles(_ context.Context, req *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	log.Debugf("Received flare files request: %v", req)

	return &pbcore.GetFlareFilesResponse{
		Files: make(map[string][]byte),
	}, nil
}

// responseRecorder is a simple HTTP response recorder
type responseRecorder struct {
	code int
	body string
}

func (r *responseRecorder) Header() http.Header {
	return make(http.Header)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.body += string(data)
	return len(data), nil
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

// startGrpcServer starts a gRPC server on the specified address with the given auth token
func startGrpcServer(listenAddr, _ string, telemetryComp telemetry.Component) (string, error) {
	// Generate self-signed certificate (following internal/remote-agent pattern)
	cert, err := generateSelfSignedCert()
	if err != nil {
		return "", fmt.Errorf("failed to generate certificate: %w", err)
	}

	// Create TLS credentials
	creds := credentials.NewServerTLSFromCert(&cert)

	// Create gRPC server with TLS
	server := grpc.NewServer(grpc.Creds(creds))

	// Register our RemoteAgent service
	systemProbeServer := &systemProbeGrpcServer{
		telemetryComp: telemetryComp,
	}
	pbcore.RegisterRemoteAgentServer(server, systemProbeServer)

	// Listen on the specified address
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	actualAddr := lis.Addr().String()
	log.Infof("Starting gRPC server on %s", actualAddr)

	// Start server in goroutine
	go func() {
		if err := server.Serve(lis); err != nil {
			log.Errorf("gRPC server error: %v", err)
		}
	}()

	return actualAddr, nil
}

// generateSelfSignedCert generates a self-signed certificate for the gRPC server
// Following the pattern from internal/remote-agent/main.go
func generateSelfSignedCert() (tls.Certificate, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Datadog"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:              []string{"localhost"},
	}

	// Generate certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Create TLS certificate
	cert := tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  privateKey,
	}

	return cert, nil
}

// generateRandomToken generates a random authentication token
func generateRandomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", bytes), nil
}

// startGrpcServerAndRemoteAgentRegistry starts the gRPC server and registers with Remote Agent Registry
func startGrpcServerAndRemoteAgentRegistry(telemetryComp telemetry.Component, config model.Reader, ipcComp ipc.Component) error {
	// Generate random auth token for gRPC server
	authToken, err := generateRandomToken()
	if err != nil {
		return fmt.Errorf("failed to generate auth token: %w", err)
	}

	// Start gRPC server on localhost with random port
	grpcAddr, err := startGrpcServer("localhost:0", authToken, telemetryComp)
	if err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	log.Infof("Started system-probe gRPC server on %s", grpcAddr)

	// Create Remote Agent Registry client and register with core agent
	remoteAgentRegistryClient, err := NewRemoteAgentRegistryClient(grpcAddr, authToken, config, ipcComp)
	if err != nil {
		return fmt.Errorf("failed to create Remote Agent Registry client: %w", err)
	}

	// Register with core agent in background to avoid blocking startup
	go func() {
		if err := remoteAgentRegistryClient.Register(); err != nil {
			log.Errorf("Failed to register with Remote Agent Registry: %v", err)
		}
	}()

	return nil
}

// filterCOATMetrics extracts only COAT metrics (system_probe_*) from Prometheus text format
// we do assume all COAT metrics starts with "system_probe_"
// this will avoid sending full metrics duplicates to core agent and emitting them twice
// NOTE(mb) we may want to move this to another pkg
// NOTE(mb) we may want to simlify the logic and keep a simple list of all metrics we want to expose for COAT
func filterCOATMetrics(promText string) string {
	lines := strings.Split(promText, "\n")
	var coatLines []string

	inCOATMetric := false
	for _, line := range lines {
		// Check if this line starts a COAT metric (# HELP or # TYPE system_probe_*)
		if strings.HasPrefix(line, "# HELP system_probe_") || strings.HasPrefix(line, "# TYPE system_probe_") {
			inCOATMetric = true
			coatLines = append(coatLines, line)
		} else if strings.HasPrefix(line, "system_probe_") {
			// Metric value line for COAT metric
			coatLines = append(coatLines, line)
			inCOATMetric = false
		} else if strings.HasPrefix(line, "# ") {
			// Different metric's help/type - no longer in COAT metric
			inCOATMetric = false
		} else if inCOATMetric {
			// Continuation line for COAT metric (multiline help text)
			coatLines = append(coatLines, line)
		}
		// Skip all other lines (non-COAT metrics)
	}

	result := strings.Join(coatLines, "\n")
	if len(result) > 0 {
		result += "\n"
	}
	return result
}
