// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunnerimpl

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

// remediationCommandProvider trusts agent-authored commands and caller-supplied allowlists carried over the authenticated Remote Agent Registry.
type remediationCommandProvider struct {
	pbcore.UnimplementedRemoteCommandProviderServer
	run func(context.Context, *pb.RunLocalRemediationRequest) (*pb.RunLocalRemediationResponse, error)
}

func (p *remediationCommandProvider) ListCommands(context.Context, *pbcore.ListCommandsRequest) (*pbcore.ListCommandsResponse, error) {
	return &pbcore.ListCommandsResponse{Providers: []*pbcore.CommandProvider{{Name: "remediation", Description: "Run health-check remediation"}}}, nil
}

func (p *remediationCommandProvider) ExecuteCommand(request *pbcore.ExecuteCommandRequest, stream grpc.ServerStreamingServer[pbcore.ExecuteCommandResponse]) error {
	if request.GetProviderName() != "remediation" {
		return status.Error(codes.NotFound, "unknown command provider")
	}
	encoded := request.GetArguments().GetFields()["request"].GetStringValue()
	if encoded == "" {
		return status.Error(codes.InvalidArgument, "missing remediation request")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid base64 remediation request")
	}
	remediationRequest := &pb.RunLocalRemediationRequest{}
	if err := proto.Unmarshal(data, remediationRequest); err != nil {
		return status.Error(codes.InvalidArgument, "invalid remediation request")
	}
	response, err := p.run(stream.Context(), remediationRequest)
	if err != nil {
		return err
	}
	data, err = proto.Marshal(response)
	if err != nil {
		return status.Error(codes.Internal, "failed to encode remediation response")
	}
	if err := stream.Send(&pbcore.ExecuteCommandResponse{Frame: &pbcore.ExecuteCommandResponse_BinaryOutput{BinaryOutput: data}}); err != nil {
		return err
	}
	var exitCode int32
	if len(response.GetSteps()) != len(remediationRequest.GetCommands()) {
		exitCode = 1
	}
	for _, step := range response.GetSteps() {
		if step.GetError() != "" || step.GetExitCode() != 0 {
			exitCode = 1
			break
		}
	}
	return stream.Send(&pbcore.ExecuteCommandResponse{Frame: &pbcore.ExecuteCommandResponse_ExitCode{ExitCode: exitCode}})
}

// remediationLifecycle follows PAR's lifecycle, including its non-Fx cluster-agent embed.
type remediationLifecycle struct {
	hooks     []compdef.Hook
	forceStop func()
}

func (l *remediationLifecycle) Append(hook compdef.Hook) {
	l.hooks = append(l.hooks, hook)
}

func (l *remediationLifecycle) stop(ctx context.Context) error {
	if l.forceStop != nil {
		stop := context.AfterFunc(ctx, l.forceStop)
		defer stop()
	}
	var err error
	for i := len(l.hooks) - 1; i >= 0; i-- {
		if l.hooks[i].OnStop != nil {
			err = errors.Join(err, l.hooks[i].OnStop(ctx))
		}
	}
	l.hooks = nil
	return err
}

func (p *PrivateActionRunner) startRemediationRemoteAgent() error {
	if !p.coreConfig.GetBool("remote_agent.registry.enabled") || !p.coreConfig.GetBool("health_check_remediation.enabled") {
		return nil
	}
	address := net.JoinHostPort(p.coreConfig.GetString("cmd_host"), strconv.Itoa(p.coreConfig.GetInt("cmd_port")))
	server, err := helper.NewUnimplementedRemoteAgentServer(p.ipc, p.logger, p.coreConfig, &p.remediationLifecycle, address, "private-action-runner", "Private Action Runner")
	if err != nil {
		return err
	}
	p.remediationLifecycle.forceStop = server.GetGRPCServer().Stop
	pbcore.RegisterRemoteCommandProviderServer(server.GetGRPCServer(), &remediationCommandProvider{run: p.workflowRunner.RunLocalRemediation})
	server.Start()
	return nil
}
