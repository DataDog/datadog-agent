// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	clock "k8s.io/utils/clock/testing"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestConfigRetriverAutoscalingValuesFollower(t *testing.T) {
	testTime := time.Now()
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	_, mockRCClient := newMockConfigRetriever(t, func() bool { return false }, store, clock.NewFakeClock(testTime))

	// Dummy objects in store
	dummy2 := model.FakePodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name2",
	}
	dummy3 := model.FakePodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name3",
	}
	store.Set("ns/name2", dummy2.Build(), "unittest")
	store.Set("ns/name3", dummy3.Build(), "unittest")

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

	// New Autoscaling values received, should store values in state
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingValuesRawConfig(t, 1, value1),
		},
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)

	assert.Equal(t, 1, stateCallbackCalled)
	podAutoscalers := store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{dummy2, dummy3}, podAutoscalers)
}

func TestConfigRetriverAutoscalingValuesLeader(t *testing.T) {
	testTime := time.Now()
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	_, mockRCClient := newMockConfigRetriever(t, func() bool { return true }, store, clock.NewFakeClock(testTime))

	// Dummy objects in store
	store.Set("ns/name1", model.FakePodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name1",
	}.Build(), "unittest")
	store.Set("ns/name2", model.FakePodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name2",
	}.Build(), "unittest")
	store.Set("ns/name3", model.FakePodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name3",
	}.Build(), "unittest")

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
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)

	assert.Equal(t, 2, stateCallbackCalled)
	podAutoscalers := store.GetAll()

	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace: "ns",
			Name:      "name1",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerManualValueSource,
					Replicas:  3,
					Timestamp: testTime,
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name2",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					Timestamp:     testTime,
					ResourcesHash: "8fe97e46aa840723",
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  5,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghqcommon.DatadogPodAutoscalerManualValueSource,
					ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
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
					ResourcesHash: "f41ccab869dc36a7",
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
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)
	assert.Equal(t, 2, stateCallbackCalled)

	podAutoscalers = store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			MainScalingValues: model.ScalingValues{},
		},
		{
			Namespace: "ns",
			Name:      "name2",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					Timestamp:     testTime,
					ResourcesHash: "8fe97e46aa840723",
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "failed to unmarshal config id:, version: 11, config key: foo1, err: invalid character '}' after object key",
			})
		},
	)
	assert.Equal(t, 1, stateCallbackCalled)

	podAutoscalers = store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			MainScalingValues: model.ScalingValues{},
		},
		{
			Namespace: "ns",
			Name:      "name2",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					ResourcesHash: "8fe97e46aa840723",
					Timestamp:     testTime,
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)
	assert.Equal(t, 1, stateCallbackCalled)

	podAutoscalers = store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace: "ns",
			Name:      "name1",
		},
		{
			Namespace: "ns",
			Name:      "name2",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  6,
					Timestamp: testTime,
				},
				Vertical: &model.VerticalScalingValues{
					Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
						{
							Name: "container1",
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("10Mi"),
							},
						},
					},
					Timestamp:     testTime,
					ResourcesHash: "8fe97e46aa840723",
				},
			},
		},
		{
			Namespace: "ns",
			Name:      "name3",
		},
	}, podAutoscalers)
}

func TestConfigRetriverAutoscalingValuesReconcile(t *testing.T) {
	testClock := clock.NewFakeClock(time.Now())
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	isLeader := false
	isLeaderFunc := func() bool {
		return isLeader
	}

	_, mockRCClient := newMockConfigRetriever(t, isLeaderFunc, store, testClock)

	// Add a PodAutoscaler to the store
	store.Set("ns/name1", model.FakePodAutoscalerInternal{
		Namespace: "ns",
		Name:      "name1",
	}.Build(), "unittest")

	// Object values
	value1 := &kubeAutoscaling.WorkloadValues{
		Namespace: "ns",
		Name:      "name1",
		Horizontal: &kubeAutoscaling.WorkloadHorizontalValues{
			Auto: &kubeAutoscaling.WorkloadHorizontalData{
				Replicas: pointer.Ptr[int32](3),
			},
		},
	}

	// New Autoscaling values received, should store values in state but not update the store
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingValuesRawConfig(t, 1, value1),
		},
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)

	// Nothing changed in the store as we are not the leader
	assert.Equal(t, 1, stateCallbackCalled)
	podAutoscalers := store.GetAll()
	assert.Equal(t, 1, len(podAutoscalers))
	// Verify the PodAutoscaler doesn't have values
	podAutoscaler := podAutoscalers[0]
	assert.Equal(t, model.ScalingValues{}, podAutoscaler.MainScalingValues())

	// Become leader and receive values again - now they should be processed and reconciled immediately
	isLeader = true
	callbackTimestamp := testClock.Now()
	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingValues,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingValuesRawConfig(t, 2, value1),
		},
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)

	assert.Equal(t, 1, stateCallbackCalled)
	podAutoscalers = store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace: "ns",
			Name:      "name1",
			MainScalingValues: model.ScalingValues{
				Horizontal: &model.HorizontalScalingValues{
					Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
					Replicas:  3,
					Timestamp: callbackTimestamp,
				},
			},
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

	return buildRawConfig(t, data.ProductContainerAutoscalingValues, version, content)
}
