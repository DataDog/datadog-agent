// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && windows

package phonehome

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

type pipeSupervisor struct {
	pb.UnimplementedProcessManagerServer
}

func (pipeSupervisor) Describe(context.Context, *pb.DescribeRequest) (*pb.DescribeResponse, error) {
	return &pb.DescribeResponse{Detail: &pb.ProcessDetail{Uuid: "local-pipe"}}, nil
}

func TestSupervisorNamedPipe(t *testing.T) {
	t.Setenv("DD_PM_SOCKET_PATH", "")
	require.Equal(t, `\\.\pipe\datadog-procmgrd`, supervisorSocketPath())
	for _, path := range []string{"", "relative", `C:\socket`, `\\server\pipe\procmgr`, `\\.\pipe\`} {
		require.False(t, validSupervisorSocket(path), path)
	}
	path := fmt.Sprintf(`\\.\pipe\par-phonehome-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	require.True(t, validSupervisorSocket(path))
	listener, err := winio.ListenPipe(path, nil)
	require.NoError(t, err)
	server := grpc.NewServer()
	pb.RegisterProcessManagerServer(server, pipeSupervisor{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := dialSupervisor(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := pb.NewProcessManagerClient(conn).Describe(ctx, &pb.DescribeRequest{NameOrUuid: controlProcess})
	require.NoError(t, err)
	require.Equal(t, "local-pipe", response.GetDetail().GetUuid())
}
