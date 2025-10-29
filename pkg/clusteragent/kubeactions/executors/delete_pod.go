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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ExecutionResult represents the result of executing an action
type ExecutionResult struct {
	Status  string // "success", "failed", "skipped"
	Message string
}

// DeletePodExecutor executes delete pod actions
type DeletePodExecutor struct {
	clientset kubernetes.Interface
}

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

	// Validate namespace is provided for pod deletion
	if namespace == "" {
		return ExecutionResult{
			Status:  "failed",
			Message: "namespace is required for pod deletion",
		}
	}

	log.Infof("Deleting pod %s/%s", namespace, name)

	// Delete the pod
	err := e.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		log.Errorf("Failed to delete pod %s/%s: %v", namespace, name, err)
		return ExecutionResult{
			Status:  "failed",
			Message: fmt.Sprintf("failed to delete pod: %v", err),
		}
	}

	log.Infof("Successfully deleted pod %s/%s", namespace, name)
	return ExecutionResult{
		Status:  "success",
		Message: fmt.Sprintf("pod %s/%s deleted", namespace, name),
	}
}
