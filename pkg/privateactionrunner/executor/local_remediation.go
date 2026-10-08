// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package executor

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	rshell "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/rshell"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const maxLocalRemediationSteps = 32

// RunLocalRemediation runs agent-authored remediation without a backend signature.
// Trust caveat (action-platform review item): the shared IPC cert authorizes any PAR-family
// client (incl. the control plane), not the health-check dispatcher specifically, so the
// boundary is "holds the IPC cert", not "agent-only". Residual risk is bounded: non-privileged
// rshell only, allowlist is default-deny intersected with the operator rshell restrictions, and
// the whole path is off unless health_check_remediation.enabled. A distinct remediation identity
// or executor-owned declarations would tighten this; deferred to action-platform.
func (s *Server) RunLocalRemediation(ctx context.Context, req *pb.RunLocalRemediationRequest) (*pb.RunLocalRemediationResponse, error) {
	if err := s.authorizeSharedIPC(ctx); err != nil {
		return nil, err
	}
	if s.localExecutor == nil {
		return nil, status.Error(codes.Unimplemented, "local remediation is disabled")
	}
	if !s.ready.Load() {
		return nil, status.Error(codes.Unavailable, "executor is not ready")
	}
	if len(req.GetCommands()) == 0 || len(req.GetCommands()) > maxLocalRemediationSteps {
		return nil, status.Error(codes.InvalidArgument, "local remediation requires between 1 and 32 steps")
	}
	policy := localRemediationPolicy(req.GetAllowlist())
	if err := taskverifier.ValidateLocalRemediationPolicy(policy); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	for _, command := range req.Commands {
		if strings.TrimSpace(command) == "" {
			return nil, status.Error(codes.InvalidArgument, "empty remediation command")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	s.startActivity()
	s.active.Add(1)
	defer func() { s.active.Add(-1); s.finishActivity() }()
	response := &pb.RunLocalRemediationResponse{}
	id := uuid.NewString()
	for i, command := range req.Commands {
		if err := ctx.Err(); err != nil {
			return nil, status.FromContextError(err).Err()
		}
		task := &types.Task{}
		task.Data.ID = fmt.Sprintf("local-remediation-%s-%d", id, i)
		task.Data.Type = "task"
		task.Data.Attributes = &types.Attributes{
			JobId:        task.Data.ID,
			BundleID:     "com.datadoghq.remoteaction.rshell",
			Name:         "runRemediationCommand",
			Inputs:       map[string]interface{}{"command": command},
			SystemInputs: &privateactionspb.SystemInputs{Input: &privateactionspb.SystemInputs_RemoteAction{RemoteAction: policy}},
		}
		result := &pb.RemediationStepResult{}
		response.Steps = append(response.Steps, result)
		prepared, _, err := s.localExecutor.PrepareTask(ctx, task)
		if err != nil {
			result.Error = "local remediation preparation failed"
			break
		}
		output, err := s.localExecutor.RunPrepared(ctx, prepared)
		if err != nil {
			result.Error = "local remediation execution failed"
			break
		}
		commandOutput, ok := output.(*rshell.RunCommandOutputs)
		if !ok || commandOutput == nil {
			result.Error = "unexpected local remediation output"
			break
		}
		result.ExitCode = int32(commandOutput.ExitCode)
		result.Stdout = scrubLocalOutput(commandOutput.Stdout)
		result.Stderr = scrubLocalOutput(commandOutput.Stderr)
		if result.ExitCode != 0 {
			break
		}
	}
	return response, nil
}

func localRemediationPolicy(allowlist *pb.RemediationAllowlist) *privateactionspb.RemoteAction {
	policy := &privateactionspb.RemoteAction{
		AllowedPaths:    allowlist.GetAllowedPaths(),
		AllowedCommands: allowlist.GetAllowedCommands(),
		SystemServices:  make(map[string]*structpb.ListValue),
	}
	for service, actions := range allowlist.GetAllowedServices() {
		values := &structpb.ListValue{}
		for _, action := range actions.GetActions() {
			values.Values = append(values.Values, structpb.NewStringValue(action))
		}
		policy.SystemServices[service] = values
	}
	return policy
}

func scrubLocalOutput(output string) string {
	scrubbed, err := scrubber.ScrubString(output)
	if err != nil {
		return "[output redacted]"
	}
	scrubbed = strings.ToValidUTF8(scrubbed, "\uFFFD")
	const limit = 4096
	if len(scrubbed) > limit {
		end := limit
		for !utf8.RuneStart(scrubbed[end]) {
			end--
		}
		return scrubbed[:end] + " [truncated]"
	}
	return scrubbed
}
