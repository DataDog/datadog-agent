// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"fmt"

	autoscalingstore "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/store"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

// ScalingValuesProvider is a read-only view into the current scaling state of workloads managed
// by the DatadogPodAutoscaler controller. Other components (e.g. the external metrics provider)
// use it to query per-workload scaling values without depending on the store internals.
type ScalingValuesProvider interface {
	// TargetReplicas returns the raw backend recommendation for the DPA in namespace that targets
	// the given kind/name workload. Returns an error with a descriptive message on any failure.
	TargetReplicas(namespace, kind, name string) (int32, error)
}

type scalingValuesProvider struct {
	store *autoscalingstore.Store[model.PodAutoscalerInternal]
}

// NewScalingValuesProvider creates a ScalingValuesProvider backed by the given DPA store.
func NewScalingValuesProvider(store *autoscalingstore.Store[model.PodAutoscalerInternal]) ScalingValuesProvider {
	return &scalingValuesProvider{store: store}
}

func (s *scalingValuesProvider) TargetReplicas(namespace, kind, name string) (int32, error) {
	results := s.store.List(func(pai model.PodAutoscalerInternal) bool {
		spec := pai.Spec()
		return spec != nil &&
			pai.Namespace() == namespace &&
			spec.TargetRef.Kind == kind &&
			spec.TargetRef.Name == name
	})
	if len(results) == 0 {
		return 0, fmt.Errorf("no DatadogPodAutoscaler found targeting %s/%s/%s", namespace, kind, name)
	}
	pai := results[0]
	if !pai.IsHPAIntegrationEnabled() {
		return 0, fmt.Errorf("DatadogPodAutoscaler %s/%s does not have hpa-integration enabled", namespace, pai.Name())
	}
	if pai.ScalingValues().Horizontal == nil {
		return 0, fmt.Errorf("DatadogPodAutoscaler %s/%s has no horizontal recommendation yet", namespace, pai.Name())
	}
	return pai.ScalingValues().Horizontal.Replicas, nil
}
