// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clocktesting "k8s.io/utils/clock/testing"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func newTestProfileController() (*Controller, *autoscaling.Store[model.PodAutoscalerProfileInternal]) {
	profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
	fakeClock := clocktesting.NewFakeClock(metav1.Now().Time)
	c := &Controller{
		Controller: &autoscaling.Controller{
			ID:     controllerID,
			Client: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		},
		clock: fakeClock,
		store: profileStore,
	}
	return c, profileStore
}

func newTestProfile(name string, generation int64, template datadoghq.DatadogPodAutoscalerTemplate) *datadoghq.DatadogPodAutoscalerClusterProfile {
	return &datadoghq.DatadogPodAutoscalerClusterProfile{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscalerProfile",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: generation,
		},
		Spec: datadoghq.DatadogPodAutoscalerProfileSpec{
			Template: template,
		},
	}
}

func validTemplate() datadoghq.DatadogPodAutoscalerTemplate {
	maxReplicas := int32(10)
	return datadoghq.DatadogPodAutoscalerTemplate{
		Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
			{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: int32Ptr(80),
					},
				},
			},
		},
		Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
			MaxReplicas: &maxReplicas,
		},
	}
}

func TestSyncProfileNewProfile(t *testing.T) {
	c, profileStore := newTestProfileController()
	ctx := context.Background()

	profile := newTestProfile("high-cpu", 1, validTemplate())
	res, err := c.syncProfile(ctx, "high-cpu", profile)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.Requeue, res)

	pi, ok := profileStore.Get("high-cpu")
	require.True(t, ok, "Profile should be in store")
	assert.Equal(t, "high-cpu", pi.Name())
	assert.Equal(t, int64(1), pi.Generation())
	assert.True(t, pi.Valid())
	assert.NotEmpty(t, pi.TemplateHash())
}

func TestSyncProfileNilBothSides(t *testing.T) {
	c, profileStore := newTestProfileController()
	ctx := context.Background()

	res, err := c.syncProfile(ctx, "missing", nil)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.NoRequeue, res)

	_, ok := profileStore.Get("missing")
	assert.False(t, ok, "Should not create entry when both are nil")
}

func TestSyncProfileDelete(t *testing.T) {
	c, profileStore := newTestProfileController()
	ctx := context.Background()

	profile := newTestProfile("high-cpu", 1, validTemplate())
	_, _ = c.syncProfile(ctx, "high-cpu", profile)

	_, ok := profileStore.Get("high-cpu")
	require.True(t, ok)

	res, err := c.syncProfile(ctx, "high-cpu", nil)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.NoRequeue, res)

	_, ok = profileStore.Get("high-cpu")
	assert.False(t, ok, "Profile should be deleted from store")
}

func TestSyncProfilePreservesWorkloadRefs(t *testing.T) {
	c, profileStore := newTestProfileController()
	ctx := context.Background()

	profile1 := newTestProfile("high-cpu", 1, validTemplate())
	_, _ = c.syncProfile(ctx, "high-cpu", profile1)

	// Simulate the workload watcher setting refs.
	pi, _ := profileStore.Get("high-cpu")
	pi.UpdateWorkloads([]model.NamespacedObjectReference{
		testRef("prod", "web-app"),
	})
	profileStore.Set("high-cpu", pi, "pw")

	// Re-sync with a new generation. The status update to K8s will fail (fake client
	// doesn't have the CRD) but the store is updated regardless.
	profile2 := newTestProfile("high-cpu", 2, validTemplate())
	_, _ = c.syncProfile(ctx, "high-cpu", profile2)

	pi2, ok := profileStore.Get("high-cpu")
	require.True(t, ok)
	assert.Equal(t, int64(2), pi2.Generation())
	assert.Len(t, pi2.Workloads(), 1, "Workload refs should be preserved across profile updates")
}

func TestSyncProfileDeleteNotifiesObservers(t *testing.T) {
	c, profileStore := newTestProfileController()
	ctx := context.Background()

	deleted := false
	profileStore.RegisterObserver(autoscaling.Observer{
		DeleteFunc: func(key string, _ autoscaling.SenderID) {
			if key == "high-cpu" {
				deleted = true
			}
		},
	})

	profile := newTestProfile("high-cpu", 1, validTemplate())
	_, _ = c.syncProfile(ctx, "high-cpu", profile)

	_, _ = c.syncProfile(ctx, "high-cpu", nil)

	assert.True(t, deleted, "Delete observer should fire when profile is removed")
}

func TestSyncProfileBurstableAnnotation(t *testing.T) {
	c, profileStore := newTestProfileController()
	ctx := context.Background()

	t.Run("burstable=true when annotation present", func(t *testing.T) {
		profile := newTestProfile("p1", 1, validTemplate())
		profile.Annotations = map[string]string{model.BurstableAnnotation: "true"}
		_, err := c.syncProfile(ctx, "p1", profile)
		require.NoError(t, err)

		pi, ok := profileStore.Get("p1")
		require.True(t, ok)
		assert.True(t, pi.Burstable())
	})

	t.Run("burstable=false when annotation absent", func(t *testing.T) {
		profile := newTestProfile("p2", 1, validTemplate())
		_, err := c.syncProfile(ctx, "p2", profile)
		require.NoError(t, err)

		pi, ok := profileStore.Get("p2")
		require.True(t, ok)
		assert.False(t, pi.Burstable())
	})

	t.Run("burstable removed on annotation drop", func(t *testing.T) {
		profile := newTestProfile("p3", 1, validTemplate())
		profile.Annotations = map[string]string{model.BurstableAnnotation: "true"}
		_, _ = c.syncProfile(ctx, "p3", profile)

		pi, _ := profileStore.Get("p3")
		assert.True(t, pi.Burstable())

		// Remove annotation and re-sync.
		// The fake K8s client doesn't have the CRD registered so the status update
		// will return an error — that's expected. The store is still updated.
		profile.Annotations = nil
		profile.Generation = 2
		_, _ = c.syncProfile(ctx, "p3", profile)

		pi, ok := profileStore.Get("p3")
		require.True(t, ok)
		assert.False(t, pi.Burstable(), "burstable should be false after annotation is removed")
	})
}
