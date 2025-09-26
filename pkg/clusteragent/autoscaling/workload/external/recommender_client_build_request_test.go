// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package external

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestBuildWorkloadRecommendationRequest_Table(t *testing.T) {
	type target struct {
		typ string
		val float64
	}

	cases := []struct {
		name              string
		dpa               model.FakePodAutoscalerInternal
		cluster           string
		expectTargets     []target
		expectTargetRef   *kubeAutoscaling.WorkloadTargetRef
		expectDesired     *int32
		expectReady       *int32
		expectConstraints *kubeAutoscaling.WorkloadRecommendationConstraints
		expectSettings    map[string]string
	}{
		{
			name: "fractional targets and fields",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "test-deployment", APIVersion: "apps/v1"},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name:  corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{Utilization: pointer.Ptr[int32](80)},
							},
						},
						{
							Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
							ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
								Name:  corev1.ResourceMemory,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{Utilization: pointer.Ptr[int32](75)},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{MinReplicas: pointer.Ptr[int32](2), MaxReplicas: 4},
				},
				CurrentReplicas: pointer.Ptr[int32](3),
				ScalingValues:   model.ScalingValues{Horizontal: &model.HorizontalScalingValues{Replicas: 3}},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "http://example.invalid",
					Settings: map[string]interface{}{"custom_setting": "value"},
				},
			},
			cluster:           "test-cluster",
			expectTargets:     []target{{typ: "cpu", val: 0.80}, {typ: "memory", val: 0.75}},
			expectTargetRef:   &kubeAutoscaling.WorkloadTargetRef{Kind: "Deployment", Name: "test-deployment", ApiVersion: "apps/v1", Namespace: "default", Cluster: "test-cluster"},
			expectDesired:     pointer.Ptr[int32](3),
			expectReady:       pointer.Ptr[int32](0),
			expectConstraints: &kubeAutoscaling.WorkloadRecommendationConstraints{MinReplicas: 2, MaxReplicas: 4},
			expectSettings:    map[string]string{"custom_setting": "value"},
		},
		{
			name: "one percent utilization",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "test-deployment", APIVersion: "apps/v1"},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name:  corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{Utilization: pointer.Ptr[int32](1)},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{Endpoint: "http://example.invalid"},
			},
			cluster:         "test-cluster",
			expectTargets:   []target{{typ: "cpu", val: 0.01}},
			expectTargetRef: &kubeAutoscaling.WorkloadTargetRef{Kind: "Deployment", Name: "test-deployment", ApiVersion: "apps/v1", Namespace: "default", Cluster: "test-cluster"},
		},
		{
			name: "minimal config",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "ns",
				Name:      "name",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "wl", APIVersion: "apps/v1"},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
							ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
								Name:  corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{Utilization: pointer.Ptr[int32](50)},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{Endpoint: "http://example.invalid"},
			},
			cluster:         "c",
			expectTargets:   []target{{typ: "cpu", val: 0.50}},
			expectTargetRef: &kubeAutoscaling.WorkloadTargetRef{Kind: "Deployment", Name: "wl", ApiVersion: "apps/v1", Namespace: "ns", Cluster: "c"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newRecommenderClient(workload.NewPodWatcher(nil, nil))
			req, err := client.buildWorkloadRecommendationRequest(tc.cluster, tc.dpa.Build(), tc.dpa.CustomRecommenderConfiguration)
			assert.NoError(t, err)
			if assert.NotNil(t, req) {
				// TargetRef
				if tc.expectTargetRef != nil {
					assert.Equal(t, tc.expectTargetRef, req.TargetRef)
				}
				// State (current replicas always equals dpa.CurrentReplicas)
				assert.Equal(t, tc.dpa.CurrentReplicas, req.State.CurrentReplicas)
				if tc.expectDesired != nil {
					assert.Equal(t, *tc.expectDesired, req.State.DesiredReplicas)
				}
				if tc.expectReady != nil {
					if assert.NotNil(t, req.State.ReadyReplicas) {
						assert.Equal(t, *tc.expectReady, *req.State.ReadyReplicas)
					}
				}
				// Targets
				if assert.Len(t, req.Targets, len(tc.expectTargets)) {
					for i, et := range tc.expectTargets {
						assert.Equal(t, et.typ, req.Targets[i].Type)
						assert.InDelta(t, et.val, req.Targets[i].TargetValue, 0.0001)
					}
				}
				// Constraints
				if tc.expectConstraints != nil {
					if assert.NotNil(t, req.Constraints) {
						assert.Equal(t, tc.expectConstraints.MinReplicas, req.Constraints.MinReplicas)
						assert.Equal(t, tc.expectConstraints.MaxReplicas, req.Constraints.MaxReplicas)
					}
				}
				// Settings
				if tc.expectSettings != nil {
					if assert.NotNil(t, req.Settings) {
						for k, v := range tc.expectSettings {
							av, ok := req.Settings[k]
							assert.True(t, ok)
							assert.Equal(t, structpb.NewStringValue(v).GetStringValue(), av.GetStringValue())
						}
					}
				}
			}
		})
	}
}
