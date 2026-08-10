// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

// DeletePodExecutor executes delete_pod actions.
type DeletePodExecutor struct {
	clientset kubernetes.Interface
}

// NewDeletePodExecutor creates a new DeletePodExecutor.
func NewDeletePodExecutor(clientset kubernetes.Interface) *DeletePodExecutor {
	return &DeletePodExecutor{clientset: clientset}
}

// Execute deletes a pod after verifying its UID matches the requested resource ID.
func (e *DeletePodExecutor) Execute(ctx context.Context, in kubeactions.DeletePodInputs) kubeactions.ExecutionResult {
	namespace := in.Namespace
	name := in.Name

	// Get the pod first to verify UID matches resource_id.
	pod, err := e.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to get pod: %v", err),
		}
	}

	// resource_id is an optional UID guard: enforce only when supplied.
	if in.ResourceID != "" && string(pod.UID) != in.ResourceID {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("pod UID mismatch: expected %s, got %s - pod may have been replaced since action was created", in.ResourceID, pod.UID),
		}
	}

	deleteOptions := metav1.DeleteOptions{}
	if in.GracePeriodSeconds != nil {
		gracePeriod := *in.GracePeriodSeconds
		deleteOptions.GracePeriodSeconds = &gracePeriod
	}

	// Close the check-then-delete race: the UID comparison above is against the
	// pod we Get'd, but the pod could be deleted and recreated with the same
	// name before the Delete below, so a name-only delete would remove the new
	// pod. A UID precondition makes the API server reject the delete atomically
	// if the current object's UID no longer matches. Only set it when the caller
	// supplied a UID guard (resource_id).
	if in.ResourceID != "" {
		uid := pod.UID
		deleteOptions.Preconditions = &metav1.Preconditions{UID: &uid}
	}

	if err := e.clientset.CoreV1().Pods(namespace).Delete(ctx, name, deleteOptions); err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to delete pod: %v", err),
		}
	}

	return kubeactions.ExecutionResult{
		Status:  kubeactions.StatusSuccess,
		Message: fmt.Sprintf("pod %s/%s deleted", namespace, name),
	}
}
