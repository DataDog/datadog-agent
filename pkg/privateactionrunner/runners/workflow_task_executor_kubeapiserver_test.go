// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

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
)

func TestWorkflowTaskExecutorRejectsAuthoredScriptOnClusterAgent(t *testing.T) {
	const (
		publicBundle = "com.datadoghq.helm"
		actionName   = "addRepo"
		executionFQN = "com.datadoghq.authoredscripts.helm.addRepo"
	)
	registry, err := privatebundles.NewRegistry(&config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, registry.GetBundle(publicBundle))
	require.Nil(t, registry.GetBundle("com.datadoghq.authoredscripts.helm"))

	executor := &WorkflowTaskExecutor{
		registry: registry,
		config: &config.Config{
			ActionsAllowlist: map[string]sets.Set[string]{publicBundle: sets.New(actionName)},
			MetricsClient:    &statsd.NoOpClient{},
		},
	}
	task := newWorkflowTask("task-id", publicBundle, actionName, "job-id")
	task.Data.Attributes.ExecutionFQN = executionFQN

	output, err := executor.RunTask(context.Background(), &PreparedWorkflowTask{Task: task})
	require.ErrorContains(t, err, "could not find bundle for com.datadoghq.authoredscripts.helm")
	assert.Nil(t, output)
}
