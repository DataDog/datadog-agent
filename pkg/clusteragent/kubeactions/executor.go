// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"
	"fmt"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"k8s.io/client-go/kubernetes"
)

// ExecutorRegistry manages action executors
type ExecutorRegistry struct {
	executors map[string]ActionExecutor
	clientset kubernetes.Interface
}

// NewExecutorRegistry creates a new ExecutorRegistry
func NewExecutorRegistry(clientset kubernetes.Interface) *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make(map[string]ActionExecutor),
		clientset: clientset,
	}
}

// Register registers an executor for a specific action type
func (r *ExecutorRegistry) Register(actionType string, executor ActionExecutor) {
	r.executors[actionType] = executor
}

// GetExecutor returns the executor for a given action type
func (r *ExecutorRegistry) GetExecutor(actionType string) (ActionExecutor, error) {
	executor, exists := r.executors[actionType]
	if !exists {
		return nil, fmt.Errorf("no executor registered for action type: %s", actionType)
	}
	return executor, nil
}

// Execute executes an action using the appropriate executor
func (r *ExecutorRegistry) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	executor, err := r.GetExecutor(action.ActionType)
	if err != nil {
		return ExecutionResult{
			Status:  "failed",
			Message: err.Error(),
		}
	}

	return executor.Execute(ctx, action)
}

// GetClientset returns the Kubernetes clientset
func (r *ExecutorRegistry) GetClientset() kubernetes.Interface {
	return r.clientset
}
