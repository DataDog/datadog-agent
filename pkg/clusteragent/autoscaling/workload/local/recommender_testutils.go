// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package local

import (
	"sync"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"

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

// newFakePod builds a single pod with the given lifecycle/readiness state. CPU and memory
// requests are set on every container so it can be used as an autoscaling target.
type fakePodConfig struct {
	namespace      string
	deployment     string
	podName        string
	containerNames []string
	cpuRequest     float64
	phase          string
	ready          bool
	readyTimestamp time.Time
}

func newFakePod(config fakePodConfig) *workloadmeta.KubernetesPod {
	if config.namespace == "" {
		config.namespace = "default"
	}
	if config.podName == "" {
		config.podName = "pod"
	}
	if len(config.containerNames) == 0 {
		config.containerNames = []string{"c"}
	}
	if config.cpuRequest == 0 {
		config.cpuRequest = 100.0 // 1 core
	}
	if config.phase == "" {
		config.phase = string(corev1.PodRunning)
	}

	// A zero readyTimestamp means "readiness time unknown" -> nil pointer.
	var readyTS *time.Time
	if !config.readyTimestamp.IsZero() {
		readyTS = &config.readyTimestamp
		config.ready = true
	}

	containers := make([]workloadmeta.OrchestratorContainer, 0, len(config.containerNames))
	for _, c := range config.containerNames {
		containers = append(containers, workloadmeta.OrchestratorContainer{
			ID:   c + "-id",
			Name: c,
			Resources: workloadmeta.ContainerResources{
				CPURequest:    pointer.Ptr(config.cpuRequest),
				MemoryRequest: pointer.Ptr[uint64](2048),
			},
		})
	}

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   config.podName,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      config.podName,
			Namespace: config.namespace,
		},
		Containers:     containers,
		Phase:          config.phase,
		Ready:          config.ready,
		ReadyTimestamp: readyTS,
	}
	if config.deployment != "" {
		pod.Owners = []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: config.deployment + "-766dbb7846"}}
	}
	return pod
}

// newCPUUsageResult builds a loadstore PodResult with two CPU usage data points at
// currentTime-15s and currentTime-30s that yield the requested utilization ratio against
// a 1-core (1e9 nanocores) request.
func newCPUUsageResult(podName, containerName string, utilization float64, currentTime time.Time) loadstore.PodResult {
	usage := utilization * 1e9 // 1 core request == 1e9 nanocores
	return loadstore.PodResult{
		PodName: podName,
		ContainerValues: map[string][]loadstore.EntityValue{
			containerName: {
				*newEntityValue(currentTime.Unix()-15, usage),
				*newEntityValue(currentTime.Unix()-30, usage),
			},
		},
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
