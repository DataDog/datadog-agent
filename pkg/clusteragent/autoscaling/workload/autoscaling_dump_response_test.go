// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestDump(t *testing.T) {
	testTime := time.Now()
	f := newFixture(t, testTime)
	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}

	dpai := model.FakePodAutoscalerInternal{
		Namespace:  "default",
		Name:       "dpa-0",
		Generation: 1,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "app-0",
				APIVersion: "apps/v1",
			},
			Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Mode: datadoghq.DatadogPodAutoscalerApplyModeApply,
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					Strategy: datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
				},
			},
			Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
				MinReplicas: pointer.Ptr(int32(1)),
				MaxReplicas: int32(10),
				Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
					{
						Name:    "app",
						Enabled: pointer.Ptr(true),
						Requests: &datadoghqcommon.DatadogPodAutoscalerContainerResourceConstraints{
							MinAllowed: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
							MaxAllowed: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1000m"),
								corev1.ResourceMemory: resource.MustParse("1000Mi"),
							},
						},
					},
				},
			},
			Fallback: &datadoghq.DatadogFallbackPolicy{
				Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
					Enabled: true,
					Triggers: datadoghq.HorizontalFallbackTriggers{
						StaleRecommendationThresholdSeconds: 600,
					},
				},
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
				{
					Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
					ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
						Name: "cpu",
						Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
							Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
							Utilization: pointer.Ptr(int32(85)),
						},
						Container: "app",
					},
				},
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: testTime,
				Replicas:  100,
			},
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     testTime,
				ResourcesHash: "1234567890",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{
						Name: "app",
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("1000Mi"),
						},
					},
				},
			},
		},
		MainScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
				Timestamp: testTime,
				Replicas:  100,
			},
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     testTime,
				ResourcesHash: "1234567890",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{
						Name: "app",
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("1000Mi"),
						},
					},
				},
			},
		},
		FallbackScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: testTime,
				Replicas:  100,
			},
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     testTime,
				ResourcesHash: "1234567890",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{
						Name: "app",
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("1000Mi"),
						},
					},
				},
			},
		},
		TargetGVK:                 expectedGVK,
		HorizontalEventsRetention: 30 * time.Minute,
		CustomRecommenderConfiguration: &model.RecommenderConfiguration{
			Endpoint: "https://custom-recommender.com",
			Settings: map[string]any{
				"key": "value",
			},
		},
		HorizontalLastActions: []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
			{
				Time:                metav1.Time{Time: testTime},
				FromReplicas:        2,
				ToReplicas:          3,
				RecommendedReplicas: ptr.To(int32(3)),
			},
		},
		VerticalLastAction: &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
			Time:    metav1.Time{Time: testTime},
			Version: "1",
			Type:    datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType,
		},
	}

	f.store.Set("default/dpa-0", dpai.Build(), "")
	_, found := f.store.Get("default/dpa-0")
	assert.True(t, found)

	dump := Dump(context.Background())
	var buf bytes.Buffer
	dump.Write(&buf)
	output := buf.String()

	expectedOutput := fmt.Sprintf(`
=== PodAutoscaler default/dpa-0 ===
----------- PodAutoscaler ID -----------
default/dpa-0
----------- PodAutoscaler Meta -----------
Creation Timestamp: 0001-01-01 00:00:00 +0000 UTC
Generation: 1
Settings Timestamp: 0001-01-01 00:00:00 +0000 UTC
----------- PodAutoscaler Spec -----------
Target Ref: {Deployment app-0 apps/v1}
Owner: Local
Remote Version: <nil>
Apply Policy Mode: Apply
Update Policy: Auto
----------- PodAutoscaler Local Fallback -----------
Horizontal Fallback Enabled: true
Horizontal Fallback Stale Recommendation Threshold: 600
----------- PodAutoscaler Constraints -----------
Min Replicas: 1
Max Replicas: 10
Container: app
Enabled: true
Requests Min Allowed: map[cpu:100m memory:100Mi]
Requests Max Allowed: map[cpu:1 memory:1000Mi]
----------- PodAutoscaler Objectives -----------
Objective Type: PodResource
Resource Name: cpu
Utilization: 80

Objective Type: ContainerResource
Resource Name: cpu
Container Name: app
Utilization: 85

----------- PodAutoscaler Scaling Values -----------
[Horizontal]
Horizontal Error: <nil>
Source: Autoscaling
Timestamp: %[1]s
Replicas: 100
--------------------------------
[Vertical]
Vertical Error: <nil>
Source: Autoscaling
Timestamp: %[1]s
ResourcesHash: 1234567890
Container Name: app
Container Resources: map[cpu:100m memory:100Mi]
Container Limits: map[cpu:1 memory:1000Mi]
--------------------------------
Error: <nil>

----------- PodAutoscaler Main Scaling Values -----------
[Horizontal]
Horizontal Error: <nil>
Source: Local
Timestamp: %[1]s
Replicas: 100
--------------------------------
[Vertical]
Vertical Error: <nil>
Source: Autoscaling
Timestamp: %[1]s
ResourcesHash: 1234567890
Container Name: app
Container Resources: map[cpu:100m memory:100Mi]
Container Limits: map[cpu:1 memory:1000Mi]
--------------------------------
Error: <nil>

----------- PodAutoscaler Fallback Scaling Values -----------
[Horizontal]
Horizontal Error: <nil>
Source: Autoscaling
Timestamp: %[1]s
Replicas: 100
--------------------------------
[Vertical]
Vertical Error: <nil>
Source: Autoscaling
Timestamp: %[1]s
ResourcesHash: 1234567890
Container Name: app
Container Resources: map[cpu:100m memory:100Mi]
Container Limits: map[cpu:1 memory:1000Mi]
--------------------------------
Error: <nil>

----------- PodAutoscaler Status -----------
--------------------------------
Horizontal Last Action: Timestamp: %[1]s
From Replicas: 2
To Replicas: 3
Recommended Replicas: 3

--------------------------------
Vertical Last Action: Timestamp: %[1]s
Version: 1
Type: RolloutTriggered

----------- Custom Recommender -----------
Endpoint: https://custom-recommender.com
Settings: map[key:value]

===
`, testTime.String())
	compareTestOutput(t, output, expectedOutput)

	resetAutoscalingStore()
}

func compareTestOutput(t *testing.T, expected, actual string) {
	assert.Equal(t, strings.ReplaceAll(expected, " ", ""), strings.ReplaceAll(actual, " ", ""))
}
