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
