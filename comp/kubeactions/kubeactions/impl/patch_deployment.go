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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

// PatchDeploymentExecutor executes patch_deployment actions.
type PatchDeploymentExecutor struct {
	clientset kubernetes.Interface
}

// NewPatchDeploymentExecutor creates a new PatchDeploymentExecutor.
func NewPatchDeploymentExecutor(clientset kubernetes.Interface) *PatchDeploymentExecutor {
	return &PatchDeploymentExecutor{clientset: clientset}
}

// resolvePatchType maps a patch strategy string to the corresponding Kubernetes
// patch type. Defaults to strategic merge if unspecified or unrecognized.
func resolvePatchType(strategy string) types.PatchType {
	switch strategy {
	case "merge":
		return types.MergePatchType
	case "json":
		return types.JSONPatchType
	default:
		return types.StrategicMergePatchType
	}
}

// Execute applies a patch to a deployment using the specified strategy.
func (e *PatchDeploymentExecutor) Execute(ctx context.Context, in kubeactions.PatchDeploymentInputs) kubeactions.ExecutionResult {
	namespace := in.Namespace
	name := in.Name

	if len(in.Patch) == 0 {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: "patch is required for patch_deployment action",
		}
	}

	// Get the deployment first to verify UID matches resource_id.
	deployment, err := e.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to get deployment: %v", err),
		}
	}

	// resource_id is an optional UID guard: enforce only when supplied.
	if in.ResourceID != "" && string(deployment.UID) != in.ResourceID {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("deployment UID mismatch: expected %s, got %s - deployment may have been replaced since action was created", in.ResourceID, deployment.UID),
		}
	}

	patchType := resolvePatchType(in.PatchStrategy)
	if _, err := e.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, patchType, in.Patch, metav1.PatchOptions{}); err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to patch deployment: %v", err),
		}
	}

	return kubeactions.ExecutionResult{
		Status:  kubeactions.StatusSuccess,
		Message: fmt.Sprintf("deployment %s/%s patched", namespace, name),
	}
}
