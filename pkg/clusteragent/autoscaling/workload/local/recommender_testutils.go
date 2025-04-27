// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package local

import (
	"fmt"
	"sync"

	autoscalingv2 "k8s.io/api/autoscaling/v2"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func resetWorkloadMetricStore() {
	loadstore.WorkloadMetricStore = nil
	loadstore.WorkloadMetricStoreOnce = sync.Once{}
}

func newFakeWLMPodEvent(ns, deployment, podName string, containerNames []string) workloadmeta.Event {
	containers := []workloadmeta.OrchestratorContainer{}
	for _, c := range containerNames {
		containers = append(containers, workloadmeta.OrchestratorContainer{
			ID:   fmt.Sprintf("%s-id", c),
			Name: c,
			Resources: workloadmeta.ContainerResources{
				CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
				MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
			},
		})
	}

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podName,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: ns,
		},
		Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: fmt.Sprintf("%s-766dbb7846", deployment)}},
		Containers: containers,
	}

	return workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	}
}

func newAutoscaler(fallbackEnabled bool) model.PodAutoscalerInternal {
	pai := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Name:       "test-deployment",
			},
			Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
				{
					Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
					PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
						Name: "cpu",
						Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
							Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
							Utilization: pointer.Ptr(int32(80)),
						},
					},
				},
			},
		},
	}

	if fallbackEnabled {
		pai.Spec.Fallback = &datadoghq.DatadogFallbackPolicy{
			Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
				Enabled: true,
				Triggers: datadoghq.HorizontalFallbackTriggers{
					StaleRecommendationThresholdSeconds: 60,
				},
			},
		}
	}

	return pai.Build()
}

func newEntity(metricName, ns, deployment, podName, containerName string) *loadstore.Entity {
	return &loadstore.Entity{
		EntityType:    loadstore.ContainerType,
		EntityName:    containerName,
		Namespace:     ns,
		MetricName:    metricName,
		PodName:       podName,
		PodOwnerName:  deployment,
		PodOwnerkind:  loadstore.Deployment,
		ContainerName: containerName,
	}
}

func newEntityValue(timestamp int64, value float64) *loadstore.EntityValue {
	return &loadstore.EntityValue{
		Timestamp: loadstore.Timestamp(timestamp),
		Value:     loadstore.ValueType(value),
	}
}
