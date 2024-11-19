// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main contains the logic for the remote-agent example client
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

type remoteAgentServer struct {
	started time.Time
}

func (s *remoteAgentServer) GetStatusDetails(_ context.Context, req *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	log.Printf("Got request for status details: %v", req)

	fields := make(map[string]string)
	fields["Started"] = s.started.Format(time.RFC3339)

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

func newRemoteAgentServer() *remoteAgentServer {
	return &remoteAgentServer{
		started: time.Now(),
	}
}

func main() {
	// Read in all of the necessary configuration for this remote agent.
	var agentID string
	var displayName string
	var listenAddr string
	var agentIpcAddress string
	var agentAuthTokenFilePath string

	flag.StringVar(&agentID, "agent-id", "", "Agent ID to register with")
	flag.StringVar(&displayName, "display-name", "", "Display name to register with")
	flag.StringVar(&listenAddr, "listen-addr", "", "Address to listen on")
	flag.StringVar(&agentIpcAddress, "agent-ipc-address", "", "Agent IPC server address")
	flag.StringVar(&agentAuthTokenFilePath, "agent-auth-token-file", "", "Path to Agent authentication token file")

	flag.Parse()

	if flag.NFlag() != 5 {
		flag.Usage()
		os.Exit(1)
	}

	// Build and spawn our gRPC server.
	selfAuthToken, err := buildAndSpawnGrpcServer(listenAddr, newRemoteAgentServer())
	if err != nil {
		log.Fatalf("failed to build/spawn gRPC server: %v", err)
	}

	log.Printf("Spawned remote agent gRPC server on %s.", listenAddr)

	// Now we'll register with the Core Agent, pointing it to our gRPC server.
	rawAgentAuthToken, err := os.ReadFile(agentAuthTokenFilePath)
	if err != nil {
		log.Fatalf("failed to read agent auth token file: %v", err)
	}

	agentAuthToken := string(rawAgentAuthToken)
	agentClient, err := newAgentSecureClient(agentIpcAddress, agentAuthToken)
	if err != nil {
		log.Fatalf("failed to create agent client: %v", err)
	}

	registerReq := &pbcore.RegisterRemoteAgentRequest{
		Id:          agentID,
		DisplayName: displayName,
		ApiEndpoint: listenAddr,
		AuthToken:   selfAuthToken,
	}

	log.Printf("Registering with Core Agent at %s...", agentIpcAddress)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := agentClient.RegisterRemoteAgent(ctx, registerReq)
	if err != nil {
		log.Fatalf("failed to register remote agent: %v", err)
	}

	log.Printf("Registered with Core Agent. Recommended refresh interval of %d seconds.", resp.RecommendedRefreshIntervalSecs)

	// Wait forever, periodically refreshing our registration.
	refreshTicker := time.NewTicker(time.Duration(resp.RecommendedRefreshIntervalSecs) * time.Second)
	for range refreshTicker.C {
		_, err := agentClient.RegisterRemoteAgent(context.Background(), registerReq)
		if err != nil {
			log.Fatalf("failed to refresh remote agent registration: %v", err)
		}

		log.Println("Refreshed registration with Core Agent.")
	}
}

func buildAndSpawnGrpcServer(listenAddr string, server pbcore.RemoteAgentServer) (string, error) {
	// Generate a self-signed certificate for our server.
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return "", fmt.Errorf("unable to extract hostname from listen address: %v", err)
	}

	tlsKeyPair, err := buildSelfSignedTLSCertificate(host)
	if err != nil {
		return "", fmt.Errorf("unable to generate TLS certificate: %v", err)
	}

	// Make sure we can listen on the intended address.
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Generate an authentication token and set up our gRPC server to both serve over TLS and authenticate each RPC
	// using the authentication token.
	authToken, err := generateAuthenticationToken()
	if err != nil {
		return "", fmt.Errorf("unable to generate authentication token: %v", err)
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewServerTLSFromCert(tlsKeyPair)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(authToken))),
	}

	grpcServer := grpc.NewServer(serverOpts...)
	pbcore.RegisterRemoteAgentServer(grpcServer, server)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	return authToken, nil
}

func buildSelfSignedTLSCertificate(host string) (*tls.Certificate, error) {
	hosts := []string{host}
	_, certPEM, key, err := security.GenerateRootCert(hosts, 2048)
	if err != nil {
		return nil, errors.New("unable to generate certificate")
	}

	// PEM encode the private key
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("unable to generate TLS key pair: %v", err)
	}

	return &pair, nil
}

func generateAuthenticationToken() (string, error) {
	rawToken := make([]byte, 32)
	_, err := rand.Read(rawToken)
	if err != nil {
		return "", fmt.Errorf("can't create authentication token value: %s", err)
	}

	return hex.EncodeToString(rawToken), nil
}

func newAgentSecureClient(ipcAddress string, agentAuthToken string) (pbcore.AgentSecureClient, error) {
	tlsCreds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	conn, err := grpc.NewClient(ipcAddress,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(agentAuthToken)),
	)
	if err != nil {
		return nil, err
	}

	return pbcore.NewAgentSecureClient(conn), nil
}
