// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"sync"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

type grpcClient struct {
	socketPath string
}

func newGRPCClient(socketPath string) Client {
	return &grpcClient{socketPath: socketPath}
}

func (c *grpcClient) Connect(_ context.Context) (ProcmgrSession, error) {
	pm, closer, err := c.connect()
	if err != nil {
		return nil, err
	}
	return &grpcSession{pm: pm, closer: closer}, nil
}

type grpcSession struct {
	pm     pb.ProcessManagerClient
	closer func()
	once   sync.Once
}

func (s *grpcSession) Status(ctx context.Context) (DaemonSnapshot, error) {
	resp, err := s.pm.GetStatus(ctx, &pb.GetStatusRequest{})
	if err != nil {
		return DaemonSnapshot{}, err
	}
	return DaemonSnapshot{
		Reachable:        true,
		Ready:            resp.GetReady(),
		RunningProcesses: resp.GetRunningProcesses(),
	}, nil
}

func (s *grpcSession) List(ctx context.Context) (map[string]ProcessSnapshot, error) {
	resp, err := s.pm.List(ctx, &pb.ListRequest{})
	if err != nil {
		return nil, err
	}
	return processesFromListResponse(resp), nil
}

func (s *grpcSession) Disconnect() error {
	s.once.Do(func() {
		if s.closer != nil {
			s.closer()
			s.closer = nil
		}
	})
	return nil
}

func processesFromListResponse(resp *pb.ListResponse) map[string]ProcessSnapshot {
	processes := make(map[string]ProcessSnapshot, len(resp.GetProcesses()))
	for _, proc := range resp.GetProcesses() {
		processes[proc.GetName()] = ProcessSnapshot{
			Name:  proc.GetName(),
			State: proc.GetState(),
		}
	}
	return processes
}

func (c *grpcClient) connect() (pb.ProcessManagerClient, func(), error) {
	conn, err := dialProcmgrGRPC(c.socketPath)
	if err != nil {
		return nil, nil, err
	}

	return pb.NewProcessManagerClient(conn), func() { _ = conn.Close() }, nil
}
