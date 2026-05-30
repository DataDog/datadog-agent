// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

type grpcClient struct {
	socketPath string
}

func newGRPCClient(socketPath string) Client {
	return &grpcClient{socketPath: socketPath}
}

func (c *grpcClient) DaemonStatus(ctx context.Context) (DaemonSnapshot, error) {
	client, closer, err := c.connect(ctx)
	if err != nil {
		return DaemonSnapshot{}, err
	}
	defer closer()

	resp, err := client.GetStatus(ctx, &pb.GetStatusRequest{})
	if err != nil {
		return DaemonSnapshot{}, err
	}

	return DaemonSnapshot{
		Reachable:        true,
		Ready:            resp.GetReady(),
		RunningProcesses: resp.GetRunningProcesses(),
	}, nil
}

func (c *grpcClient) ListProcesses(ctx context.Context) (map[string]ProcessSnapshot, error) {
	client, closer, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer closer()

	resp, err := client.List(ctx, &pb.ListRequest{})
	if err != nil {
		return nil, err
	}

	processes := make(map[string]ProcessSnapshot, len(resp.GetProcesses()))
	for _, proc := range resp.GetProcesses() {
		processes[proc.GetName()] = ProcessSnapshot{
			Name:  proc.GetName(),
			State: processStateString(proc.GetState()),
		}
	}
	return processes, nil
}

func (c *grpcClient) connect(_ context.Context) (pb.ProcessManagerClient, func(), error) {
	if runtime.GOOS != "windows" {
		if _, err := os.Stat(c.socketPath); err != nil {
			return nil, nil, err
		}
	}

	conn, err := grpc.NewClient(
		grpcDialTarget(c.socketPath),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to dd-procmgrd: %w", err)
	}

	return pb.NewProcessManagerClient(conn), func() { _ = conn.Close() }, nil
}

func grpcDialTarget(socketPath string) string {
	if runtime.GOOS == "windows" {
		return "npipe://" + socketPath
	}
	return "unix://" + socketPath
}

func processStateString(state pb.ProcessState) string {
	switch state {
	case pb.ProcessState_UNKNOWN:
		return "Unknown"
	case pb.ProcessState_CREATED:
		return "Created"
	case pb.ProcessState_STARTING:
		return "Starting"
	case pb.ProcessState_RUNNING:
		return "Running"
	case pb.ProcessState_STOPPING:
		return "Stopping"
	case pb.ProcessState_STOPPED:
		return "Stopped"
	case pb.ProcessState_CRASHED:
		return "Crashed"
	case pb.ProcessState_EXITED:
		return "Exited"
	case pb.ProcessState_FAILED:
		return "Failed"
	default:
		return "Unknown"
	}
}
