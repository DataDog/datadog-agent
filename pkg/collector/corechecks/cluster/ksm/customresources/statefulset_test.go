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
	factory := NewStatefulSetRolloutFactory(client)

	assert.Equal(t, "apps/v1, Resource=statefulsets_rollout", factory.Name())
}

func TestStatefulSetRolloutFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.StatefulSet)
	assert.True(t, ok, "Expected type should be *appsv1.StatefulSet")
}

func TestStatefulSetRolloutFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	generator := generators[0]
	assert.Equal(t, "kube_statefulset_ongoing_rollout_duration", generator.Name)
	assert.Equal(t, "Duration of ongoing StatefulSet rollout in seconds", generator.Help)
	assert.Equal(t, metric.Gauge, generator.Type)
}

func TestStatefulSetRolloutGeneration_OngoingRollout_RevisionMismatch(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	// Create StatefulSet with ongoing rollout (revision mismatch)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{3}[0],
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-statefulset-abc123",
			UpdateRevision:  "test-statefulset-def456", // Different revision indicates rollout
			ReadyReplicas:   3,
			Replicas:        3,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(statefulSet)

	// Should return dummy metric with value 1 for ongoing rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 1.0, metric.Value)
	assert.Equal(t, []string{"namespace", "statefulset"}, metric.LabelKeys)
	assert.Equal(t, []string{"default", "test-statefulset"}, metric.LabelValues)
}

func TestStatefulSetRolloutGeneration_OngoingRollout_ReplicasMismatch(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	// Create StatefulSet with ongoing rollout (replica mismatch)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{5}[0],
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-statefulset-abc123",
			UpdateRevision:  "test-statefulset-abc123", // Same revision
			ReadyReplicas:   3,                          // Less than total replicas
			Replicas:        5,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(statefulSet)

	// Should return dummy metric with value 1 for ongoing rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 1.0, metric.Value)
	assert.Equal(t, []string{"namespace", "statefulset"}, metric.LabelKeys)
	assert.Equal(t, []string{"default", "test-statefulset"}, metric.LabelValues)
}

func TestStatefulSetRolloutGeneration_CompletedRollout(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	// Create StatefulSet with completed rollout
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{3}[0],
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-statefulset-abc123",
			UpdateRevision:  "test-statefulset-abc123", // Same revision
			ReadyReplicas:   3,                          // Same as total replicas
			Replicas:        3,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(statefulSet)

	// Should return metric with value 0 for completed rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 0.0, metric.Value)
	assert.Equal(t, []string{"namespace", "statefulset"}, metric.LabelKeys)
	assert.Equal(t, []string{"default", "test-statefulset"}, metric.LabelValues)
}

func TestStatefulSetRolloutGeneration_EmptyRevisions(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	// Create StatefulSet with empty revisions (should be considered completed)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{3}[0],
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "", // Empty revision
			UpdateRevision:  "", // Empty revision
			ReadyReplicas:   3,
			Replicas:        3,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(statefulSet)

	// Should return metric with value 0 for completed rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 0.0, metric.Value)
}

func TestStatefulSetRolloutGeneration_OnlyUpdateRevisionSet(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	// Create StatefulSet with only update revision set (initial rollout)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{3}[0],
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "",                          // Empty
			UpdateRevision:  "test-statefulset-abc123",   // Set - indicates rollout in progress
			ReadyReplicas:   1,
			Replicas:        3,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(statefulSet)

	// Should return dummy metric with value 1 for ongoing rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 1.0, metric.Value)
	assert.Equal(t, []string{"namespace", "statefulset"}, metric.LabelKeys)
	assert.Equal(t, []string{"default", "test-statefulset"}, metric.LabelValues)
}

func TestStatefulSetRolloutGeneration_ZeroReplicas(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewStatefulSetRolloutFactory(client)

	// Create StatefulSet with zero replicas
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{0}[0],
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-statefulset-abc123",
			UpdateRevision:  "test-statefulset-abc123", // Same revision
			ReadyReplicas:   0,                          // Same as total replicas (0)
			Replicas:        0,
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(statefulSet)

	// Should return metric with value 0 for completed rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 0.0, metric.Value)
}