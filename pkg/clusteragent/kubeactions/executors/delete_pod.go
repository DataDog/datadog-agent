// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"context"
	"fmt"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Execution status constants
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// ExecutionResult represents the result of executing an action
type ExecutionResult struct {
	Status   string
	Message  string
	Payloads map[string][]byte
}

// DeletePodExecutor executes delete pod actions
type DeletePodExecutor struct {
	clientset kubernetes.Interface
}

var _ Executor = (*DeletePodExecutor)(nil)

// NewDeletePodExecutor creates a new DeletePodExecutor
func NewDeletePodExecutor(clientset kubernetes.Interface) *DeletePodExecutor {
	return &DeletePodExecutor{
		clientset: clientset,
	}
}

// Execute deletes a pod
func (e *DeletePodExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := resource.Namespace
	name := resource.Name
	resourceID := resource.ResourceId

	// Get the pod first to verify UID matches resource_id
	pod, err := e.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to get pod: %v", err),
		}
	}

	if string(pod.UID) != resourceID {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("pod UID mismatch: expected %s, got %s - pod may have been replaced since action was created", resourceID, pod.UID),
		}
	}

	// Build delete options
	deleteOptions := metav1.DeleteOptions{}
	if params := action.GetDeletePod(); params != nil && params.GracePeriodSeconds != nil {
		gracePeriod := *params.GracePeriodSeconds
		deleteOptions.GracePeriodSeconds = &gracePeriod
	}

	if err := e.clientset.CoreV1().Pods(namespace).Delete(ctx, name, deleteOptions); err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to delete pod: %v", err),
		}
	}

	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("pod %s/%s deleted", namespace, name),
	}
}
