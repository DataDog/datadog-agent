// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package traceimpl

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type mockCoreAgent struct {
	pbcore.UnimplementedRemoteAgentServer
	registrations chan *pbcore.RegisterRemoteAgentRequest
}

func (m *mockCoreAgent) RegisterRemoteAgent(_ context.Context, req *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
	m.registrations <- req
	return &pbcore.RegisterRemoteAgentResponse{
		SessionId:                      "trace-session",
		RecommendedRefreshIntervalSecs: 60,
	}, nil
}

type mockStatusProvider struct {
	pbcore.UnimplementedStatusProviderServer
}

func TestNewComponentRegistersStatusProviderBeforeStart(t *testing.T) {
	ipc := ipcmock.New(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	coreAgent := &mockCoreAgent{registrations: make(chan *pbcore.RegisterRemoteAgentRequest, 1)}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(ipc.GetTLSServerConfig())))
	pbcore.RegisterRemoteAgentServer(server, coreAgent)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	config := coreconfig.NewMock(t)
	config.SetInTest("remote_agent.registry.enabled", true)
	config.SetInTest("remote_agent.registry.query_timeout", time.Second)
	config.SetInTest("cmd_host", host)
	config.SetInTest("cmd_port", port)

	lifecycle := compdef.NewTestLifecycle(t)
	_, err = NewComponent(Requires{
		Lifecycle: lifecycle,
		Log:       logmock.New(t),
		IPC:       ipc,
		Config:    config,
		Status:    &mockStatusProvider{},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, lifecycle.Stop(context.Background()))
	})

	select {
	case registration := <-coreAgent.registrations:
		assert.Contains(t, registration.Services, "datadog.remoteagent.status.v1.StatusProvider")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Trace Agent RAR registration")
	}
}
