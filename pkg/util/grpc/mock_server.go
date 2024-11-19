// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type grpcServer struct {
	pb.UnimplementedAgentSecureServer
}

// NewMockGrpcSecureServer creates a new agent secure gRPC server for testing purposes.
func NewMockGrpcSecureServer(port string) (*grpc.Server, string, error) {
	// Generate a self-signed TLS certificate for the gRPC server.

	tlsKeyPair, err := buildSelfSignedTLSCertificate("127.0.0.1")
	if err != nil {
		return nil, "", err
	}

	// Generate an authentication token and set up our gRPC server to both serve over TLS and authenticate each RPC
	// using the authentication token.
	authToken, err := generateAuthenticationToken()
	if err != nil {
		return nil, "", err
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewServerTLSFromCert(tlsKeyPair)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(StaticAuthInterceptor(authToken))),
	}

	// Start dummy gRPc server mocking the core agent
	serverListener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return nil, "", err
	}

	s := grpc.NewServer(serverOpts...)
	pb.RegisterAgentSecureServer(s, &grpcServer{})

	go func() {
		err := s.Serve(serverListener)
		if err != nil {
			panic(err)
		}
	}()

	return s, authToken, nil
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
