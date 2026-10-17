// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotereporterimpl

import (
	"context"
	"net"
	"testing"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// fakeCoreAgent is an AgentSecure server recording the health issue RPCs it receives.
type fakeCoreAgent struct {
	pbcore.UnimplementedAgentSecureServer
	reports  chan *pbcore.ReportHealthIssueRequest
	resolves chan *pbcore.ResolveHealthIssueRequest
}

func (f *fakeCoreAgent) ReportHealthIssue(_ context.Context, req *pbcore.ReportHealthIssueRequest) (*emptypb.Empty, error) {
	f.reports <- req
	return &emptypb.Empty{}, nil
}

func (f *fakeCoreAgent) ResolveHealthIssue(_ context.Context, req *pbcore.ResolveHealthIssueRequest) (*emptypb.Empty, error) {
	f.resolves <- req
	return &emptypb.Empty{}, nil
}

// startFakeCoreAgent serves a fakeCoreAgent with the TLS and auth token checks
// of the core agent, and returns it with its port.
func startFakeCoreAgent(t *testing.T, ipc *ipcmock.IPCMock) (*fakeCoreAgent, string) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer(grpcutil.ServerOptionsWithMetricsAndAuth(
		grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(ipc.GetAuthToken())),
		nil,
		grpc.Creds(credentials.NewTLS(ipc.GetTLSServerConfig())),
	)...)
	agent := &fakeCoreAgent{
		reports:  make(chan *pbcore.ReportHealthIssueRequest, 10),
		resolves: make(chan *pbcore.ResolveHealthIssueRequest, 10),
	}
	pbcore.RegisterAgentSecureServer(server, agent)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	return agent, port
}

func TestNewComponentDeliversToCoreAgent(t *testing.T) {
	ipc := ipcmock.New(t)
	agent, port := startFakeCoreAgent(t, ipc)

	lc := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{
		Lifecycle: lc,
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			"health_platform.enabled": true,
			"cmd_host":                "127.0.0.1",
			"cmd_port":                port,
		}),
		IPC: ipc,
		Log: logmock.New(t),
	})
	require.NoError(t, err)
	require.NoError(t, lc.Start(context.Background()))

	require.NoError(t, provides.Comp.ReportIssue(testIssue("a", "title")))
	select {
	case req := <-agent.reports:
		assert.Equal(t, "a", req.GetIssue().GetId())
		assert.Equal(t, "title", req.GetIssue().GetTitle())
		assert.Empty(t, req.GetRemoteAgentSessionId())
	case <-time.After(10 * time.Second):
		require.FailNow(t, "issue was not reported to the core agent")
	}

	provides.Comp.ResolveIssue("a")
	select {
	case req := <-agent.resolves:
		assert.Equal(t, "a", req.GetIssueId())
	case <-time.After(10 * time.Second):
		require.FailNow(t, "issue was not resolved in the core agent")
	}

	require.NoError(t, lc.Stop(context.Background()))
}

func TestNewComponentFallsBackToNoop(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
	}{
		{
			name:      "health platform disabled",
			overrides: map[string]interface{}{"health_platform.enabled": false},
		},
		{
			name: "core agent client cannot be created",
			overrides: map[string]interface{}{
				"health_platform.enabled": true,
				"vsock_addr":              "invalid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := compdef.NewTestLifecycle(t)
			provides, err := NewComponent(Requires{
				Lifecycle: lc,
				Config:    config.NewMockWithOverrides(t, tt.overrides),
				IPC:       ipcmock.New(t),
				Log:       logmock.New(t),
			})
			require.NoError(t, err)
			assert.IsType(t, noopReporter{}, provides.Comp)
			lc.AssertHooksNumber(0)
		})
	}
}
