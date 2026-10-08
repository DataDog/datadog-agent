// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package healthcheck

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

type remediationTestServer struct {
	pb.UnimplementedExecutorServer
	ready    bool
	request  chan *pb.RunLocalRemediationRequest
	response *pb.RunLocalRemediationResponse
	err      error
}

func (s *remediationTestServer) Health(context.Context, *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Ready: s.ready}, nil
}

func (s *remediationTestServer) RunLocalRemediation(_ context.Context, request *pb.RunLocalRemediationRequest) (*pb.RunLocalRemediationResponse, error) {
	s.request <- request
	return s.response, s.err
}

func TestPARDispatcherRPC(t *testing.T) {
	for _, test := range []struct {
		name    string
		ready   bool
		err     error
		steps   []*pb.RemediationStepResult
		outcome string
		command string
	}{
		{name: "success", ready: true, steps: []*pb.RemediationStepResult{{Stdout: "sensitive-output"}}, outcome: "remediated"},
		{name: "failure", ready: true, steps: []*pb.RemediationStepResult{{ExitCode: 1, Error: "sensitive-error", Stderr: "sensitive-stderr"}}, outcome: "escalate"},
		{name: "incomplete", ready: true, outcome: "escalate"},
		{name: "unready", outcome: "dry-run"},
		{name: "old executor", ready: true, err: status.Error(codes.Unimplemented, "not supported"), outcome: "dry-run"},
		{name: "invalid policy", ready: true, outcome: "escalate", command: "$COMMAND"},
		{name: "execution deadline", ready: true, err: status.Error(codes.DeadlineExceeded, "timed out"), outcome: "escalate"},
		{name: "uncertain execution", ready: true, err: status.Error(codes.Unavailable, "connection lost"), outcome: "escalate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := configmock.New(t)
			config.SetInTest("health_check_remediation.enabled", true)
			config.SetInTest("ipc_cert_file_path", filepath.Join(t.TempDir(), "ipc-cert.pem"))
			_, serverTLS, _, err := cert.FetchOrCreateIPCCert(context.Background(), config)
			require.NoError(t, err)
			dir, err := os.MkdirTemp("", "hcr-")
			require.NoError(t, err)
			t.Cleanup(func() { os.RemoveAll(dir) })
			address := filepath.Join(dir, "executor.sock")
			if runtime.GOOS == "windows" {
				address = `\\.\pipe\` + filepath.Base(dir)
			}
			listener, err := executor.Listen(address)
			require.NoError(t, err)
			serverTLS.ClientAuth = tls.RequireAndVerifyClientCert
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
			service := &remediationTestServer{request: make(chan *pb.RunLocalRemediationRequest, 1), ready: test.ready, response: &pb.RunLocalRemediationResponse{Steps: test.steps}, err: test.err}
			pb.RegisterExecutorServer(server, service)
			stopped := make(chan struct{})
			go func() { defer close(stopped); _ = server.Serve(listener) }()
			t.Cleanup(func() { server.Stop(); <-stopped })
			config.SetInTest("private_action_runner.executor.socket_path", address)
			out := make(chan event.Event, 4)
			dispatcher := NewRemediationDispatcher(config, out, "agent-host")
			cfg := healthConfig()
			cfg.Remediation.Steps = []integration.RemediationStep{{Command: "echo hello"}}
			if test.command != "" {
				cfg.Remediation.Steps[0].Command = test.command
			}
			dispatcher.Dispatch(context.Background(), "check:123", cfg.ServiceCheck, "scratch full", cfg)
			require.NotEmpty(t, out)
			if test.outcome == "dry-run" {
				require.Len(t, out, 1)
			}
			var last event.Event
			for len(out) > 0 {
				last = <-out
				assert.NotContains(t, last.Text, "sensitive")
			}
			assert.Equal(t, "health-check remediation ("+test.outcome+")", last.Title)
			assert.Equal(t, "agent-host", last.Host)
			assert.Contains(t, last.Tags, "remediation:"+test.outcome)
			if test.ready && test.command == "" {
				require.Len(t, service.request, 1)
				request := <-service.request
				assert.Equal(t, []string{"echo hello"}, request.Commands)
				assert.Equal(t, []string{"rshell:echo"}, request.Allowlist.AllowedCommands)
			} else {
				assert.Empty(t, service.request)
			}
		})
	}
}

func TestPARDispatcherUnreachable(t *testing.T) {
	config := configmock.New(t)
	out := make(chan event.Event, 2)
	assert.Nil(t, NewRemediationDispatcher(config, out, "host"))
	config.SetInTest("health_check_remediation.enabled", true)
	config.SetInTest("ipc_cert_file_path", filepath.Join(t.TempDir(), "ipc-cert.pem"))
	_, _, _, err := cert.FetchOrCreateIPCCert(context.Background(), config)
	require.NoError(t, err)
	config.SetInTest("private_action_runner.executor.socket_path", filepath.Join(t.TempDir(), "absent.sock"))
	dispatcher := NewRemediationDispatcher(config, out, "host")
	dispatcher.Dispatch(context.Background(), "check:123", "health", "", healthConfig())
	require.Len(t, out, 1)
	assert.Contains(t, (<-out).Tags, "remediation:dry-run")
}
