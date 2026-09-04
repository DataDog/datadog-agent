// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	remotecommand "github.com/DataDog/datadog-agent/cmd/agent/subcommands/remotecommand"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type commandProviderFixture struct {
	pb.UnimplementedRemoteCommandProviderServer
	provider  *pb.CommandProvider
	sessionID string
	requests  []*pb.ExecuteCommandRequest
}

func (p *commandProviderFixture) ListCommands(context.Context, *pb.ListCommandsRequest) (*pb.ListCommandsResponse, error) {
	return &pb.ListCommandsResponse{Providers: []*pb.CommandProvider{p.provider}}, nil
}

func (p *commandProviderFixture) ExecuteCommand(request *pb.ExecuteCommandRequest, stream grpc.ServerStreamingServer[pb.ExecuteCommandResponse]) error {
	if err := stream.SetHeader(metadata.Pairs("session_id", p.sessionID)); err != nil {
		return err
	}
	p.requests = append(p.requests, request)
	return stream.Send(&pb.ExecuteCommandResponse{Frame: &pb.ExecuteCommandResponse_ExitCode{ExitCode: 0}})
}

func withCommandProvider(provider *commandProviderFixture) mockProvider {
	return func(server *grpc.Server, remote *testRemoteAgentServer) {
		pb.RegisterRemoteCommandProviderServer(server, provider)
		remote.RegistrationData.Services = append(remote.RegistrationData.Services, CommandProviderServiceName)
	}
}

func registerCommandProvider(t *testing.T, registry *remoteAgentRegistry, ipcComponent ipc.Component, provider *commandProviderFixture, pid string) *testRemoteAgentServer {
	t.Helper()
	remote := buildRemoteAgent(t, ipcComponent, "fixture-agent", "Fixture Agent", pid, withCommandProvider(provider))
	sessionID, _, err := registry.RegisterRemoteAgent(&remote.RegistrationData)
	require.NoError(t, err)
	remote.registeredSessionID = sessionID
	provider.sessionID = sessionID
	return remote
}

func TestCommandProviderRegistrationDiscoveryCLIAndExecutionFlow(t *testing.T) {
	provides, _, _, _, ipcComponent := buildComponent(t)
	registry := provides.Comp.(*remoteAgentRegistry)
	commandTree := []*pb.Command{{
		Name:      "diagnostics",
		ShortName: "diagnostics",
		Children: []*pb.Command{{
			Name:       "diagnostics.inspect",
			ShortName:  "inspect",
			IsRunnable: true,
			Parameters: []*pb.CommandParameter{{Name: "limit", ShortName: "l", Type: pb.ParameterType_TYPE_INT, IsFlag: true, Required: true}},
		}},
	}}
	providerGroup := &pb.CommandProvider{Name: "fixture-agent", Description: "Fixture remote command provider", Commands: commandTree}
	oldestProvider := &commandProviderFixture{provider: providerGroup}
	newestProvider := &commandProviderFixture{provider: providerGroup}
	oldest := registerCommandProvider(t, registry, ipcComponent, oldestProvider, "100")
	_ = registerCommandProvider(t, registry, ipcComponent, newestProvider, "200")

	listed := registry.ListCommands(context.Background())
	require.Len(t, listed, 1)
	remote := remotecommand.Commands(&command.GlobalParams{})[0]
	require.NoError(t, remotecommand.AttachCommandProviders(remote, listed, func(providerName string, commandPath []string, arguments *structpb.Struct, _, _ io.Writer) error {
		return registry.ExecuteCommand(context.Background(), &pb.ExecuteCommandRequest{ProviderName: providerName, CommandPath: commandPath, Arguments: arguments}, func(*pb.ExecuteCommandResponse) error { return nil })
	}))

	providerCommand, _, err := remote.Find([]string{"fixture-agent"})
	require.NoError(t, err)
	require.Equal(t, "Fixture remote command provider", providerCommand.Short)
	remote.SetArgs([]string{"fixture-agent", "diagnostics", "inspect", "--limit", "7"})
	require.NoError(t, remote.Execute())
	require.Len(t, oldestProvider.requests, 1)
	require.Empty(t, newestProvider.requests)
	require.Equal(t, "fixture-agent", oldestProvider.requests[0].GetProviderName())
	require.Equal(t, []string{"diagnostics", "inspect"}, oldestProvider.requests[0].GetCommandPath())
	require.Equal(t, float64(7), oldestProvider.requests[0].GetArguments().GetFields()["limit"].GetNumberValue())

	registry.agentMapMu.Lock()
	delete(registry.agentMap, oldest.registeredSessionID)
	registry.agentMapMu.Unlock()
	require.NoError(t, remote.Execute())
	require.Len(t, newestProvider.requests, 1)
}
