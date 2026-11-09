// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

// RestartDeploymentExecutor executes restart_deployment actions.
type RestartDeploymentExecutor struct {
	clientset kubernetes.Interface
}

// NewRestartDeploymentExecutor creates a new RestartDeploymentExecutor.
func NewRestartDeploymentExecutor(clientset kubernetes.Interface) *RestartDeploymentExecutor {
	return &RestartDeploymentExecutor{clientset: clientset}
}

// Execute restarts a deployment by updating its restart annotation.
func (e *RestartDeploymentExecutor) Execute(ctx context.Context, in kubeactions.RestartDeploymentInputs) kubeactions.ExecutionResult {
	namespace := in.Namespace
	name := in.Name

	// Get the deployment and verify UID matches resource_id.
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

	// Add or update the restart annotation.
	if deployment.Spec.Template.ObjectMeta.Annotations == nil {
		deployment.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	if _, err := e.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to restart deployment: %v", err),
		}
	}

	return kubeactions.ExecutionResult{
		Status:  kubeactions.StatusSuccess,
		Message: fmt.Sprintf("deployment %s/%s restarted", namespace, name),
	}
}
