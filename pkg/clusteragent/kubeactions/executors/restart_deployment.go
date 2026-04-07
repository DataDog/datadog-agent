// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"context"
	"fmt"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RestartDeploymentExecutor executes restart deployment actions
type RestartDeploymentExecutor struct {
	clientset kubernetes.Interface
}

// NewRestartDeploymentExecutor creates a new RestartDeploymentExecutor
func NewRestartDeploymentExecutor(clientset kubernetes.Interface) *RestartDeploymentExecutor {
	return &RestartDeploymentExecutor{
		clientset: clientset,
	}
}

// Execute restarts a deployment by updating its restart annotation
func (e *RestartDeploymentExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := resource.Namespace
	name := resource.Name
	resourceID := resource.ResourceId

	log.Infof("Restarting deployment %s/%s (uid=%s)", namespace, name, resourceID)

	// Get the deployment and verify UID matches resource_id
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

	// Add or update the restart annotation
	if deployment.Spec.Template.ObjectMeta.Annotations == nil {
		deployment.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	// Update the deployment
	_, err = e.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("Failed to restart deployment %s/%s: %v", namespace, name, err)
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to restart deployment: %v", err),
		}
	}

	log.Infof("Successfully restarted deployment %s/%s", namespace, name)
	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("deployment %s/%s restarted", namespace, name),
	}
}
