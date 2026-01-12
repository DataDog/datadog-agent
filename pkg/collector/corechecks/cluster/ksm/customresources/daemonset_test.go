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

func TestDaemonSetRolloutFactory_Name(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewDaemonSetRolloutFactory(client, tracker)

	assert.Equal(t, "daemonsets_extended", factory.Name())
}

func TestDaemonSetRolloutFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewDaemonSetRolloutFactory(client, tracker)

	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.DaemonSet)
	assert.True(t, ok, "Expected type should be *appsv1.DaemonSet")
}

func TestDaemonSetRolloutFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewDaemonSetRolloutFactory(client, tracker)

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	generator := generators[0]
	assert.Equal(t, "kube_daemonset_ongoing_rollout_duration", generator.Name)
	assert.Equal(t, "Duration of ongoing DaemonSet rollout in seconds", generator.Help)
	assert.Equal(t, metric.Gauge, generator.Type)
}

func TestDaemonSetRolloutTracking_OngoingRollout(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewDaemonSetRolloutFactory(client, tracker).(*daemonSetRolloutFactory)

	namespace := "default"
	daemonSetName := "test-daemonset"

	// Create a DaemonSet with ongoing rollout (updatedNumberScheduled < desiredNumberScheduled)
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonSetName,
			Namespace: namespace,
		},
		Spec: appsv1.DaemonSetSpec{},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			UpdatedNumberScheduled: 2,
			ObservedGeneration:     2,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics (this should detect ongoing rollout)
	metricFamily := generators[0].Generate(ds)

	require.NotNil(t, metricFamily)
	require.Len(t, metricFamily.Metrics, 1)

	metric := metricFamily.Metrics[0]
	assert.Equal(t, 1.0, metric.Value) // Should indicate ongoing rollout
	assert.Contains(t, metric.LabelKeys, "namespace")
	assert.Contains(t, metric.LabelKeys, "daemonset")
	assert.Contains(t, metric.LabelValues, namespace)
	assert.Contains(t, metric.LabelValues, daemonSetName)
}

func TestDaemonSetRolloutTracking_CompletedRollout(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewDaemonSetRolloutFactory(client, tracker).(*daemonSetRolloutFactory)

	namespace := "default"
	daemonSetName := "test-daemonset"

	// Create a DaemonSet with completed rollout
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       daemonSetName,
			Namespace:  namespace,
			Generation: 2,
		},
		Spec: appsv1.DaemonSetSpec{},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			UpdatedNumberScheduled: 3,
			NumberAvailable:        3,
			ObservedGeneration:     2,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics (this should detect completed rollout)
	metricFamily := generators[0].Generate(ds)

	require.NotNil(t, metricFamily)
	require.Len(t, metricFamily.Metrics, 1)

	metric := metricFamily.Metrics[0]
	assert.Equal(t, 0.0, metric.Value) // Should indicate completed rollout
	assert.Contains(t, metric.LabelKeys, "namespace")
	assert.Contains(t, metric.LabelKeys, "daemonset")
	assert.Contains(t, metric.LabelValues, namespace)
	assert.Contains(t, metric.LabelValues, daemonSetName)
}
