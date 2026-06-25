// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	"k8s.io/client-go/kubernetes"
)

// ExecutorRegistry dispatches helm actions to the appropriate executor.
type ExecutorRegistry struct {
	executors map[string]ActionExecutor
}

// NewExecutorRegistry creates an empty ExecutorRegistry.
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make(map[string]ActionExecutor),
	}
}

// Register registers an executor for a given action type.
func (r *ExecutorRegistry) Register(actionType string, executor ActionExecutor) {
	r.executors[actionType] = executor
}

// Execute dispatches action to its registered executor.
func (r *ExecutorRegistry) Execute(ctx context.Context, action *HelmAction) ExecutionResult {
	actionType := GetActionType(action)
	executor, exists := r.executors[actionType]
	if !exists {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("no executor registered for action type: %s", actionType),
		}
	}
	return executor.Execute(ctx, action)
}

// rollbackActionExecutor adapts RollbackExecutor to ActionExecutor.
type rollbackActionExecutor struct {
	exec *RollbackExecutor
}

func newRollbackActionExecutor(clientset kubernetes.Interface) *rollbackActionExecutor {
	return &rollbackActionExecutor{exec: NewRollbackExecutor(clientset)}
}

func (e *rollbackActionExecutor) Execute(ctx context.Context, action *HelmAction) ExecutionResult {
	p := action.Rollback
	opts := helmactions.RollbackInputs{
		Release:               p.Release,
		ReleaseNamespace:      p.ReleaseNamespace,
		Revision:              p.Revision,
		JobNamespace:          p.JobNamespace,
		JobServiceAccountName: p.ServiceAccount,
		Image:                 p.Image,
		Driver:                p.Driver,
	}

	job, err := e.exec.Run(ctx, opts)
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("helm rollback job creation failed: %v", err),
		}
	}
	return ExecutionResult{
		Status: StatusSuccess,
		Message: fmt.Sprintf("helm rollback job %s/%s created for release %s/%s (revision=%d)",
			job.Namespace, job.Name, p.ReleaseNamespace, p.Release, p.Revision),
	}
}
