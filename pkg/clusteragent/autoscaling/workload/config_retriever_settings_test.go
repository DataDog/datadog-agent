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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	clock "k8s.io/utils/clock/testing"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestConfigRetriverAutoscalingSettingsFollower(t *testing.T) {
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
	object1Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghq.DatadogPodAutoscalerRemoteOwner,
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
							Spec:      object1Spec,
						},
					},
				}),
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

func TestConfigRetriverAutoscalingSettingsLeader(t *testing.T) {
	testTime := time.Now()
	cr, mockRCClient := newMockConfigRetriever(t, true, clock.NewFakeClock(testTime))

	// Object specs
	object1Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghq.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy1",
		},
	}
	object2Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghq.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy2",
		},
	}
	object3Spec := &datadoghq.DatadogPodAutoscalerSpec{
		Owner: datadoghq.DatadogPodAutoscalerRemoteOwner,
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "v1",
			Kind:       "Deployment",
			Name:       "deploy3",
		},
		Policy: &datadoghq.DatadogPodAutoscalerPolicy{
			ApplyMode: datadoghq.DatadogPodAutoscalerAllApplyNone,
			Update: &datadoghq.DatadogPodAutoscalerUpdatePolicy{
				Strategy: datadoghq.DatadogPodAutoscalerAutoUpdateStrategy,
			},
		},
	}

	// Original update, 3 objects splitted in 2 configs
	settingsList1 := model.AutoscalingSettingsList{
		Settings: []model.AutoscalingSettings{
			{
				Namespace: "ns",
				Name:      "name1",
				Spec:      object1Spec,
			},
			{
				Namespace: "ns",
				Name:      "name2",
				Spec:      object2Spec,
			},
		},
	}

	settingsList2 := model.AutoscalingSettingsList{
		Settings: []model.AutoscalingSettings{
			{
				Namespace: "ns",
				Name:      "name3",
				Spec:      object3Spec,
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

	// Set expected versions
	object1Spec.RemoteVersion = pointer.Ptr[uint64](1)
	object2Spec.RemoteVersion = pointer.Ptr[uint64](1)
	object3Spec.RemoteVersion = pointer.Ptr[uint64](10)
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{
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
	object3Spec.Recommender = &datadoghq.DatadogPodAutoscalerRecommender{
		Name: "some-option",
	}
	object3Spec.Policy = nil

	stateCallbackCalled = 0
	mockRCClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo1": buildAutoscalingSettingsRawConfig(t, 1, settingsList1),  // Version unchanged
			"foo2": buildAutoscalingSettingsRawConfig(t, 11, settingsList2), // New version
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

	// Set expected versions: only one change for for foo2
	object3Spec.RemoteVersion = pointer.Ptr[uint64](11)
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{
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
		func(configKey string, applyState state.ApplyStatus) {
			stateCallbackCalled++
			assert.Equal(t, applyState, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "failed to unmarshal config id:, version: 12, config key: foo2, err: invalid character '}' after object key",
			})
		},
	)

	assert.Equal(t, 1, stateCallbackCalled)
	podAutoscalers = cr.store.GetAll()

	// No changes in expected versions
	model.AssertPodAutoscalersEqual(t, []model.PodAutoscalerInternal{
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

func buildAutoscalingSettingsRawConfig(t *testing.T, version uint64, autoscalingSettings model.AutoscalingSettingsList) state.RawConfig {
	t.Helper()

	content, err := json.Marshal(autoscalingSettings)
	assert.NoError(t, err)

	return buildRawConfig(t, data.ProductContainerAutoscalingSettings, version, content)
}
