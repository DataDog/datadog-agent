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

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
)

func newFakeDynamicClient() *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	_ = datadoghq.AddToScheme(scheme)
	return dynamicfake.NewSimpleDynamicClient(scheme)
}

func TestBuiltinProfilesAreCreated(t *testing.T) {
	client := newFakeDynamicClient()
	mgr := NewBuiltinProfileManager(client, func() bool { return true })
	ctx := context.Background()

	mgr.reconcile(ctx)

	for _, p := range builtinProfiles {
		obj, err := client.Resource(podAutoscalerClusterProfileGVR).Get(ctx, p.Name, metav1.GetOptions{})
		require.NoError(t, err, "profile %s should exist", p.Name)
		assert.Equal(t, builtinLabelValue, obj.GetLabels()[builtinLabelKey])
	}
}

func TestBuiltinProfilesRestoredOnDrift(t *testing.T) {
	client := newFakeDynamicClient()
	mgr := NewBuiltinProfileManager(client, func() bool { return true })
	ctx := context.Background()

	mgr.reconcile(ctx)

	// Tamper with the cost profile by changing the utilization target.
	obj, err := client.Resource(podAutoscalerClusterProfileGVR).Get(ctx, "datadog-optimize-cost", metav1.GetOptions{})
	require.NoError(t, err)

	var profile datadoghq.DatadogPodAutoscalerClusterProfile
	require.NoError(t, autoscaling.FromUnstructured(obj, &profile))
	*profile.Spec.Template.Objectives[0].PodResource.Value.Utilization = 50
	profile.TypeMeta = podAutoscalerClusterProfileMeta

	tampered, err := autoscaling.ToUnstructured(&profile)
	require.NoError(t, err)
	_, err = client.Resource(podAutoscalerClusterProfileGVR).Update(ctx, tampered, metav1.UpdateOptions{})
	require.NoError(t, err)

	mgr.reconcile(ctx)

	restored, err := client.Resource(podAutoscalerClusterProfileGVR).Get(ctx, "datadog-optimize-cost", metav1.GetOptions{})
	require.NoError(t, err)

	var restoredProfile datadoghq.DatadogPodAutoscalerClusterProfile
	require.NoError(t, autoscaling.FromUnstructured(restored, &restoredProfile))
	assert.Equal(t, int32(85), *restoredProfile.Spec.Template.Objectives[0].PodResource.Value.Utilization)
}

func TestBuiltinProfilesOverwriteReservedName(t *testing.T) {
	client := newFakeDynamicClient()
	mgr := NewBuiltinProfileManager(client, func() bool { return true })
	ctx := context.Background()

	// Pre-create a profile with a reserved name but different spec and no label.
	userProfile := builtinProfiles[0].DeepCopy()
	userProfile.Labels = nil
	*userProfile.Spec.Template.Objectives[0].PodResource.Value.Utilization = 50
	obj, err := autoscaling.ToUnstructured(userProfile)
	require.NoError(t, err)
	_, err = client.Resource(podAutoscalerClusterProfileGVR).Create(ctx, obj, metav1.CreateOptions{})
	require.NoError(t, err)

	mgr.reconcile(ctx)

	// Should overwrite — names are reserved, utilization should be 85 and label restored.
	existing, err := client.Resource(podAutoscalerClusterProfileGVR).Get(ctx, "datadog-optimize-cost", metav1.GetOptions{})
	require.NoError(t, err)

	var profile datadoghq.DatadogPodAutoscalerClusterProfile
	require.NoError(t, autoscaling.FromUnstructured(existing, &profile))
	assert.Equal(t, int32(85), *profile.Spec.Template.Objectives[0].PodResource.Value.Utilization)
	assert.Equal(t, builtinLabelValue, existing.GetLabels()[builtinLabelKey])
}

func TestBuiltinNoOpOnMatchingHash(t *testing.T) {
	client := newFakeDynamicClient()
	mgr := NewBuiltinProfileManager(client, func() bool { return true })
	ctx := context.Background()

	mgr.reconcile(ctx)

	// Record the resource versions after initial creation.
	versions := map[string]string{}
	for _, p := range builtinProfiles {
		obj, err := client.Resource(podAutoscalerClusterProfileGVR).Get(ctx, p.Name, metav1.GetOptions{})
		require.NoError(t, err)
		versions[p.Name] = obj.GetResourceVersion()
	}

	mgr.reconcile(ctx)

	for _, p := range builtinProfiles {
		obj, err := client.Resource(podAutoscalerClusterProfileGVR).Get(ctx, p.Name, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, versions[p.Name], obj.GetResourceVersion(), "profile %s should not have been updated", p.Name)
	}
}
