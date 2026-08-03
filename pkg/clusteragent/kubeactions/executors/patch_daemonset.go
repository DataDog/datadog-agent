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

// PatchDaemonSetExecutor executes patch daemonset actions
type PatchDaemonSetExecutor struct {
	clientset kubernetes.Interface
}

var _ Executor = (*PatchDaemonSetExecutor)(nil)

// NewPatchDaemonSetExecutor creates a new PatchDaemonSetExecutor
func NewPatchDaemonSetExecutor(clientset kubernetes.Interface) *PatchDaemonSetExecutor {
	return &PatchDaemonSetExecutor{
		clientset: clientset,
	}
}

// Execute applies a patch to a daemonset using the specified strategy
func (e *PatchDaemonSetExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := resource.Namespace
	name := resource.Name
	resourceID := resource.ResourceId

	// Get patch params
	patchParams := action.GetPatchDaemonset()
	if patchParams == nil || patchParams.GetPatch() == nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: "patch is required for patch_daemonset action",
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

	// Get the daemonset first to verify UID matches resource_id
	daemonset, err := e.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to get daemonset: %v", err),
		}
	}

	if string(daemonset.UID) != resourceID {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("daemonset UID mismatch: expected %s, got %s - daemonset may have been replaced since action was created", resourceID, daemonset.UID),
		}
	}

	patchType := resolvePatchType(patchParams.GetPatchStrategy())
	if _, err := e.clientset.AppsV1().DaemonSets(namespace).Patch(ctx, name, patchType, patchBytes, metav1.PatchOptions{}); err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to patch daemonset: %v", err),
		}
	}

	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("daemonset %s/%s patched", namespace, name),
	}
}
