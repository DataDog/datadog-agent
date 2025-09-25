// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kube-state-metrics/v2/pkg/metric"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestReplicaSetRolloutFactory_Name(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewReplicaSetRolloutFactory(client)

	assert.Equal(t, "replicasets_extended", factory.Name())
}

func TestReplicaSetRolloutFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewReplicaSetRolloutFactory(client)

	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.ReplicaSet)
	assert.True(t, ok, "Expected type should be *appsv1.ReplicaSet")
}

func TestReplicaSetRolloutFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewReplicaSetRolloutFactory(client)

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	generator := generators[0]
	assert.Equal(t, "kube_replicaset_rollout_tracker", generator.Name)
	assert.Equal(t, "Tracks ReplicaSets for deployment rollout duration calculation", generator.Help)
	assert.Equal(t, metric.Gauge, generator.Type)
}

func TestReplicaSetTracking(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	factory := NewReplicaSetRolloutFactory(client).(*replicaSetRolloutFactory)

	deploymentUID := types.UID("dep-123")
	deploymentName := "test-deployment"
	namespace := "default"

	// Create a ReplicaSet owned by a Deployment
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-rs",
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Deployment",
					Name: deploymentName,
					UID:  deploymentUID,
				},
			},
		},
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics (this should store the ReplicaSet)
	metricFamily := generators[0].Generate(rs)

	// Verify the ReplicaSet was stored
	rolloutMutex.RLock()
	rsInfo, exists := replicaSetMap[namespace+"/"+rs.Name]
	rolloutMutex.RUnlock()

	require.True(t, exists, "ReplicaSet should be stored in the map")
	assert.Equal(t, rs.Name, rsInfo.Name)
	assert.Equal(t, namespace, rsInfo.Namespace)
	assert.Equal(t, deploymentName, rsInfo.OwnerName)
	assert.Equal(t, string(deploymentUID), rsInfo.OwnerUID)

	// Verify empty metric family is returned (we don't emit actual metrics)
	assert.Len(t, metricFamily.Metrics, 0)
}
