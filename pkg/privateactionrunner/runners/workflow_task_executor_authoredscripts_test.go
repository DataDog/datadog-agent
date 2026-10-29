// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubeapiserver

package runners

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestWorkflowTaskExecutorRoutesNewPublicActionToAuthoredScript(t *testing.T) {
	const (
		publicBundle = "com.datadoghq.hosts"
		actionName   = "diskUtilization"
		scriptBundle = "com.datadoghq.authoredscripts"
		executionFQN = scriptBundle + ".hosts." + actionName
	)

	tests := []struct {
		name      string
		allowlist map[string]sets.Set[string]
		wantError string
	}{
		{
			name:      "public FQN is allowlisted",
			allowlist: map[string]sets.Set[string]{publicBundle: sets.New(actionName)},
		},
		{
			name:      "execution FQN cannot authorize public action",
			allowlist: map[string]sets.Set[string]{scriptBundle + ".hosts": sets.New(actionName)},
			wantError: "not allowlisted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scriptAction := &recordingAction{}
			scriptHandler := &fakeBundle{action: scriptAction}
			registry := &privatebundles.Registry{Bundles: map[string]types.Bundle{
				scriptBundle: scriptHandler,
			}}
			require.Nil(t, registry.GetBundle(publicBundle))
			executor := &WorkflowTaskExecutor{
				registry: registry,
				config: &config.Config{
					ActionsAllowlist: tt.allowlist,
					MetricsClient:    &statsd.NoOpClient{},
				},
			}
			task := newWorkflowTask("task-id", publicBundle, actionName, "job-id")
			task.Data.Attributes.ExecutionFQN = executionFQN

			output, err := executor.RunTask(context.Background(), &PreparedWorkflowTask{Task: task})
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				assert.Nil(t, output)
				assert.False(t, scriptAction.called)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "executed", output)
				assert.True(t, scriptAction.called)
				require.Same(t, task, scriptAction.task)
				assert.Equal(t, publicBundle+"."+actionName, scriptAction.task.GetFQN())
			}
			assert.Equal(t, actionName, scriptHandler.gotActionName)
		})
	}
}

func TestWorkflowTaskExecutorRejectsMismatchedAuthoredScript(t *testing.T) {
	const (
		publicBundle = "com.datadoghq.hosts"
		actionName   = "diskUtilization"
		scriptBundle = "com.datadoghq.authoredscripts"
	)
	scriptAction := &recordingAction{}
	executor := &WorkflowTaskExecutor{
		registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{
			scriptBundle: &fakeBundle{action: scriptAction},
		}},
		config: &config.Config{
			ActionsAllowlist: map[string]sets.Set[string]{publicBundle: sets.New(actionName)},
			MetricsClient:    &statsd.NoOpClient{},
		},
	}
	task := newWorkflowTask("task-id", publicBundle, actionName, "job-id")
	task.Data.Attributes.ExecutionFQN = scriptBundle + ".hosts.deleteDisk"

	output, err := executor.RunTask(context.Background(), &PreparedWorkflowTask{Task: task})
	require.ErrorContains(t, err, "does not match public action")
	assert.Nil(t, output)
	assert.False(t, scriptAction.called)
}
