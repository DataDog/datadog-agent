// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotecommand

import (
	"context"
	"io"
	"net"
	"testing"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	remoteagentregistryimpl "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/impl"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

type commandProviderFlowFixture struct {
	pb.UnimplementedRemoteCommandProviderServer
	provider  *pb.CommandProvider
	sessionID string
	requests  []*pb.ExecuteCommandRequest
}

func (p *commandProviderFlowFixture) ListCommands(context.Context, *pb.ListCommandsRequest) (*pb.ListCommandsResponse, error) {
	return &pb.ListCommandsResponse{Providers: []*pb.CommandProvider{p.provider}}, nil
}

func (p *commandProviderFlowFixture) ExecuteCommand(request *pb.ExecuteCommandRequest, stream grpc.ServerStreamingServer[pb.ExecuteCommandResponse]) error {
	if err := stream.SetHeader(metadata.Pairs("session_id", p.sessionID)); err != nil {
		return err
	}
	p.requests = append(p.requests, request)
	return stream.Send(&pb.ExecuteCommandResponse{Frame: &pb.ExecuteCommandResponse_ExitCode{ExitCode: 0}})
}

func TestCommandProviderRegistrationDiscoveryCLIAndExecutionFlow(t *testing.T) {
	registry, ipcComponent := newCommandProviderFlowRegistry(t)
	providerGroup := &pb.CommandProvider{
		Name:        "fixture-agent",
		Description: "Fixture remote command provider",
		Commands: []*pb.Command{{
			Name:      "diagnostics",
			ShortName: "diagnostics",
			Children: []*pb.Command{{
				Name:       "diagnostics.inspect",
				ShortName:  "inspect",
				IsRunnable: true,
				Parameters: []*pb.CommandParameter{{Name: "limit", ShortName: "l", Type: pb.ParameterType_TYPE_INT, IsFlag: true, Required: true}},
			}},
		}},
	}
	oldProvider := &commandProviderFlowFixture{provider: providerGroup}
	newProvider := &commandProviderFlowFixture{provider: providerGroup}
	registerCommandProviderFlowFixture(t, registry, ipcComponent, oldProvider, "100")
	registerCommandProviderFlowFixture(t, registry, ipcComponent, newProvider, "200")

	remote := Commands(&command.GlobalParams{})[0]
	require.NoError(t, AttachCommandProviders(remote, registry.ListCommands(context.Background()), func(providerName string, commandPath []string, arguments *structpb.Struct, _, _ io.Writer) error {
		return registry.ExecuteCommand(context.Background(), &pb.ExecuteCommandRequest{ProviderName: providerName, CommandPath: commandPath, Arguments: arguments}, func(*pb.ExecuteCommandResponse) error { return nil })
	}))

	providerCommand, _, err := remote.Find([]string{"fixture-agent"})
	require.NoError(t, err)
	require.Equal(t, "Fixture remote command provider", providerCommand.Short)
	remote.SetArgs([]string{"fixture-agent", "diagnostics", "inspect", "--limit", "7"})
	require.NoError(t, remote.Execute())
	require.Empty(t, oldProvider.requests)
	require.Len(t, newProvider.requests, 1)
	require.Equal(t, "fixture-agent", newProvider.requests[0].GetProviderName())
	require.Equal(t, []string{"diagnostics", "inspect"}, newProvider.requests[0].GetCommandPath())
	require.Equal(t, float64(7), newProvider.requests[0].GetArguments().GetFields()["limit"].GetNumberValue())
}

func newCommandProviderFlowRegistry(t *testing.T) (remoteagentregistry.Component, ipc.Component) {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("remote_agent.registry.enabled", true)
	ipcComponent := ipcmock.New(t)
	provides := remoteagentregistryimpl.NewComponent(remoteagentregistryimpl.Requires{
		Config:    cfg,
		Ipc:       ipcComponent,
		Lifecycle: compdef.NewTestLifecycle(t),
		Telemetry: telemetryimpl.NewMock(t),
		Secrets:   secretsmock.New(t),
	})
	require.NotNil(t, provides.Comp)
	return provides.Comp, ipcComponent
}

func registerCommandProviderFlowFixture(t *testing.T, registry remoteagentregistry.Component, ipcComponent ipc.Component, provider *commandProviderFlowFixture, pid string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(ipcComponent.GetTLSServerConfig())),
		grpc.ChainUnaryInterceptor(
			grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(ipcComponent.GetAuthToken())),
			func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
				if err := grpc.SetHeader(ctx, metadata.Pairs("session_id", provider.sessionID)); err != nil {
					return nil, err
				}
				return handler(ctx, req)
			},
		),
		grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(grpcutil.StaticAuthInterceptor(ipcComponent.GetAuthToken()))),
	)
	pb.RegisterRemoteCommandProviderServer(server, provider)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	sessionID, _, err := registry.RegisterRemoteAgent(&remoteagentregistry.RegistrationData{
		AgentFlavor:      "fixture-agent",
		AgentDisplayName: "Fixture Agent",
		AgentPID:         pid,
		APIEndpointURI:   listener.Addr().String(),
		Services:         []string{"datadog.remoteagent.command.v1.RemoteCommandProvider"},
	})
	require.NoError(t, err)
	provider.sessionID = sessionID
}
