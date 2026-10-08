// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package runners

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	rshell "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/remoteaction/rshell"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestLocalRemediationUsesSharedExecutionWithoutChangingSigning(t *testing.T) {
	const bundle = "com.datadoghq.remoteaction.rshell"
	dir := filepath.ToSlash(t.TempDir())
	cfg := &config.Config{
		OrgId: 42, RunnerId: "runner",
		MetricsClient:                  &statsd.NoOpClient{},
		ActionsAllowlist:               map[string]sets.Set[string]{bundle: sets.New("runRemediationCommand")},
		RShellAllowedPaths:             []string{dir + ":rw"},
		RShellAllowedCommands:          []string{"rshell:echo"},
		RShellDisableDetailedTelemetry: true,
	}
	signed := &WorkflowTaskExecutor{
		config: cfg, taskVerifier: taskverifier.NewTaskVerifier(nil, cfg),
		registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{bundle: rshell.NewRshellBundle(cfg)}},
	}
	local := signed.ForLocalRemediation()
	task := newWorkflowTask("local", bundle, "runRemediationCommand", "job")
	filename := dir + "/created"
	task.Data.Attributes.Inputs = map[string]interface{}{"command": fmt.Sprintf("echo remediated > %q", filename)}
	task.Data.Attributes.SystemInputs = &pb.SystemInputs{Input: &pb.SystemInputs_RemoteAction{RemoteAction: &pb.RemoteAction{
		AllowedPaths: []string{dir + ":rw"}, AllowedCommands: []string{"rshell:echo"},
	}}}
	prepared, _, err := local.PrepareTask(context.Background(), task)
	require.NoError(t, err)
	require.Nil(t, prepared.Credential)
	require.Equal(t, "runner", prepared.Task.Data.Attributes.ConnectionInfo.RunnerId)
	output, err := local.RunPrepared(context.Background(), prepared)
	require.NoError(t, err)
	require.Zero(t, output.(*rshell.RunCommandOutputs).ExitCode)
	require.FileExists(t, filename)
	_, _, err = signed.PrepareTask(context.Background(), task)
	require.ErrorContains(t, err, "missing signed envelope")
	cfg.ActionsAllowlist = nil
	_, err = local.RunPrepared(context.Background(), prepared)
	require.ErrorContains(t, err, "not allowlisted")
}

func TestLocalRemediationDoesNotRequireSignedRegistryBundle(t *testing.T) {
	const bundle = "com.datadoghq.remoteaction.rshell"
	cfg := &config.Config{
		OrgId: 42, RunnerId: "runner", MetricsClient: &statsd.NoOpClient{},
		ActionsAllowlist:      map[string]sets.Set[string]{bundle: sets.New("runRemediationCommand")},
		RShellAllowedCommands: []string{"rshell:echo"}, RShellDisableDetailedTelemetry: true,
	}
	signed := &WorkflowTaskExecutor{config: cfg, registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{}}}
	local := signed.ForLocalRemediation()
	task := newWorkflowTask("local", bundle, "runRemediationCommand", "job")
	task.Data.Attributes.Inputs = map[string]interface{}{"command": "echo remediated"}
	task.Data.Attributes.SystemInputs = &pb.SystemInputs{Input: &pb.SystemInputs_RemoteAction{RemoteAction: &pb.RemoteAction{
		AllowedCommands: []string{"rshell:echo"},
	}}}
	prepared, _, err := local.PrepareTask(context.Background(), task)
	require.NoError(t, err)
	output, err := local.RunPrepared(context.Background(), prepared)
	require.NoError(t, err)
	require.Equal(t, "remediated\n", output.(*rshell.RunCommandOutputs).Stdout)
	require.Empty(t, signed.registry.Bundles)
}
