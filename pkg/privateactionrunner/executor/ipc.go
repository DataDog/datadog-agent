// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"net"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	// ProtocolVersion is the version of the local executor IPC protocol.
	ProtocolVersion uint32 = 1

	grpcTarget = "passthrough:///private-action-runner-executor"
)

// SocketPath returns the configured local executor endpoint path.
func SocketPath(cfg model.Reader) string {
	runPath := cfg.GetString("run_path")
	if runPath == "" {
		runPath = filepath.Dir(pkgconfigsetup.DefaultDDAgentBin)
	}
	return defaultSocketPath(runPath)
}

// Listen creates the platform-local listener for the executor IPC server.
func Listen(address string) (net.Listener, error) {
	return listen(address)
}

func newGRPCClient(address string, timeout time.Duration) (*grpc.ClientConn, executorpb.ExecutorClient, error) {
	conn, err := grpc.NewClient(
		grpcTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return dial(ctx, address, timeout)
		}),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, executorpb.NewExecutorClient(conn), nil
}

func withAuth(ctx context.Context, authToken string) context.Context {
	if authToken == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+authToken)
}

func statusResponse(active int32, version string) *executorpb.StatusResponse {
	return &executorpb.StatusResponse{
		ProtocolVersion: ProtocolVersion,
		Ready:           true,
		ActiveTasks:     active,
		Version:         version,
	}
}
