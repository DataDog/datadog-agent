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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// PatchDeploymentExecutor executes patch deployment actions
type PatchDeploymentExecutor struct {
	clientset kubernetes.Interface
}

// NewPatchDeploymentExecutor creates a new PatchDeploymentExecutor
func NewPatchDeploymentExecutor(clientset kubernetes.Interface) *PatchDeploymentExecutor {
	return &PatchDeploymentExecutor{
		clientset: clientset,
	}
}

// resolvePatchType maps a patch strategy string to the corresponding Kubernetes patch type.
// Defaults to strategic merge if unspecified or unrecognized.
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

// Execute applies a patch to a deployment using the specified strategy
func (e *PatchDeploymentExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := resource.Namespace
	name := resource.Name
	resourceID := resource.ResourceId

	// Get patch params
	patchParams := action.GetPatchDeployment()
	if patchParams == nil || patchParams.GetPatch() == nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: "patch is required for patch_deployment action",
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

	log.Infof("Patching deployment %s/%s (uid=%s) with patch: %s", namespace, name, resourceID, string(patchBytes))

	// Get the deployment first to verify UID matches resource_id
	deployment, err := e.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Failed to get deployment %s/%s: %v", namespace, name, err)
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to get deployment: %v", err),
		}
	}

	if string(deployment.UID) != resourceID {
		log.Errorf("Deployment %s/%s UID mismatch: expected %s, got %s - deployment may have been replaced", namespace, name, resourceID, deployment.UID)
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("deployment UID mismatch: expected %s, got %s - deployment may have been replaced since action was created", resourceID, deployment.UID),
		}
	}

	// Determine patch strategy
	patchType := resolvePatchType(patchParams.GetPatchStrategy())
	log.Infof("Using patch strategy %q for deployment %s/%s", patchParams.GetPatchStrategy(), namespace, name)

	// Apply the patch
	_, err = e.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, patchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		log.Errorf("Failed to patch deployment %s/%s: %v", namespace, name, err)
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to patch deployment: %v", err),
		}
	}

	log.Infof("Successfully patched deployment %s/%s", namespace, name)
	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("deployment %s/%s patched", namespace, name),
	}
}
