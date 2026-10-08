// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package healthcheck

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	registrymock "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	corepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type remediationTestRegistry struct {
	remoteagentregistry.Component
	requests []*corepb.ExecuteCommandRequest
	frames   []*corepb.ExecuteCommandResponse
	err      error
}

func (r *remediationTestRegistry) ExecuteCommand(_ context.Context, request *corepb.ExecuteCommandRequest, send func(*corepb.ExecuteCommandResponse) error) error {
	r.requests = append(r.requests, request)
	for _, frame := range r.frames {
		if err := send(frame); err != nil {
			return err
		}
	}
	return r.err
}

func TestRegistryDispatcher(t *testing.T) {
	for _, test := range []struct {
		name              string
		err               error
		steps             []*pb.RemediationStepResult
		exitCode          int32
		outcome           string
		command           string
		framesBeforeError int
	}{
		{name: "success", steps: []*pb.RemediationStepResult{{Stdout: "sensitive-output"}}, outcome: "remediated"},
		{name: "failure", steps: []*pb.RemediationStepResult{{ExitCode: 1, Error: "sensitive-error", Stderr: "sensitive-stderr"}}, exitCode: 1, outcome: "escalate"},
		{name: "step failure with zero command exit", steps: []*pb.RemediationStepResult{{ExitCode: 1}}, outcome: "escalate"},
		{name: "command failure", steps: []*pb.RemediationStepResult{{}}, exitCode: 1, outcome: "escalate"},
		{name: "incomplete", outcome: "escalate"},
		{name: "absent provider", err: status.Error(codes.NotFound, "absent"), outcome: "dry-run"},
		{name: "unavailable provider", err: status.Error(codes.Unavailable, "unavailable"), outcome: "escalate"},
		{name: "unavailable after response", err: status.Error(codes.Unavailable, "stream dropped"), steps: []*pb.RemediationStepResult{{}}, outcome: "escalate", framesBeforeError: 1},
		{name: "unavailable after exit", err: status.Error(codes.Unavailable, "stream dropped"), steps: []*pb.RemediationStepResult{{}}, outcome: "escalate", framesBeforeError: 2},
		{name: "old provider", err: status.Error(codes.Unimplemented, "not supported"), outcome: "dry-run"},
		{name: "invalid policy", outcome: "escalate", command: "$COMMAND"},
		{name: "execution deadline", err: status.Error(codes.DeadlineExceeded, "timed out"), outcome: "escalate"},
		{name: "execution rejected", err: status.Error(codes.PermissionDenied, "denied"), outcome: "escalate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := configmock.New(t)
			config.SetInTest("health_check_remediation.enabled", true)
			payload, err := proto.Marshal(&pb.RunLocalRemediationResponse{Steps: test.steps})
			require.NoError(t, err)
			registry := &remediationTestRegistry{Component: registrymock.Mock(t), err: test.err}
			if test.err == nil {
				registry.frames = []*corepb.ExecuteCommandResponse{binaryFrame(payload), exitFrame(test.exitCode)}
			} else if test.framesBeforeError > 0 {
				registry.frames = []*corepb.ExecuteCommandResponse{binaryFrame(payload), exitFrame(test.exitCode)}[:test.framesBeforeError]
			}
			out := make(chan event.Event, 4)
			dispatcher := NewRemediationDispatcher(config, out, "agent-host", option.New[remoteagentregistry.Component](registry))
			cfg := healthConfig()
			cfg.Remediation.Steps = []integration.RemediationStep{{Command: "echo hello"}}
			if test.command != "" {
				cfg.Remediation.Steps[0].Command = test.command
			}
			dispatcher.Dispatch(context.Background(), "check:123", cfg.ServiceCheck, "scratch full", cfg)
			if test.outcome == "dry-run" || test.command != "" {
				require.Len(t, out, 1)
			} else {
				require.Len(t, out, 2)
				detected := <-out
				assert.Equal(t, "health-check remediation (detected)", detected.Title)
				assert.Contains(t, detected.Text, "scratch full")
				assert.Contains(t, detected.Text, "echo hello")
				assert.Equal(t, event.AlertTypeWarning, detected.AlertType)
			}
			last := <-out
			assert.NotContains(t, last.Text, "sensitive")
			assert.Equal(t, "health-check remediation ("+test.outcome+")", last.Title)
			assert.Equal(t, "agent-host", last.Host)
			assert.Equal(t, remediationSource, last.SourceTypeName)
			assert.Equal(t, "health_check_remediation:check:123", last.AggregationKey)
			assert.Contains(t, last.Tags, "remediation:"+test.outcome)
			assert.Contains(t, last.Tags, "check_id:check:123")
			assert.Contains(t, last.Tags, "service_check:"+cfg.ServiceCheck)
			if test.err != nil && test.outcome == "escalate" {
				assert.Equal(t, "Local remediation did not return a confirmed outcome; steps may have executed.", last.Text)
			}
			if test.command == "" {
				require.Len(t, registry.requests, 1)
				command := registry.requests[0]
				assert.Equal(t, "remediation", command.ProviderName)
				assert.Empty(t, command.CommandPath)
				payload, err := base64.StdEncoding.DecodeString(command.Arguments.Fields["request"].GetStringValue())
				require.NoError(t, err)
				request := &pb.RunLocalRemediationRequest{}
				require.NoError(t, proto.Unmarshal(payload, request))
				assert.Equal(t, []string{"echo hello"}, request.Commands)
				assert.Equal(t, []string{"rshell:echo"}, request.Allowlist.AllowedCommands)
			} else {
				assert.Empty(t, registry.requests)
			}
		})
	}
}

func TestRegistryDispatcherInvalidResponse(t *testing.T) {
	payload, err := proto.Marshal(&pb.RunLocalRemediationResponse{Steps: []*pb.RemediationStepResult{{}}})
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		frames []*corepb.ExecuteCommandResponse
	}{
		{name: "no frames"},
		{name: "missing exit", frames: []*corepb.ExecuteCommandResponse{binaryFrame(payload)}},
		{name: "missing response", frames: []*corepb.ExecuteCommandResponse{exitFrame(0)}},
		{name: "malformed response", frames: []*corepb.ExecuteCommandResponse{binaryFrame([]byte{0xff}), exitFrame(0)}},
		{name: "duplicate response", frames: []*corepb.ExecuteCommandResponse{binaryFrame(payload), binaryFrame(payload), exitFrame(0)}},
		{name: "duplicate exit", frames: []*corepb.ExecuteCommandResponse{binaryFrame(payload), exitFrame(0), exitFrame(0)}},
		{name: "nil frame", frames: []*corepb.ExecuteCommandResponse{nil}},
		{name: "unexpected stdout", frames: []*corepb.ExecuteCommandResponse{{Frame: &corepb.ExecuteCommandResponse_Stdout{Stdout: "sensitive-output"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := &remediationTestRegistry{Component: registrymock.Mock(t), frames: test.frames}
			out := make(chan event.Event, 2)
			dispatcher := &RegistryDispatcher{registry: registry, fallback: NewEventDispatcher(out, "host")}
			cfg := healthConfig()
			cfg.Remediation.Steps = []integration.RemediationStep{{Command: "echo hello"}}
			dispatcher.Dispatch(context.Background(), "check:123", "health", "", cfg)
			require.Len(t, registry.requests, 1)
			require.Len(t, out, 2)
			assert.Contains(t, (<-out).Tags, "remediation:detected")
			last := <-out
			assert.Contains(t, last.Tags, "remediation:escalate")
			assert.NotContains(t, last.Text, "sensitive")
		})
	}
}

func TestRegistryDispatcherOptionalRegistry(t *testing.T) {
	for _, test := range []struct {
		name     string
		registry option.Option[remoteagentregistry.Component]
	}{
		{name: "empty option", registry: option.None[remoteagentregistry.Component]()},
		{name: "nil component", registry: option.New[remoteagentregistry.Component](nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := configmock.New(t)
			out := make(chan event.Event, 2)
			assert.Nil(t, NewRemediationDispatcher(config, out, "host", test.registry))
			config.SetInTest("health_check_remediation.enabled", true)
			dispatcher := NewRemediationDispatcher(config, out, "host", test.registry)
			dispatcher.Dispatch(context.Background(), "check:123", "health", "", healthConfig())
			require.Len(t, out, 1)
			assert.Contains(t, (<-out).Tags, "remediation:dry-run")
			config.SetInTest("health_check_remediation.execution_mode", "local")
			assert.IsType(t, &LocalExecDispatcher{}, NewRemediationDispatcher(config, out, "host", test.registry))
		})
	}
}

func binaryFrame(payload []byte) *corepb.ExecuteCommandResponse {
	return &corepb.ExecuteCommandResponse{Frame: &corepb.ExecuteCommandResponse_BinaryOutput{BinaryOutput: payload}}
}

func exitFrame(code int32) *corepb.ExecuteCommandResponse {
	return &corepb.ExecuteCommandResponse{Frame: &corepb.ExecuteCommandResponse_ExitCode{ExitCode: code}}
}
