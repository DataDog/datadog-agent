// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && (linux || darwin || windows)

package privateactionrunnerimpl

import (
	"context"
	"encoding/base64"
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/util/sets"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type remediationStream struct {
	grpc.ServerStream
	frames []*pbcore.ExecuteCommandResponse
}

func (s *remediationStream) Context() context.Context { return context.Background() }
func (s *remediationStream) Send(frame *pbcore.ExecuteCommandResponse) error {
	s.frames = append(s.frames, frame)
	return nil
}

type remediationRCClient struct{ rcclient.Client }

func (remediationRCClient) Subscribe(string, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

func TestRemediationCommandProvider(t *testing.T) {
	workflow, err := runners.NewWorkflowRunner(&parconfig.Config{
		OrgId: 42, RunnerId: "test-runner", MetricsClient: &statsd.NoOpClient{},
		ActionsAllowlist:      map[string]sets.Set[string]{"com.datadoghq.remoteaction.rshell": sets.New("runRemediationCommand")},
		RShellAllowedCommands: []string{"rshell:echo"}, RShellDisableDetailedTelemetry: true,
	}, remediationRCClient{}, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	provider := &remediationCommandProvider{run: workflow.RunLocalRemediation}
	commands, err := provider.ListCommands(context.Background(), &pbcore.ListCommandsRequest{})
	require.NoError(t, err)
	require.Len(t, commands.Providers, 1)
	require.Equal(t, "remediation", commands.Providers[0].Name)

	for _, allowed := range []bool{true, false} {
		request := &pb.RunLocalRemediationRequest{Commands: []string{"echo remediated"}, Allowlist: &pb.RemediationAllowlist{}}
		if allowed {
			request.Allowlist.AllowedCommands = []string{"rshell:echo"}
		}
		data, err := proto.Marshal(request)
		require.NoError(t, err)
		stream := &remediationStream{}
		err = provider.ExecuteCommand(&pbcore.ExecuteCommandRequest{
			ProviderName: "remediation",
			Arguments: &structpb.Struct{Fields: map[string]*structpb.Value{
				"request": structpb.NewStringValue(base64.StdEncoding.EncodeToString(data)),
			}},
		}, stream)
		require.NoError(t, err)
		require.Len(t, stream.frames, 2)
		require.IsType(t, &pbcore.ExecuteCommandResponse_BinaryOutput{}, stream.frames[0].Frame)
		require.IsType(t, &pbcore.ExecuteCommandResponse_ExitCode{}, stream.frames[1].Frame)
		response := &pb.RunLocalRemediationResponse{}
		require.NoError(t, proto.Unmarshal(stream.frames[0].GetBinaryOutput(), response))
		require.Len(t, response.Steps, 1)
		if allowed {
			require.Empty(t, response.Steps[0].Error)
			require.Equal(t, "remediated", response.Steps[0].Stdout)
			require.Zero(t, response.Steps[0].ExitCode)
			require.Zero(t, stream.frames[1].GetExitCode())
		} else {
			require.True(t, response.Steps[0].Error != "" || response.Steps[0].ExitCode != 0)
			require.NotZero(t, stream.frames[1].GetExitCode())
		}
	}
}

func TestRemediationCommandProviderInvalidRequest(t *testing.T) {
	provider := &remediationCommandProvider{run: func(context.Context, *pb.RunLocalRemediationRequest) (*pb.RunLocalRemediationResponse, error) {
		t.Fatal("invalid request must not execute")
		return nil, nil
	}}
	for _, encoded := range []string{"", "invalid base64", base64.StdEncoding.EncodeToString([]byte{0xff})} {
		stream := &remediationStream{}
		err := provider.ExecuteCommand(&pbcore.ExecuteCommandRequest{
			ProviderName: "remediation",
			Arguments: &structpb.Struct{Fields: map[string]*structpb.Value{
				"request": structpb.NewStringValue(encoded),
			}},
		}, stream)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Empty(t, stream.frames)
	}
	require.Equal(t, codes.InvalidArgument, status.Code(provider.ExecuteCommand(&pbcore.ExecuteCommandRequest{ProviderName: "remediation"}, &remediationStream{})))
	require.Equal(t, codes.NotFound, status.Code(provider.ExecuteCommand(&pbcore.ExecuteCommandRequest{ProviderName: "other"}, &remediationStream{})))
}

func TestRemediationRegistrationGates(t *testing.T) {
	for _, registryEnabled := range []bool{false, true} {
		for _, remediationEnabled := range []bool{false, true} {
			ipc := ipcmock.New(t)
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"remote_agent.registry.enabled":       registryEnabled,
				"health_check_remediation.enabled":    remediationEnabled,
				"remote_agent.registry.query_timeout": 100 * time.Millisecond,
				"cmd_host":                            "127.0.0.1", "cmd_port": 0,
			})
			runner := &PrivateActionRunner{coreConfig: cfg, ipc: ipc, logger: logmock.New(t), workflowRunner: &runners.WorkflowRunner{}}
			require.NoError(t, runner.startRemediationRemoteAgent())
			if registryEnabled && remediationEnabled {
				require.Len(t, runner.remediationLifecycle.hooks, 1)
			} else {
				require.Empty(t, runner.remediationLifecycle.hooks)
			}
			require.NoError(t, runner.remediationLifecycle.stop(context.Background()))
		}
	}
}

func TestRemediationShutdownCancelsActiveRPC(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })
	server := grpc.NewServer()
	t.Cleanup(server.Stop)
	active := make(chan struct{})
	canceled := make(chan struct{})
	pbcore.RegisterRemoteCommandProviderServer(server, &remediationCommandProvider{
		run: func(ctx context.Context, _ *pb.RunLocalRemediationRequest) (*pb.RunLocalRemediationResponse, error) {
			close(active)
			<-ctx.Done()
			close(canceled)
			return nil, ctx.Err()
		},
	})
	go func() { _ = server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///remediation", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	request, err := proto.Marshal(&pb.RunLocalRemediationRequest{Commands: []string{"echo remediated"}})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := pbcore.NewRemoteCommandProviderClient(conn).ExecuteCommand(ctx, &pbcore.ExecuteCommandRequest{
		ProviderName: "remediation",
		Arguments: &structpb.Struct{Fields: map[string]*structpb.Value{
			"request": structpb.NewStringValue(base64.StdEncoding.EncodeToString(request)),
		}},
	})
	require.NoError(t, err)
	select {
	case <-active:
	case <-ctx.Done():
		t.Fatal("remediation RPC did not start")
	}
	lifecycle := &remediationLifecycle{forceStop: server.Stop}
	stopping := make(chan struct{})
	lifecycle.Append(compdef.Hook{OnStop: func(context.Context) error {
		close(stopping)
		server.GracefulStop()
		return nil
	}})
	stopCtx, cancelStop := context.WithCancel(context.Background())
	defer cancelStop()
	stopped := make(chan error, 1)
	go func() { stopped <- lifecycle.stop(stopCtx) }()
	select {
	case <-stopping:
	case <-ctx.Done():
		t.Fatal("graceful shutdown did not start")
	}
	cancelStop()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("shutdown did not honor context cancellation")
	}
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal("active remediation was not canceled")
	}
	_, err = stream.Recv()
	require.Error(t, err)
}
