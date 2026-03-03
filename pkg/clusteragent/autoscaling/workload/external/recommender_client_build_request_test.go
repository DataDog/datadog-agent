// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package external

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	clock "k8s.io/utils/clock/testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func TestBuildWorkloadRecommendationRequest_Table(t *testing.T) {
	type target struct {
		typ string
		val float64
	}

	cases := []struct {
		name          string
		dpa           model.FakePodAutoscalerInternal
		cluster       string
		expectTargets []target
		expectReq     *kubeAutoscaling.WorkloadRecommendationRequest
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
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{MinReplicas: pointer.Ptr[int32](2), MaxReplicas: pointer.Ptr[int32](4)},
				},
				CurrentReplicas: pointer.Ptr[int32](3),
				ScalingValues:   model.ScalingValues{Horizontal: &model.HorizontalScalingValues{Replicas: 3}},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "http://example.invalid",
					Settings: map[string]interface{}{"custom_setting": "value"},
				},
			},
			cluster:       "test-cluster",
			expectTargets: []target{{typ: "cpu", val: 0.80}, {typ: "memory", val: 0.75}},
			expectReq: &kubeAutoscaling.WorkloadRecommendationRequest{
				TargetRef: &kubeAutoscaling.WorkloadTargetRef{Kind: "Deployment", Name: "test-deployment", ApiVersion: "apps/v1", Namespace: "default", Cluster: "test-cluster"},
				State: &kubeAutoscaling.WorkloadState{
					DesiredReplicas: 3,
					ReadyReplicas:   lo.ToPtr[int32](0),
					CurrentReplicas: lo.ToPtr[int32](3),
				},
				Constraints: &kubeAutoscaling.WorkloadRecommendationConstraints{MinReplicas: 2, MaxReplicas: 4},
				Settings:    map[string]*structpb.Value{"custom_setting": {Kind: &structpb.Value_StringValue{StringValue: "value"}}},
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{Type: "cpu", TargetValue: 0.80},
					{Type: "memory", TargetValue: 0.75},
				},
			},
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
			cluster:       "test-cluster",
			expectTargets: []target{{typ: "cpu", val: 0.01}},
			expectReq: &kubeAutoscaling.WorkloadRecommendationRequest{
				TargetRef: &kubeAutoscaling.WorkloadTargetRef{Kind: "Deployment", Name: "test-deployment", ApiVersion: "apps/v1", Namespace: "default", Cluster: "test-cluster"},
				State: &kubeAutoscaling.WorkloadState{
					ReadyReplicas: lo.ToPtr[int32](0),
				},
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{Type: "cpu", TargetValue: 0.01},
				},
			},
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
			cluster:       "c",
			expectTargets: []target{{typ: "cpu", val: 0.50}},
			expectReq: &kubeAutoscaling.WorkloadRecommendationRequest{
				TargetRef: &kubeAutoscaling.WorkloadTargetRef{Kind: "Deployment", Name: "wl", ApiVersion: "apps/v1", Namespace: "ns", Cluster: "c"},
				State: &kubeAutoscaling.WorkloadState{
					ReadyReplicas: lo.ToPtr[int32](0),
				},
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{Type: "cpu", TargetValue: 0.50},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClock := clock.NewFakeClock(time.Now())
			client, err := newRecommenderClient(context.Background(), fakeClock, workload.NewPodWatcher(nil, nil), nil)
			assert.NoError(t, err)
			req, err := client.buildWorkloadRecommendationRequest(tc.cluster, tc.dpa.Build(), tc.dpa.CustomRecommenderConfiguration)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectReq, req)
		})
	}
}
