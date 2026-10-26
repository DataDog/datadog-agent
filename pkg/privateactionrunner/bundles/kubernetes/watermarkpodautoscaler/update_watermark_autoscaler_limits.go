// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_watermarkpodautoscaler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/DataDog/watermarkpodautoscaler/apis/datadoghq/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// watermarkPodAutoscalerResource identifies the WatermarkPodAutoscaler custom resource.
// The WPA is a CRD, so it is reached through the dynamic client rather than a typed one.
var watermarkPodAutoscalerResource = v1alpha1.GroupVersion.WithResource("watermarkpodautoscalers")

type UpdateWatermarkAutoscalerLimitsHandler struct {
	client dynamic.Interface
}

func NewUpdateWatermarkAutoscalerLimitsHandler() *UpdateWatermarkAutoscalerLimitsHandler {
	return &UpdateWatermarkAutoscalerLimitsHandler{}
}

type UpdateWatermarkAutoscalerLimitsInputs struct {
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name,omitempty"`
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

type UpdateWatermarkAutoscalerLimitsOutputs struct {
	WatermarkPodAutoscaler *v1alpha1.WatermarkPodAutoscaler `json:"watermarkPodAutoscaler"`
}

func (h *UpdateWatermarkAutoscalerLimitsHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[UpdateWatermarkAutoscalerLimitsInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.MinReplicas == nil && inputs.MaxReplicas == nil {
		return nil, errors.New("at least one of minReplicas or maxReplicas must be provided")
	}
	// A WatermarkPodAutoscaler cannot scale to zero: minReplicas is constrained to
	// Minimum=1 by the CRD, unlike a HorizontalPodAutoscaler which permits 0.
	if inputs.MinReplicas != nil && *inputs.MinReplicas < 1 {
		return nil, errors.New("minReplicas must be greater than or equal to 1")
	}
	if inputs.MaxReplicas != nil && *inputs.MaxReplicas < 1 {
		return nil, errors.New("maxReplicas must be greater than or equal to 1")
	}

	client, err := h.getClient(credential)
	if err != nil {
		return nil, err
	}
	resource := client.Resource(watermarkPodAutoscalerResource).Namespace(inputs.Namespace)

	// Both bounds have to be checked against each other, even when only one is
	// supplied. The CRD validates each field's minimum but has no cross-field
	// validation, so a single-bound patch can persist maxReplicas < minReplicas.
	// The WatermarkPodAutoscaler controller rejects such a resource through
	// CheckWPAValidity and stops autoscaling the target, so the bound that is not
	// being changed is read from the live object and validated first. A
	// HorizontalPodAutoscaler needs none of this: the API server validates the
	// pair for it.
	minReplicas, maxReplicas := inputs.MinReplicas, inputs.MaxReplicas
	if minReplicas == nil || maxReplicas == nil {
		existing, err := h.getWatermarkPodAutoscaler(ctx, resource, inputs.Name)
		if err != nil {
			return nil, err
		}
		if minReplicas == nil {
			minReplicas = existing.Spec.MinReplicas
		}
		if maxReplicas == nil {
			maxReplicas = &existing.Spec.MaxReplicas
		}
	}
	if minReplicas != nil && maxReplicas != nil && *minReplicas > *maxReplicas {
		return nil, errors.New("minReplicas must be less than or equal to maxReplicas")
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

	response, err := resource.Patch(
		ctx,
		inputs.Name,
		typesv1.MergePatchType,
		body,
		metav1.PatchOptions{},
	)
	if err != nil {
		return nil, err
	}

	watermarkPodAutoscaler, err := decodeWatermarkPodAutoscaler(response)
	if err != nil {
		return nil, err
	}

	return &UpdateWatermarkAutoscalerLimitsOutputs{WatermarkPodAutoscaler: watermarkPodAutoscaler}, nil
}

func (h *UpdateWatermarkAutoscalerLimitsHandler) getWatermarkPodAutoscaler(
	ctx context.Context,
	resource dynamic.ResourceInterface,
	name string,
) (*v1alpha1.WatermarkPodAutoscaler, error) {
	response, err := resource.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return decodeWatermarkPodAutoscaler(response)
}

func decodeWatermarkPodAutoscaler(response *unstructured.Unstructured) (*v1alpha1.WatermarkPodAutoscaler, error) {
	watermarkPodAutoscaler := &v1alpha1.WatermarkPodAutoscaler{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(response.Object, watermarkPodAutoscaler); err != nil {
		return nil, err
	}
	return watermarkPodAutoscaler, nil
}

func (h *UpdateWatermarkAutoscalerLimitsHandler) getClient(credential *privateconnection.PrivateCredentials) (dynamic.Interface, error) {
	if h.client != nil {
		return h.client, nil
	}
	return support.DynamicKubeClient(credential)
}
