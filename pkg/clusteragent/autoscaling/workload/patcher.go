// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PatcherAdapter allows a workload patcher to access and modify data available in controller store
type PatcherAdapter interface {
	// GetPodAutoscalerFromOwnerRef returns the PodAutoscalerInternal object associated with the given owner reference
	// NOTE: The returned PodAutoscalerInternal should never be modified
	GetRecommendations(ns string, ownerRef metav1.OwnerReference) (string, []datadoghq.DatadogPodAutoscalerContainerResources, error)
}

type patcherAdapter struct {
	store *store
}

var _ PatcherAdapter = patcherAdapter{}

func newPatcherAdapter(store *store) PatcherAdapter {
	return patcherAdapter{store: store}
}

// GetPodAutoscalerFromOwnerRef searches for a PodAutoscalerInternal object associated with the given owner reference
// If no PodAutoscalerInternal is found or no vertical recommendation, it returns ("", nil, nil)
func (pa patcherAdapter) GetRecommendations(ns string, ownerRef metav1.OwnerReference) (string, []datadoghq.DatadogPodAutoscalerContainerResources, error) {
	// TODO: Implementation is slow
	podAutoscalers := pa.store.GetFiltered(func(podAutoscaler model.PodAutoscalerInternal) bool {
		if podAutoscaler.Namespace == ns &&
			podAutoscaler.Spec.TargetRef.Name == ownerRef.Name &&
			podAutoscaler.Spec.TargetRef.Kind == ownerRef.Kind &&
			podAutoscaler.Spec.TargetRef.APIVersion == ownerRef.APIVersion {
			return true
		}
		return false
	})

	if len(podAutoscalers) == 0 {
		return "", nil, nil
	}

	if len(podAutoscalers) > 1 {
		return "", nil, fmt.Errorf("Multiple Pod Autoscalers found for %s/%s/%s", ns, ownerRef.Kind, ownerRef.Name)
	}

	if podAutoscalers[0].ScalingValues.Vertical != nil {
		return podAutoscalers[0].ScalingValues.Vertical.ResourcesHash, podAutoscalers[0].ScalingValues.Vertical.ContainerResources, nil
	}

	return "", nil, nil
}
