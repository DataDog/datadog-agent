// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ScaleDeploymentVerticallyHandler struct {
	c kubernetes.Interface
}

func NewScaleDeploymentVerticallyHandler() *ScaleDeploymentVerticallyHandler {
	return &ScaleDeploymentVerticallyHandler{}
}

type ContainerResourceUpdate struct {
	ContainerName string                   `json:"containerName,omitempty"`
	Requests      *ContainerResourceValues `json:"requests,omitempty"`
	Limits        *ContainerResourceValues `json:"limits,omitempty"`
}

type ContainerResourceValues struct {
	CPU    *string `json:"cpu,omitempty"`    // e.g., "100m", "0.5", "1"
	Memory *string `json:"memory,omitempty"` // e.g., "128Mi", "1Gi", "512M"
}

type ScaleDeploymentVerticallyInputs struct {
	Namespace        string                    `json:"namespace,omitempty"`
	Name             string                    `json:"name,omitempty"`
	ContainerUpdates []ContainerResourceUpdate `json:"containerUpdates,omitempty"`
}

type ScaleDeploymentVerticallyOutputs struct {
	ObjectMeta metav1.ObjectMeta   `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec       v1.DeploymentSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status     v1.DeploymentStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (h *ScaleDeploymentVerticallyHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[ScaleDeploymentVerticallyInputs](task)
	if err != nil {
		return nil, err
	}

	if len(inputs.ContainerUpdates) == 0 {
		return nil, errors.New("containerUpdates cannot be empty")
	}

	client, err := h.getClient(credential)
	if err != nil {
		return nil, err
	}

	// First, get the current deployment to validate container names
	deployment, err := client.AppsV1().Deployments(inputs.Namespace).Get(ctx, inputs.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Create patch object
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{},
				},
			},
		},
	}

	containers := []interface{}{}

	// Process each container update
	for _, containerUpdate := range inputs.ContainerUpdates {
		if containerUpdate.ContainerName == "" {
			return nil, errors.New("containerName cannot be empty")
		}

		// Validate that the container exists in the deployment
		containerFound := false
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == containerUpdate.ContainerName {
				containerFound = true
				break
			}
		}

		if !containerFound {
			return nil, fmt.Errorf("container %s not found in deployment %s", containerUpdate.ContainerName, inputs.Name)
		}

		containerPatch := map[string]interface{}{
			"name": containerUpdate.ContainerName,
		}

		// Build resources patch
		resources := map[string]interface{}{}

		if containerUpdate.Requests != nil {
			requests := map[string]interface{}{}
			if containerUpdate.Requests.CPU != nil {
				if _, err := resource.ParseQuantity(*containerUpdate.Requests.CPU); err != nil {
					return nil, fmt.Errorf("invalid CPU request format %s: %w", *containerUpdate.Requests.CPU, err)
				}
				requests["cpu"] = *containerUpdate.Requests.CPU
			}
			if containerUpdate.Requests.Memory != nil {
				if _, err := resource.ParseQuantity(*containerUpdate.Requests.Memory); err != nil {
					return nil, fmt.Errorf("invalid memory request format %s: %w", *containerUpdate.Requests.Memory, err)
				}
				requests["memory"] = *containerUpdate.Requests.Memory
			}
			if len(requests) > 0 {
				resources["requests"] = requests
			}
		}

		if containerUpdate.Limits != nil {
			limits := map[string]interface{}{}
			if containerUpdate.Limits.CPU != nil {
				if _, err := resource.ParseQuantity(*containerUpdate.Limits.CPU); err != nil {
					return nil, fmt.Errorf("invalid CPU limit format %s: %w", *containerUpdate.Limits.CPU, err)
				}
				limits["cpu"] = *containerUpdate.Limits.CPU
			}
			if containerUpdate.Limits.Memory != nil {
				if _, err := resource.ParseQuantity(*containerUpdate.Limits.Memory); err != nil {
					return nil, fmt.Errorf("invalid memory limit format %s: %w", *containerUpdate.Limits.Memory, err)
				}
				limits["memory"] = *containerUpdate.Limits.Memory
			}
			if len(limits) > 0 {
				resources["limits"] = limits
			}
		}

		if len(resources) > 0 {
			containerPatch["resources"] = resources
		}

		containers = append(containers, containerPatch)
	}

	patch["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"] = containers

	body, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	response, err := client.AppsV1().Deployments(inputs.Namespace).Patch(ctx, inputs.Name, typesv1.StrategicMergePatchType, body, metav1.PatchOptions{})
	if err != nil {
		return nil, err
	}

	return &ScaleDeploymentVerticallyOutputs{
		ObjectMeta: response.ObjectMeta,
		Spec:       response.Spec,
		Status:     response.Status,
	}, nil
}

func (h *ScaleDeploymentVerticallyHandler) getClient(credential *privateconnection.PrivateCredentials) (kubernetes.Interface, error) {
	if h.c != nil {
		return h.c, nil
	}
	return support.KubeClient(credential)
}
