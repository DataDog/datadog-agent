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

func TestControllerRevisionRolloutFactory_Name(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewControllerRevisionRolloutFactory(client, tracker)

	assert.Equal(t, "controllerrevisions", factory.Name())
}

func TestControllerRevisionRolloutFactory_ExpectedType(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewControllerRevisionRolloutFactory(client, tracker)

	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.ControllerRevision)
	assert.True(t, ok, "Expected type should be *appsv1.ControllerRevision")
}

func TestControllerRevisionRolloutFactory_MetricFamilyGenerators(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewControllerRevisionRolloutFactory(client, tracker)

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	generator := generators[0]
	assert.Equal(t, "kube_controllerrevision_rollout_tracker", generator.Name)
	assert.Equal(t, "Tracks ControllerRevisions for StatefulSet rollout duration calculation", generator.Help)
	assert.Equal(t, metric.Gauge, generator.Type)
}

func TestControllerRevisionTracking(t *testing.T) {
	client := &apiserver.APIClient{
		Cl: fake.NewSimpleClientset(),
	}
	tracker := NewRolloutTracker()
	factory := NewControllerRevisionRolloutFactory(client, tracker).(*controllerRevisionRolloutFactory)

	statefulSetUID := types.UID("sts-123")
	statefulSetName := "test-statefulset"
	namespace := "default"

	// Create a ControllerRevision owned by a StatefulSet
	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-sts-revision-1",
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "StatefulSet",
					Name: statefulSetName,
					UID:  statefulSetUID,
				},
			},
		},
		Revision: 1,
	}

	generators := factory.MetricFamilyGenerators()
	require.Len(t, generators, 1)

	// Generate metrics (this should store the ControllerRevision)
	metricFamily := generators[0].Generate(cr)

	// Verify the ControllerRevision was stored (indirectly by ensuring it was processed)
	// Since ControllerRevisions are stored during metric generation, we can verify by checking
	// that the factory processed the ControllerRevision without error
	require.NotNil(t, metricFamily, "MetricFamily should be generated successfully, indicating ControllerRevision was processed")

	// Verify empty metric family is returned (we don't emit actual metrics)
	assert.Len(t, metricFamily.Metrics, 0)
}
