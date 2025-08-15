// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"crypto/tls"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type grpcServer struct {
	pb.UnimplementedAgentSecureServer
}

// NewMockGrpcSecureServer creates a new agent secure gRPC server for testing purposes.
func NewMockGrpcSecureServer(port string, authtoken string, serverTLSConfig *tls.Config) (*grpc.Server, error) {
	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(serverTLSConfig)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(StaticAuthInterceptor(authtoken))),
	}

	// Start dummy gRPc server mocking the core agent
	serverListener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return nil, err
	}

	s := grpc.NewServer(serverOpts...)
	pb.RegisterAgentSecureServer(s, &grpcServer{})

	go func() {
		err := s.Serve(serverListener)
		if err != nil {
			panic(err)
		}
	}()

	return s, nil
}
