// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package runners

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	rshell "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/rshell"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

type sandboxExecutor struct {
	verifier taskverifier.TaskVerifier
	handler  *rshell.RunCommandHandler
	tasks    []*types.Task
}

func (e *sandboxExecutor) PrepareTask(_ context.Context, task *types.Task) (*PreparedWorkflowTask, *types.Task, error) {
	verified, err := e.verifier.UnwrapTask(task)
	if err != nil {
		return nil, task, err
	}
	e.tasks = append(e.tasks, verified)
	return &PreparedWorkflowTask{Task: verified}, nil, nil
}

func (e *sandboxExecutor) RunPrepared(ctx context.Context, prepared *PreparedWorkflowTask) (interface{}, error) {
	return e.handler.Run(ctx, prepared.Task, nil)
}

func TestLocalRemediationSandboxAndStop(t *testing.T) {
	for _, scenario := range []string{"success", "denied path", "denied command", "failed step"} {
		t.Run(scenario, func(t *testing.T) {
			dir := filepath.ToSlash(t.TempDir())
			first := dir + "/first"
			later := dir + "/later"
			core := &sandboxExecutor{
				verifier: taskverifier.NewLocalTrustVerifier(&config.Config{OrgId: 42, RunnerId: "local-runner"}),
				handler:  rshell.NewRunRemediationCommandHandler(rshell.RunCommandHandlerConfig{OperatorAllowedPaths: []string{dir + ":rw"}, OperatorAllowedCommands: []string{"rshell:echo", "rshell:false"}, DisableDetailedTelemetry: true}),
			}
			request := &pb.RunLocalRemediationRequest{
				Commands:  []string{fmt.Sprintf("echo remediated > %q", first), fmt.Sprintf("echo remediated > %q", later)},
				Allowlist: &pb.RemediationAllowlist{AllowedPaths: []string{dir + ":rw"}, AllowedCommands: []string{"rshell:echo", "rshell:false"}},
			}
			switch scenario {
			case "denied path":
				request.Allowlist.AllowedPaths = nil
			case "denied command":
				request.Allowlist.AllowedCommands = nil
			case "failed step":
				request.Commands[0] = "false"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			response, err := runLocalRemediation(ctx, core, request)
			require.NoError(t, err)
			if scenario == "success" {
				require.Len(t, response.Steps, 2)
				require.Empty(t, response.Steps[0].Error)
				require.Zero(t, response.Steps[0].ExitCode)
				require.FileExists(t, first)
				require.FileExists(t, later)
			} else {
				require.Len(t, response.Steps, 1)
				require.True(t, response.Steps[0].Error != "" || response.Steps[0].ExitCode != 0)
				_, err := os.Stat(later)
				require.True(t, os.IsNotExist(err))
			}
			require.NotEmpty(t, core.tasks)
			attrs := core.tasks[0].Data.Attributes
			require.Equal(t, "com.datadoghq.remoteaction.rshell", attrs.BundleID)
			require.Equal(t, "runRemediationCommand", attrs.Name)
			require.Equal(t, request.Commands[0], attrs.Inputs["command"])
			require.Equal(t, int64(42), attrs.OrgId)
			require.Equal(t, "local-runner", attrs.ConnectionInfo.RunnerId)
			require.Nil(t, attrs.SignedEnvelope)
			require.Nil(t, attrs.VerificationKey)
			require.NotContains(t, attrs.Inputs, "effectivePermissions")
		})
	}
}

func TestLocalRemediationInvalidRequest(t *testing.T) {
	for _, request := range []*pb.RunLocalRemediationRequest{nil, {}, {Commands: []string{" "}}, {Commands: make([]string, 33)}} {
		_, err := runLocalRemediation(context.Background(), nil, request)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}

func TestLocalRemediationOutputIsSafeForProtobuf(t *testing.T) {
	for _, output := range []string{strings.Repeat("a", 4095) + "€", string([]byte{0xff, 0xfe}), "api_key: 0123456789abcdef0123456789abcdef"} {
		scrubbed := scrubLocalOutput(output)
		require.True(t, utf8.ValidString(scrubbed))
		require.NotContains(t, scrubbed, "0123456789abcdef0123456789abcdef")
		_, err := proto.Marshal(&pb.RemediationStepResult{Stdout: scrubbed})
		require.NoError(t, err)
	}
	require.Equal(t, strings.Repeat("a", 4095)+" [truncated]", scrubLocalOutput(strings.Repeat("a", 4095)+"€"))
}
