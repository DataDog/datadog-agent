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

func TestDeploymentRolloutFactory_Name(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewDeploymentRolloutFactory(client)

	assert.Equal(t, "deployments_extended", factory.Name())
}

func TestDeploymentRolloutFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewDeploymentRolloutFactory(client)

	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.Deployment)
	assert.True(t, ok, "Expected type should be *appsv1.Deployment")
}

func TestDeploymentRolloutFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewDeploymentRolloutFactory(client)

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	generator := generators[0]
	assert.Equal(t, "kube_deployment_ongoing_rollout_duration", generator.Name)
	assert.Equal(t, "Duration of ongoing Deployment rollout in seconds", generator.Help)
	assert.Equal(t, metric.Gauge, generator.Type)
}

func TestDeploymentRolloutGeneration_OngoingRollout(t *testing.T) {

	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewDeploymentRolloutFactory(client)

	// Create deployment with ongoing rollout (generation != observed generation)
	replicas := int32(3)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 5,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 4, // Different from Generation indicates ongoing rollout
			ReadyReplicas:      2, // Less than desired replicas
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(deployment)

	// Should return dummy metric with value 1 for ongoing rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 1.0, metric.Value)
	assert.Equal(t, []string{"namespace", "deployment"}, metric.LabelKeys)
	assert.Equal(t, []string{"default", "test-deployment"}, metric.LabelValues)

	// Verify deployment was stored in map
	rolloutMutex.RLock()
	stored, exists := deploymentMap["default/test-deployment"]
	rolloutMutex.RUnlock()

	require.True(t, exists, "Deployment should be stored for ongoing rollout")
	assert.Equal(t, deployment.Name, stored.Name)
	assert.Equal(t, deployment.Namespace, stored.Namespace)
}

func TestDeploymentRolloutGeneration_CompletedRollout(t *testing.T) {

	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewDeploymentRolloutFactory(client)

	// Create deployment with completed rollout
	replicas := int32(3)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 5,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 5, // Same as Generation
			ReadyReplicas:      3, // Same as desired replicas
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(deployment)

	// Should return metric with value 0 for completed rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 0.0, metric.Value)
	assert.Equal(t, []string{"namespace", "deployment"}, metric.LabelKeys)
	assert.Equal(t, []string{"default", "test-deployment"}, metric.LabelValues)

	// Deployment should not be stored for completed rollout
	rolloutMutex.RLock()
	_, exists := deploymentMap["default/test-deployment"]
	rolloutMutex.RUnlock()

	assert.False(t, exists, "Completed deployment should not be stored")
}

func TestDeploymentRolloutGeneration_OngoingRollout_ReplicasMismatch(t *testing.T) {

	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewDeploymentRolloutFactory(client)

	// Create deployment with ongoing rollout (ready replicas != desired replicas)
	replicas := int32(5)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 3,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3, // Same as Generation
			ReadyReplicas:      2, // Less than desired replicas
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(deployment)

	// Should return dummy metric with value 1 for ongoing rollout
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 1.0, metric.Value)

	// Verify deployment was stored in map
	rolloutMutex.RLock()
	_, exists := deploymentMap["default/test-deployment"]
	rolloutMutex.RUnlock()

	require.True(t, exists, "Deployment should be stored for ongoing rollout")
}

func TestDeploymentRolloutGeneration_NilReplicas(t *testing.T) {

	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewDeploymentRolloutFactory(client)

	// Create deployment with nil replicas (defaults to 1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 3,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: nil, // nil defaults to 1
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
			ReadyReplicas:      1, // Matches default of 1
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics
	metricFamily := generators[0].Generate(deployment)

	// Should return metric with value 0 for completed rollout (nil replicas handled)
	require.Len(t, metricFamily.Metrics, 1)
	metric := metricFamily.Metrics[0]
	assert.Equal(t, 0.0, metric.Value)
}
