// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workloadimpl

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	clock "k8s.io/utils/clock/testing"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/comp/autoscaling/workload/impl/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestConfigRetriverAutoscalingValuesFollower(t *testing.T) {
	testTime := time.Now()
	cr, mockRCClient := newMockConfigRetriever(t, false, clock.NewFakeClock(testTime))

	// Dummy objects in store
	dummy2 := model.PodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name2",
	}
	dummy3 := model.PodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name3",
	}
	cr.store.Set("ns/name2", dummy2, "unittest")
	cr.store.Set("ns/name3", dummy3, "unittest")

	// Object specs
	value1 := &kubeAutoscaling.WorkloadValues{
		Namespace: "ns",
		Name:      "name1",
		Horizontal: &kubeAutoscaling.WorkloadHorizontalValues{
			Auto: &kubeAutoscaling.WorkloadHorizontalData{
				Replicas: pointer.Ptr[int32](3),
			},
		},
	}

	// New Autoscaling settings received, should do nothing
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingValuesRawConfig(t, 1, value1),
		},
		func(configKey string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
				Error: "",
			})
		},
	)

	assert.Equal(t, 1, stateCallbackCalled)
	podAutoscalers := cr.store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{dummy2, dummy3}, podAutoscalers)
}

func TestConfigRetriverAutoscalingValuesLeader(t *testing.T) {
	testTime := time.Now()
	cr, mockRCClient := newMockConfigRetriever(t, true, clock.NewFakeClock(testTime))

	// Dummy objects in store
	cr.store.Set("ns/name1", model.PodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name1",
	}, "unittest")
	cr.store.Set("ns/name2", model.PodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name2",
	}, "unittest")
	cr.store.Set("ns/name3", model.PodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name3",
	}, "unittest")

	// Object specs
	value1 := &kubeAutoscaling.WorkloadValues{
		Namespace: "ns",
		Name:      "name1",
		Horizontal: &kubeAutoscaling.WorkloadHorizontalValues{
			Manual: &kubeAutoscaling.WorkloadHorizontalData{
				Replicas: pointer.Ptr[int32](3),
			},
			// Validate that Manual has priority
			Auto: &kubeAutoscaling.WorkloadHorizontalData{
				Replicas: pointer.Ptr[int32](200),
			},
		},
	}
	value2 := &kubeAutoscaling.WorkloadValues{
		Namespace: "ns",
		Name:      "name2",
		Vertical: &kubeAutoscaling.WorkloadVerticalValues{
			Auto: &kubeAutoscaling.WorkloadVerticalData{
				Resources: []*kubeAutoscaling.ContainerResources{
					{
						ContainerName: "container1",
						Requests: []*kubeAutoscaling.ContainerResources_ResourceList{
							{
								Name:  "cpu",
								Value: "10m",
							},
							{
								Name:  "memory",
								Value: "10Mi",
							},
						},
					},
				},
			},
		},
		Horizontal: &kubeAutoscaling.WorkloadHorizontalValues{
			Auto: &kubeAutoscaling.WorkloadHorizontalData{
				Replicas: pointer.Ptr[int32](6),
			},
		},
	}
	value3 := &kubeAutoscaling.WorkloadValues{
		Namespace: "ns",
		Name:      "name3",
		Horizontal: &kubeAutoscaling.WorkloadHorizontalValues{
			Auto: &kubeAutoscaling.WorkloadHorizontalData{
				Replicas: pointer.Ptr[int32](5),
			},
		},
		Vertical: &kubeAutoscaling.WorkloadVerticalValues{
			Manual: &kubeAutoscaling.WorkloadVerticalData{
				Resources: []*kubeAutoscaling.ContainerResources{
					{
						ContainerName: "container1",
						Requests: []*kubeAutoscaling.ContainerResources_ResourceList{
							{
								Name:  "cpu",
								Value: "100m",
							},
							{
								Name:  "memory",
								Value: "100Mi",
							},
						},
						Limits: []*kubeAutoscaling.ContainerResources_ResourceList{
							{
								Name:  "cpu",
								Value: "200m",
							},
							{
								Name:  "memory",
								Value: "200Mi",
							},
						},
					},
				},
			},
			Auto: &kubeAutoscaling.WorkloadVerticalData{
				Resources: []*kubeAutoscaling.ContainerResources{
					{
						ContainerName: "container100",
						Requests: []*kubeAutoscaling.ContainerResources_ResourceList{
							{
								Name:  "cpu",
								Value: "100m",
							},
						},
					},
				},
			},
		},
	}

	// Trigger update from Autoscaling values
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingValuesRawConfig(t, 1, value1),
			"foo2": buildAutoscalingValuesRawConfig(t, 2, value2, value3),
		},
		func(configKey string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)

	assert.Equal(t, 2, stateCallbackCalled)
	podAutoscalers := cr.store.GetAll()

	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{
		{
			Namespace: "ns",
			Name:      "name1",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerManualValueSource,
					Replicas:  3,
					Timestamp: testTime,
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name2",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghq.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					Timestamp:     testTime,
					ResourcesHash: "9d1474c7c20f3820",
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  5,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghq.DatadogPodAutoscalerManualValueSource,
					ContainerResources: []datadoghq.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
						},
					},
					Timestamp:     testTime,
					ResourcesHash: "e55f79588b87a881",
				},
			},
		},
	}, podAutoscalers)

	// Update some values, check that we are processing correctly
	value1.Horizontal = nil
	value3.Vertical = nil
	value3.Horizontal.Auto.Replicas = pointer.Ptr[int32](6)

	// Trigger update
	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingValuesRawConfig(t, 10, value1),
			"foo2": buildAutoscalingValuesRawConfig(t, 20, value2, value3),
		},
		func(configKey string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)
	assert.Equal(t, 2, stateCallbackCalled)

	podAutoscalers = cr.store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{
		{
			Namespace:     "ns",
			Name:          "name1",
			ScalingValues: model.ScalingValues{},
		},
		{
			Namespace: "ns",
			Name:      "name2",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghq.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					Timestamp:     testTime,
					ResourcesHash: "9d1474c7c20f3820",
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
			},
		},
	}, podAutoscalers)

	// Receive some incorrect values, should keep old values
	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo1": buildRawConfig(t, data.ProductContainerAutoscalingValues, 11, []byte(`{"foo"}`)),
		},
		func(configKey string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "failed to unmarshal config id:, version: 11, config key: foo1, err: invalid character '}' after object key",
			})
		},
	)
	assert.Equal(t, 1, stateCallbackCalled)

	podAutoscalers = cr.store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{
		{
			Namespace:     "ns",
			Name:          "name1",
			ScalingValues: model.ScalingValues{},
		},
		{
			Namespace: "ns",
			Name:      "name2",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghq.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					ResourcesHash: "9d1474c7c20f3820",
					Timestamp:     testTime,
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
			},
		},
	}, podAutoscalers)

	// Deactvating autoscaling values, should clean-up values that are not present anymore (value1, value2)
	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo2": buildAutoscalingValuesRawConfig(t, 21, value2),
		},
		func(configKey string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)
	assert.Equal(t, 1, stateCallbackCalled)

	podAutoscalers = cr.store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{
		{
			Namespace: "ns",
			Name:      "name1",
		},
		{
			Namespace: "ns",
			Name:      "name2",
			ScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghq.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					Timestamp:     testTime,
					ResourcesHash: "9d1474c7c20f3820",
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
		},
	}, podAutoscalers)
}

func buildAutoscalingValuesRawConfig(t *testing.T, version uint64, values ...*kubeAutoscaling.WorkloadValues) state.RawConfig {
	t.Helper()

	valuesList := &kubeAutoscaling.WorkloadValuesList{
		Values: values,
	}

	content, err := json.Marshal(valuesList)
	assert.NoError(t, err)

	return buildRawConfig(t, data.ProductContainerAutoscalingSettings, version, content)
}
