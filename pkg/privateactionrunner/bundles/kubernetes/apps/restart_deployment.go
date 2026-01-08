// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"encoding/json"
	"time"

	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RestartDeploymentHandler struct{}

func NewRestartDeploymentHandler() *RestartDeploymentHandler {
	return &RestartDeploymentHandler{}
}

type RestartDeploymentInputs struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

type RestartDeploymentOutputs struct {
	ObjectMeta metav1.ObjectMeta   `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.DeploymentSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status     v1.DeploymentStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (h *RestartDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[RestartDeploymentInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	// Kubernetes a several strategies for patch updates https://kubernetes.io/docs/tasks/manage-kubernetes-objects/update-api-object-kubectl-patch/#use-a-json-merge-patch-to-update-a-deployment
	// We are using the merge patch strategy here (see https://stackoverflow.com/a/57480206)
	body, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						// just patch with a new date https://github.com/kubernetes/kubectl/blob/release-1.16/pkg/polymorphichelpers/objectrestarter.go#L41
						"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	response, err := client.AppsV1().Deployments(inputs.Namespace).Patch(ctx, inputs.Name, typesv1.MergePatchType, body, metav1.PatchOptions{})
	if err != nil {
		return nil, err
	}

	return &RestartDeploymentOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
		Status:     response.Status,
	}, nil
}
