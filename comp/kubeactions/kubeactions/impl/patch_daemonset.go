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

// PatchDaemonSetExecutor executes patch_daemonset actions.
type PatchDaemonSetExecutor struct {
	clientset kubernetes.Interface
}

// NewPatchDaemonSetExecutor creates a new PatchDaemonSetExecutor.
func NewPatchDaemonSetExecutor(clientset kubernetes.Interface) *PatchDaemonSetExecutor {
	return &PatchDaemonSetExecutor{clientset: clientset}
}

// Execute applies a patch to a daemonset using the specified strategy.
func (e *PatchDaemonSetExecutor) Execute(ctx context.Context, in kubeactions.PatchDaemonSetInputs) kubeactions.ExecutionResult {
	namespace := in.Namespace
	name := in.Name

	if len(in.Patch) == 0 {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: "patch is required for patch_daemonset action",
		}
	}

	// Get the daemonset first to verify UID matches resource_id.
	daemonset, err := e.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to get daemonset: %v", err),
		}
	}

	// resource_id is an optional UID guard: enforce only when supplied.
	if in.ResourceID != "" && string(daemonset.UID) != in.ResourceID {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("daemonset UID mismatch: expected %s, got %s - daemonset may have been replaced since action was created", in.ResourceID, daemonset.UID),
		}
	}

	patchType := resolvePatchType(in.PatchStrategy)
	patch, err := applyUIDGuard(patchType, in.Patch, in.ResourceID)
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to build patch: %v", err),
		}
	}
	if _, err := e.clientset.AppsV1().DaemonSets(namespace).Patch(ctx, name, patchType, patch, metav1.PatchOptions{}); err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to patch daemonset: %v", err),
		}
	}

	return kubeactions.ExecutionResult{
		Status:  kubeactions.StatusSuccess,
		Message: fmt.Sprintf("daemonset %s/%s patched", namespace, name),
	}
}
