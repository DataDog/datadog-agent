// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"context"
	"encoding/json"
	"errors"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type UpdateAutoscalerLimitsHandler struct {
	client kubernetes.Interface
}

func NewUpdateAutoscalerLimitsHandler() *UpdateAutoscalerLimitsHandler {
	return &UpdateAutoscalerLimitsHandler{}
}

type UpdateAutoscalerLimitsInputs struct {
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name,omitempty"`
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

type UpdateAutoscalerLimitsOutputs struct {
	HorizontalPodAutoscaler *autoscalingv2.HorizontalPodAutoscaler `json:"horizontalPodAutoscaler"`
}

func (h *UpdateAutoscalerLimitsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[UpdateAutoscalerLimitsInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.MinReplicas == nil && inputs.MaxReplicas == nil {
		return nil, errors.New("at least one of minReplicas or maxReplicas must be provided")
	}
	if inputs.MinReplicas != nil && *inputs.MinReplicas < 0 {
		return nil, errors.New("minReplicas must be greater than or equal to 0")
	}
	if inputs.MaxReplicas != nil && *inputs.MaxReplicas < 1 {
		return nil, errors.New("maxReplicas must be greater than or equal to 1")
	}
	if inputs.MinReplicas != nil && inputs.MaxReplicas != nil && *inputs.MinReplicas > *inputs.MaxReplicas {
		return nil, errors.New("minReplicas must be less than or equal to maxReplicas")
	}

	client, err := h.getClient(credential)
	if err != nil {
		return nil, err
	}

	specPatch := map[string]interface{}{}
	if inputs.MinReplicas != nil {
		specPatch["minReplicas"] = *inputs.MinReplicas
	}
	if inputs.MaxReplicas != nil {
		specPatch["maxReplicas"] = *inputs.MaxReplicas
	}

	body, err := json.Marshal(map[string]interface{}{"spec": specPatch})
	if err != nil {
		return nil, err
	}

	response, err := client.AutoscalingV2().HorizontalPodAutoscalers(inputs.Namespace).Patch(
		ctx,
		inputs.Name,
		typesv1.MergePatchType,
		body,
		metav1.PatchOptions{},
	)
	if err != nil {
		return nil, err
	}

	return &UpdateAutoscalerLimitsOutputs{HorizontalPodAutoscaler: response}, nil
}

func (h *UpdateAutoscalerLimitsHandler) getClient(credential *privateconnection.PrivateCredentials) (kubernetes.Interface, error) {
	if h.client != nil {
		return h.client, nil
	}
	return support.KubeClient(credential)
}
