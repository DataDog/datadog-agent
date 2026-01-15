// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kube-state-metrics/v2/pkg/metric"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestStatefulSetRolloutFactory_Name(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewStatefulSetRolloutFactory(client, tracker)

	assert.Equal(t, "statefulsets_extended", factory.Name())
}

func TestStatefulSetRolloutFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewStatefulSetRolloutFactory(client, tracker)

	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.StatefulSet)
	assert.True(t, ok, "Expected type should be *appsv1.StatefulSet")
}

func TestStatefulSetRolloutFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewStatefulSetRolloutFactory(client, tracker)

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	generator := generators[0]
	assert.Equal(t, "kube_statefulset_ongoing_rollout_duration", generator.Name)
	assert.Equal(t, "Duration of ongoing StatefulSet rollout in seconds", generator.Help)
	assert.Equal(t, metric.Gauge, generator.Type)
}

func TestStatefulSetRolloutTracking_OngoingRollout(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewStatefulSetRolloutFactory(client, tracker).(*statefulSetRolloutFactory)

	namespace := "default"
	statefulSetName := "test-statefulset"
	replicas := int32(3)

	// Create a StatefulSet with ongoing rollout (generation != observedGeneration)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 2, // New generation
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1, // Old generation - indicates rollout in progress
			ReadyReplicas:      2, // Not all replicas ready
			UpdateRevision:     "rev-2",
			CurrentRevision:    "rev-1", // Different revisions indicate rollout
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics (this should detect ongoing rollout)
	metricFamily := generators[0].Generate(sts)

	require.NotNil(t, metricFamily)
	require.Len(t, metricFamily.Metrics, 1)

	metric := metricFamily.Metrics[0]
	assert.Equal(t, 1.0, metric.Value) // Should indicate ongoing rollout
	assert.Contains(t, metric.LabelKeys, "namespace")
	assert.Contains(t, metric.LabelKeys, "statefulset")
	assert.Contains(t, metric.LabelValues, namespace)
	assert.Contains(t, metric.LabelValues, statefulSetName)
}

func TestStatefulSetRolloutTracking_CompletedRollout(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewStatefulSetRolloutFactory(client, tracker).(*statefulSetRolloutFactory)

	namespace := "default"
	statefulSetName := "test-statefulset"
	replicas := int32(3)

	// Create a StatefulSet with completed rollout
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 2,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2, // Same generation - rollout complete
			ReadyReplicas:      3, // All replicas ready
			UpdateRevision:     "rev-2",
			CurrentRevision:    "rev-2", // Same revisions indicate completion
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics (this should detect completed rollout)
	metricFamily := generators[0].Generate(sts)

	require.NotNil(t, metricFamily)
	require.Len(t, metricFamily.Metrics, 1)

	metric := metricFamily.Metrics[0]
	assert.Equal(t, 0.0, metric.Value) // Should indicate completed rollout
	assert.Contains(t, metric.LabelKeys, "namespace")
	assert.Contains(t, metric.LabelKeys, "statefulset")
	assert.Contains(t, metric.LabelValues, namespace)
	assert.Contains(t, metric.LabelValues, statefulSetName)
}
