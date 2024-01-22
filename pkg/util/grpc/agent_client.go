// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpc implements helper functions to interact with grpc
package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var defaultBackoffConfig = backoff.Config{
	BaseDelay:  1.0 * time.Second,
	Multiplier: 1.1,
	Jitter:     0.2,
	MaxDelay:   2 * time.Second,
}

func getGRPCClientConn(ctx context.Context, ipcAddress string, cmdPort string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if cmdPort == "-1" {
		return nil, errors.New("grpc client disabled via cmd_port: -1")
	}

	// This is needed as the server hangs when using "grpc.WithInsecure()"
	tlsConf := tls.Config{InsecureSkipVerify: true}

	opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tlsConf)))

	target := net.JoinHostPort(ipcAddress, cmdPort)

	log.Debugf("attempting to create grpc agent client connection to: %s", target)
	return grpc.DialContext(ctx, target, opts...)
}

// defaultAgentClientDialOpts default dial options to the main agent which blocks and retries based on the backoffConfig
var defaultAgentClientDialOpts = []grpc.DialOption{
	grpc.WithConnectParams(grpc.ConnectParams{Backoff: defaultBackoffConfig}),
	grpc.WithBlock(),
}

// GetDDAgentClient creates a pb.AgentClient for IPC with the main agent via gRPC. This call is blocking by default, so
// it is up to the caller to supply a context with appropriate timeout/cancel options
func GetDDAgentClient(ctx context.Context, ipcAddress string, cmdPort string, opts ...grpc.DialOption) (pb.AgentClient, error) {
	if len(opts) == 0 {
		opts = defaultAgentClientDialOpts
	}
	conn, err := getGRPCClientConn(ctx, ipcAddress, cmdPort, opts...)
	if err != nil {
		return nil, err
	}

	log.Debug("grpc agent client created")
	return pb.NewAgentClient(conn), nil
}

// GetDDAgentSecureClient creates a pb.AgentSecureClient for IPC with the main agent via gRPC. This call is blocking by default, so
// it is up to the caller to supply a context with appropriate timeout/cancel options
func GetDDAgentSecureClient(ctx context.Context, ipcAddress string, cmdPort string, opts ...grpc.DialOption) (pb.AgentSecureClient, error) {
	conn, err := getGRPCClientConn(ctx, ipcAddress, cmdPort, opts...)
	if err != nil {
		return nil, err
	}

	log.Debug("grpc agent secure client created")
	return pb.NewAgentSecureClient(conn), nil
}
