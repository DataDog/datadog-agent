// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"crypto/tls"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/metadata"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// SigningKeysClient fetches authoritative PAR signing-key snapshots.
type SigningKeysClient interface {
	GetPARSigningKeys(context.Context, *pb.GetPARSigningKeysRequest) (*pb.GetPARSigningKeysResponse, error)
}

type authenticatedSigningKeysClient struct {
	client    pb.AgentSecureClient
	authToken string
}

// NewAgentSigningKeysClient creates an authenticated AgentSecure client.
func NewAgentSigningKeysClient(ctx context.Context, ipcAddress, cmdPort, authToken string, tlsConfig *tls.Config) (SigningKeysClient, error) {
	client, err := ddgrpc.GetDDAgentSecureClient(ctx, ipcAddress, cmdPort, tlsConfig,
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.Config{
			BaseDelay:  time.Second,
			Multiplier: 1.2,
			Jitter:     0.2,
			MaxDelay:   5 * time.Second,
		}}),
	)
	if err != nil {
		return nil, err
	}
	return &authenticatedSigningKeysClient{client: client, authToken: authToken}, nil
}

func (c *authenticatedSigningKeysClient) GetPARSigningKeys(ctx context.Context, request *pb.GetPARSigningKeysRequest) (*pb.GetPARSigningKeysResponse, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.authToken)
	return c.client.GetPARSigningKeys(ctx, request)
}
