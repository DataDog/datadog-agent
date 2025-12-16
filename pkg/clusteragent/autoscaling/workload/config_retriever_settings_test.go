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
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	clock "k8s.io/utils/clock/testing"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestConfigRetriverAutoscalingSettingsFollower(t *testing.T) {
	testTime := time.Now()
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	_, mockRCClient := newMockConfigRetriever(t, func() bool { return false }, store, clock.NewFakeClock(testTime))

	// Dummy objects in store
	dummy2 := model.NewFakePodAutoscalerInternal("ns", "name2", nil)
	dummy3 := model.NewFakePodAutoscalerInternal("ns", "name3", nil)

	store.Set("ns/name2", dummy2, "unittest")
	store.Set("ns/name3", dummy3, "unittest")

	// Object specs
	object1Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy1",
		},
	}

	// New Autoscaling settings received, should do nothing
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingSettingsRawConfig(t, 1,
				model.AutoscalingSettingsList{
					Settings: []model.AutoscalingSettings{
						{
							Namespace: "ns",
							Name:      "name1",
							Specs: &model.AutoscalingSpecs{
								V1Alpha2: object1Spec,
							},
						},
					},
				}),
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
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{dummy2, dummy3}, podAutoscalers)
}

func TestConfigRetriverAutoscalingSettingsLeader(t *testing.T) {
	testTime := time.Now()
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	_, mockRCClient := newMockConfigRetriever(t, func() bool { return true }, store, clock.NewFakeClock(testTime))

	// Object specs
	object1Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy1",
		},
	}
	object2Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy2",
		},
	}
	object3Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy3",
		},
		ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
			Mode: datadoghq.DatadogPodAutoscalerApplyModePreview,
			Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
				Strategy: datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
			},
		},
	}

	// Original update, 3 objects splitted in 2 configs
	settingsList1 := model.AutoscalingSettingsList{
		Settings: []model.AutoscalingSettings{
			{
				Namespace: "ns",
				Name:      "name1",
				Specs: &model.AutoscalingSpecs{
					V1Alpha2: object1Spec,
				},
			},
			{
				Namespace: "ns",
				Name:      "name2",
				Specs: &model.AutoscalingSpecs{
					V1Alpha2: object2Spec,
				},
			},
		},
	}

	settingsList2 := model.AutoscalingSettingsList{
		Settings: []model.AutoscalingSettings{
			{
				Namespace: "ns",
				Name:      "name3",
				Specs: &model.AutoscalingSpecs{
					V1Alpha2: object3Spec,
				},
			},
		},
	}

	// New Autoscaling settings received
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingSettingsRawConfig(t, 1, settingsList1),
			"foo2": buildAutoscalingSettingsRawConfig(t, 10, settingsList2),
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

	// Set expected versions
	object1Spec.RemoteVersion = pointer.Ptr[uint64](versionOffset + 1)
	object2Spec.RemoteVersion = pointer.Ptr[uint64](versionOffset + 1)
	object3Spec.RemoteVersion = pointer.Ptr[uint64](versionOffset + 10)
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			Spec:              object1Spec,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name2",
			Spec:              object2Spec,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name3",
			Spec:              object3Spec,
			SettingsTimestamp: testTime,
		},
	}, podAutoscalers)

	// Update to existing autoscalingsettings received
	// Update the settings for object3
	// Both adding and removing fields
	object3Spec.Objectives = []datadoghqcommon.DatadogPodAutoscalerObjective{
		{
			Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
			PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
				Name: corev1.ResourceCPU,
				Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
					Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
					Utilization: pointer.Ptr[int32](80),
				},
			},
		},
	}
	object3Spec.ApplyPolicy = nil

	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingSettingsRawConfig(t, 1, settingsList1),  // Version unchanged
			"foo2": buildAutoscalingSettingsRawConfig(t, 11, settingsList2), // New version
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

	// Set expected versions: only one change for for foo2
	object3Spec.RemoteVersion = pointer.Ptr[uint64](versionOffset + 11)
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			Spec:              object1Spec,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name2",
			Spec:              object2Spec,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name3",
			Spec:              object3Spec,
			SettingsTimestamp: testTime,
		},
	}, podAutoscalers)

	// invalid update received, keeping old settings
	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo2": buildRawConfig(t, data.ProductContainerAutoscalingSettings, 12, []byte(`{"foo"}`)),
		},
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "failed to unmarshal config id:, version: 12, config key: foo2, err: invalid character '}' after object key",
			})
		},
	)

	assert.Equal(t, 1, stateCallbackCalled)
	podAutoscalers = store.GetAll()

	// No changes in expected versions
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			Spec:              object1Spec,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name2",
			Spec:              object2Spec,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name3",
			Spec:              object3Spec,
			SettingsTimestamp: testTime,
		},
	}, podAutoscalers)
}

func TestConfigRetrieverAutoscalingSettingOldVersions(t *testing.T) {
	testTime := time.Now()
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	_, mockRCClient := newMockConfigRetriever(t, func() bool { return true }, store, clock.NewFakeClock(testTime))

	// Object specs from different versions
	objectv1alpha1Spec := &datadoghqv1alpha1.DatadogPodAutoscalerSpec{
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy1",
		},
		Targets: []datadoghqcommon.DatadogPodAutoscalerObjective{
			{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: corev1.ResourceCPU,
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr[int32](80),
					},
				},
			},
		},
		Policy: &datadoghqv1alpha1.DatadogPodAutoscalerPolicy{
			ApplyMode: datadoghqv1alpha1.DatadogPodAutoscalerManualApplyMode,
			Upscale: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
				Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMinChangeStrategySelect),
			},
		},
	}

	settingsList1 := model.AutoscalingSettingsList{
		Settings: []model.AutoscalingSettings{
			{
				Namespace: "ns",
				Name:      "name1",
				// Old .Spec field
				Spec: objectv1alpha1Spec,
			},
			{
				Namespace: "ns",
				Name:      "name2",
				Specs: &model.AutoscalingSpecs{
					V1Alpha1: objectv1alpha1Spec,
				},
			},
		},
	}

	// New Autoscaling settings received
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingSettingsRawConfig(t, 1, settingsList1),
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

	// Expected v1alpha2 version output
	objectv1alpha2Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy1",
		},
		Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
			{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: corev1.ResourceCPU,
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr[int32](80),
					},
				},
			},
		},
		ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
			Mode: datadoghq.DatadogPodAutoscalerApplyModePreview,
			ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
				Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMinChangeStrategySelect),
			},
		},
	}

	// Copy with RemoteVersion set
	expected := objectv1alpha2Spec.DeepCopy()
	expected.RemoteVersion = pointer.Ptr[uint64](versionOffset + 1)
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			Spec:              expected,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name2",
			Spec:              expected,
			SettingsTimestamp: testTime,
		},
	}, podAutoscalers)

	// Now the backend has upgraded to v2alpha1 keeping old v1alpha1 for old Cluster Agents
	// v1alpha2 should be prioritized, creating a small diff to check output
	objectv1alpha2Spec.TargetRef.Name = "deploy2"
	settingsList1 = model.AutoscalingSettingsList{
		Settings: []model.AutoscalingSettings{
			{
				Namespace: "ns",
				Name:      "name1",
				Specs: &model.AutoscalingSpecs{
					V1Alpha1: objectv1alpha1Spec,
					V1Alpha2: objectv1alpha2Spec,
				},
			},
			{
				Namespace: "ns",
				Name:      "name2",
				Spec:      objectv1alpha1Spec,
				Specs: &model.AutoscalingSpecs{
					V1Alpha2: objectv1alpha2Spec,
				},
			},
		},
	}

	// New Autoscaling settings received (version 2)
	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingSettingsRawConfig(t, 2, settingsList1),
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

	// Copy with RemoteVersion set
	expected = objectv1alpha2Spec.DeepCopy()
	expected.RemoteVersion = pointer.Ptr[uint64](versionOffset + 2)
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			Spec:              expected,
			SettingsTimestamp: testTime,
		},
		{
			Namespace:         "ns",
			Name:              "name2",
			Spec:              expected,
			SettingsTimestamp: testTime,
		},
	}, podAutoscalers)
}

func TestConfigRetriverAutoscalingSettingsReconcile(t *testing.T) {
	testClock := clock.NewFakeClock(time.Now())
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	isLeader := false
	isLeaderFunc := func() bool {
		return isLeader
	}

	_, mockRCClient := newMockConfigRetriever(t, isLeaderFunc, store, testClock)

	// Object specs
	object1Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghqcommon.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy1",
		},
	}

	// New Autoscaling settings received, should not update store as we are not the leader
	callbackTimestamp := testClock.Now()
	stateCallbackCalled := 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingSettingsRawConfig(t, 1,
				model.AutoscalingSettingsList{
					Settings: []model.AutoscalingSettings{
						{
							Namespace: "ns",
							Name:      "name1",
							Specs: &model.AutoscalingSpecs{
								V1Alpha2: object1Spec,
							},
						},
					},
				}),
		},
		func(_ string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		},
	)

	// Nothing in the store as we are not the leader
	assert.Equal(t, 1, stateCallbackCalled)
	podAutoscalers := store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{}, podAutoscalers)

	// Become leader, should reconcile. Unfortunately, as it's another goroutine running the reconcile,
	// we need to wait for the reconcile to happen.
	isLeader = true
	// Wait for the ticker to be registered before stepping the clock to avoid race conditions
	require.Eventually(t, testClock.HasWaiters, 5*time.Second, 10*time.Millisecond, "ticker should be registered")
	testClock.Step(settingsReconcileInterval)
	require.Eventually(t, func() bool {
		return store.Count() == 1
	}, 1*time.Minute, 200*time.Millisecond)

	// Copy with RemoteVersion set
	object1Spec.RemoteVersion = pointer.Ptr[uint64](versionOffset + 1)
	podAutoscalers = store.GetAll()
	model.AssertPodAutoscalersEqual(t, []model.FakePodAutoscalerInternal{
		{
			Namespace:         "ns",
			Name:              "name1",
			Spec:              object1Spec,
			SettingsTimestamp: callbackTimestamp,
		},
	}, podAutoscalers)
}

func buildAutoscalingSettingsRawConfig(t *testing.T, version uint64, autoscalingSettings model.AutoscalingSettingsList) state.RawConfig {
	t.Helper()

	content, err := json.Marshal(autoscalingSettings)
	assert.NoError(t, err)

	return buildRawConfig(t, data.ProductContainerAutoscalingSettings, version, content)
}
