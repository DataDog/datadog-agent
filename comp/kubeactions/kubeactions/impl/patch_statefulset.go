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

// PatchStatefulSetExecutor executes patch_statefulset actions.
type PatchStatefulSetExecutor struct {
	clientset kubernetes.Interface
}

// NewPatchStatefulSetExecutor creates a new PatchStatefulSetExecutor.
func NewPatchStatefulSetExecutor(clientset kubernetes.Interface) *PatchStatefulSetExecutor {
	return &PatchStatefulSetExecutor{clientset: clientset}
}

// Execute applies a patch to a statefulset using the specified strategy.
func (e *PatchStatefulSetExecutor) Execute(ctx context.Context, in kubeactions.PatchStatefulSetInputs) kubeactions.ExecutionResult {
	namespace := in.Namespace
	name := in.Name

	if len(in.Patch) == 0 {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: "patch is required for patch_statefulset action",
		}
	}

	// Get the statefulset first to verify UID matches resource_id.
	statefulset, err := e.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to get statefulset: %v", err),
		}
	}

	// resource_id is an optional UID guard: enforce only when supplied.
	if in.ResourceID != "" && string(statefulset.UID) != in.ResourceID {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("statefulset UID mismatch: expected %s, got %s - statefulset may have been replaced since action was created", in.ResourceID, statefulset.UID),
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
	if _, err := e.clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, patchType, patch, metav1.PatchOptions{}); err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to patch statefulset: %v", err),
		}
	}

	return kubeactions.ExecutionResult{
		Status:  kubeactions.StatusSuccess,
		Message: fmt.Sprintf("statefulset %s/%s patched", namespace, name),
	}
}
