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

// PatchStatefulSetExecutor executes patch statefulset actions
type PatchStatefulSetExecutor struct {
	clientset kubernetes.Interface
}

var _ Executor = (*PatchStatefulSetExecutor)(nil)

// NewPatchStatefulSetExecutor creates a new PatchStatefulSetExecutor
func NewPatchStatefulSetExecutor(clientset kubernetes.Interface) *PatchStatefulSetExecutor {
	return &PatchStatefulSetExecutor{
		clientset: clientset,
	}
}

// Execute applies a patch to a statefulset using the specified strategy
func (e *PatchStatefulSetExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := resource.Namespace
	name := resource.Name
	resourceID := resource.ResourceId

	// Get patch params
	patchParams := action.GetPatchStatefulset()
	if patchParams == nil || patchParams.GetPatch() == nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: "patch is required for patch_statefulset action",
		}
	}

	// Marshal the protobuf Value back to JSON bytes for the Kubernetes API
	patchBytes, err := patchParams.GetPatch().MarshalJSON()
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to serialize patch to JSON: %v", err),
		}
	}

	// Get the statefulset first to verify UID matches resource_id
	statefulset, err := e.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to get statefulset: %v", err),
		}
	}

	if string(statefulset.UID) != resourceID {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("statefulset UID mismatch: expected %s, got %s - statefulset may have been replaced since action was created", resourceID, statefulset.UID),
		}
	}

	patchType := resolvePatchType(patchParams.GetPatchStrategy())
	if _, err := e.clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, patchType, patchBytes, metav1.PatchOptions{}); err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to patch statefulset: %v", err),
		}
	}

	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("statefulset %s/%s patched", namespace, name),
	}
}
