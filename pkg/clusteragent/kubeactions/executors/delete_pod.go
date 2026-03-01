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

	// Validate resource_id is provided for UID safety check
	resourceID := resource.ResourceId
	if resourceID == "" {
		return ExecutionResult{
			Status:  "failed",
			Message: "resource_id is required for pod deletion (used for UID safety check)",
		}
	}

	// Get the pod first to verify UID matches resource_id
	pod, err := e.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Failed to get pod %s/%s: %v", namespace, name, err)
		return ExecutionResult{
			Status:  "failed",
			Message: fmt.Sprintf("failed to get pod: %v", err),
		}
	}

	if string(pod.UID) != resourceID {
		log.Errorf("Pod %s/%s UID mismatch: expected %s, got %s - pod may have been replaced", namespace, name, resourceID, pod.UID)
		return ExecutionResult{
			Status:  "failed",
			Message: fmt.Sprintf("pod UID mismatch: expected %s, got %s - pod may have been replaced since action was created", resourceID, pod.UID),
		}
	}

	// Get delete_pod specific parameters
	params := action.GetDeletePod()

	// Build delete options
	deleteOptions := metav1.DeleteOptions{}
	if params != nil && params.GracePeriodSeconds != nil {
		gracePeriod := *params.GracePeriodSeconds
		deleteOptions.GracePeriodSeconds = &gracePeriod
		log.Infof("Deleting pod %s/%s (uid=%s) with grace period %d seconds", namespace, name, resourceID, gracePeriod)
	} else {
		log.Infof("Deleting pod %s/%s (uid=%s) with default grace period", namespace, name, resourceID)
	}

	// Delete the pod
	err = e.clientset.CoreV1().Pods(namespace).Delete(ctx, name, deleteOptions)
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
