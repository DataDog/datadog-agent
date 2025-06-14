// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"bytes"
	"encoding/json"
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

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestDump(t *testing.T) {
	config := config.NewMock(t)
	config.SetWithoutSource("autoscaling.workload.enabled", true)
	testTime := time.Now()
	f := newFixture(t, testTime)
	InitDumper(f.store)

	dpai := createFakePodAutoscaler(testTime)

	f.store.Set("default/dpa-0", dpai.Build(), "")
	_, found := f.store.Get("default/dpa-0")
	assert.True(t, found)

	dump := Dump()
	var buf bytes.Buffer
	dump.Print(&buf)
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
Apply Policy Mode: Apply
Update Policy: Auto
Scale Up Strategy: Max
Scale Up Rule Type: Pods
Scale Up Rule Value: 1
Scale Up Rule Period: 10
Scale Up Stabilization Window: 10
Scale Down Strategy: Min
Scale Down Rule Type: Pods
Scale Down Rule Value: 1
Scale Down Rule Period: 10
Scale Down Stabilization Window: 10

----------- PodAutoscaler Local Fallback -----------
Horizontal Fallback Enabled: true
Horizontal Fallback Stale Recommendation Threshold: 600

----------- PodAutoscaler Constraints -----------
Min Replicas: 1
Max Replicas: 10
Container: app
Enabled: true
Requests Min Allowed: [cpu:100m][memory:100Mi]
Requests Max Allowed: [cpu:1][memory:1000Mi]

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
Vertical Error: test vertical error
Source: Autoscaling
Timestamp: %[1]s
ResourcesHash: 1234567890
Container Name: app
Container Resources: [cpu:100m][memory:100Mi]
Container Limits: [cpu:1][memory:1000Mi]
--------------------------------
Error: test error

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
Container Resources: [cpu:100m][memory:100Mi]
Container Limits: [cpu:1][memory:1000Mi]
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
Container Resources: [cpu:100m][memory:100Mi]
Container Limits: [cpu:1][memory:1000Mi]
--------------------------------
Error: <nil>

----------- PodAutoscaler Status -----------
Error: test error
--------------------------------
Horizontal Last Action: Timestamp: %[1]s
From Replicas: 2
To Replicas: 3
Recommended Replicas: 3
--------------------------------
Vertical Last Action Error: test vertical last action error
Vertical Last Action: Timestamp: %[1]s
Version: 1
Type: RolloutTriggered

----------- Custom Recommender -----------
Endpoint: https://custom-recommender.com
Settings: map[key:value]

===
`, testTime.String())
	compareTestOutput(t, expectedOutput, output)
}

func TestMarshalUnmarshal(t *testing.T) {
	// json serialization drops nanoseconds; strip it here
	testTime := time.Unix(time.Now().Unix(), 0).UTC()
	fakeDpai := createFakePodAutoscaler(testTime)
	realDpai := fakeDpai.Build()
	jsonDpai, err := json.Marshal(&realDpai)
	assert.NoError(t, err)

	unmarshalledDpai := model.PodAutoscalerInternal{}
	err = json.Unmarshal(jsonDpai, &unmarshalledDpai)
	assert.NoError(t, err)

	compareTestOutput(t, realDpai.String(true), unmarshalledDpai.String(true))
}

func createFakePodAutoscaler(testTime time.Time) model.FakePodAutoscalerInternal {
	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
	return model.FakePodAutoscalerInternal{
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
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMaxChangeStrategySelect),
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         1,
							PeriodSeconds: 10,
						},
					},
					StabilizationWindowSeconds: 10,
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMinChangeStrategySelect),
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         1,
							PeriodSeconds: 10,
						},
					},
					StabilizationWindowSeconds: 10,
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
			VerticalError:   fmt.Errorf("test vertical error"),
			HorizontalError: nil,
			Error:           fmt.Errorf("test error"),
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
		VerticalLastActionError: fmt.Errorf("test vertical last action error"),
		Error:                   fmt.Errorf("test error"),
	}
}

func compareTestOutput(t *testing.T, expected, actual string) {
	expected = strings.ReplaceAll(expected, " ", "")
	expected = strings.ReplaceAll(expected, "GMT", "UTC")

	actual = strings.ReplaceAll(actual, " ", "")
	actual = strings.ReplaceAll(actual, "GMT", "UTC")

	assert.Equal(t, expected, actual)
}
