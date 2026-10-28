// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_watermarkpodautoscaler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DataDog/watermarkpodautoscaler/apis/datadoghq/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	if err := inputs.validate(); err != nil {
		return nil, err
	}

	client, err := h.getClient(credential)
	if err != nil {
		return nil, err
	}
	resource := client.Resource(watermarkPodAutoscalerResource).Namespace(inputs.Namespace)

	resourceVersion, err := h.validateAgainstExisting(ctx, resource, inputs)
	if err != nil {
		return nil, err
	}

	body, err := inputs.specPatch(resourceVersion)
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
		if apierrors.IsConflict(err) {
			return nil, fmt.Errorf("the watermark pod autoscaler was modified while this action was running, so the replica bounds were not applied; re-run the action: %w", err)
		}
		return nil, err
	}

	watermarkPodAutoscaler, err := decodeWatermarkPodAutoscaler(response)
	if err != nil {
		return nil, err
	}

	return &UpdateWatermarkAutoscalerLimitsOutputs{WatermarkPodAutoscaler: watermarkPodAutoscaler}, nil
}

// validate checks the bounds the caller supplied, without reading the cluster.
func (inputs UpdateWatermarkAutoscalerLimitsInputs) validate() error {
	if inputs.MinReplicas == nil && inputs.MaxReplicas == nil {
		return errors.New("at least one of minReplicas or maxReplicas must be provided")
	}
	for _, bound := range []struct {
		name  string
		value *int32
	}{
		{"minReplicas", inputs.MinReplicas},
		{"maxReplicas", inputs.MaxReplicas},
	} {
		if bound.value != nil && *bound.value < 1 {
			return fmt.Errorf("%s must be greater than or equal to 1", bound.name)
		}
	}
	return compareBounds(inputs.MinReplicas, inputs.MaxReplicas)
}

// validateAgainstExisting fills whichever bound the caller omitted from the live
// object and compares the pair. The CRD has no cross-field validation, so a
// single-bound patch would otherwise be able to persist maxReplicas below
// minReplicas. It returns the resourceVersion it read so the patch can be made
// conditional on the object not having moved in between; the empty string means
// nothing was read and no precondition is needed.
func (h *UpdateWatermarkAutoscalerLimitsHandler) validateAgainstExisting(
	ctx context.Context,
	resource dynamic.ResourceInterface,
	inputs UpdateWatermarkAutoscalerLimitsInputs,
) (string, error) {
	if inputs.MinReplicas != nil && inputs.MaxReplicas != nil {
		return "", nil
	}
	existing, err := h.getWatermarkPodAutoscaler(ctx, resource, inputs.Name)
	if err != nil {
		return "", err
	}
	minReplicas, maxReplicas := inputs.MinReplicas, inputs.MaxReplicas
	if minReplicas == nil {
		minReplicas = existing.Spec.MinReplicas
	}
	if maxReplicas == nil {
		maxReplicas = &existing.Spec.MaxReplicas
	}
	return existing.ResourceVersion, compareBounds(minReplicas, maxReplicas)
}

// compareBounds enforces minReplicas <= maxReplicas, and is a no-op while either
// bound is still unknown.
func compareBounds(minReplicas, maxReplicas *int32) error {
	if minReplicas == nil || maxReplicas == nil {
		return nil
	}
	if *minReplicas > *maxReplicas {
		return errors.New("minReplicas must be less than or equal to maxReplicas")
	}
	return nil
}

// specPatch builds the merge patch, carrying only the bounds that were supplied.
// A non-empty resourceVersion is included as an optimistic-concurrency
// precondition: the API server rejects the patch with 409 Conflict if the object
// changed after it was read, which keeps the single-bound comparison in
// validateAgainstExisting from being invalidated by a concurrent write.
func (inputs UpdateWatermarkAutoscalerLimitsInputs) specPatch(resourceVersion string) ([]byte, error) {
	spec := map[string]interface{}{}
	if inputs.MinReplicas != nil {
		spec["minReplicas"] = *inputs.MinReplicas
	}
	if inputs.MaxReplicas != nil {
		spec["maxReplicas"] = *inputs.MaxReplicas
	}
	patch := map[string]interface{}{"spec": spec}
	if resourceVersion != "" {
		patch["metadata"] = map[string]interface{}{"resourceVersion": resourceVersion}
	}
	return json.Marshal(patch)
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
