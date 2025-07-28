// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestStatefulSetRolloutFactory_getRolloutDuration(t *testing.T) {
	tests := []struct {
		name           string
		statefulSet    *appsv1.StatefulSet
		revisions      []runtime.Object
		expectedResult float64
		expectZero     bool
	}{
		{
			name: "no rollout in progress - revisions match",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts",
					Namespace: "default",
				},
				Status: appsv1.StatefulSetStatus{
					CurrentRevision: "test-sts-abc123",
					UpdateRevision:  "test-sts-abc123",
				},
			},
			expectedResult: 0,
			expectZero:     true,
		},
		{
			name: "rollout in progress with ControllerRevision",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts",
					Namespace: "default",
				},
				Status: appsv1.StatefulSetStatus{
					CurrentRevision: "test-sts-abc123",
					UpdateRevision:  "test-sts-def456",
				},
			},
			revisions: []runtime.Object{
				&appsv1.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-sts-def456",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
					},
				},
			},
			expectedResult: 300, // 5 minutes in seconds
			expectZero:     false,
		},
		{
			name: "rollout in progress but no update revision",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts",
					Namespace: "default",
				},
				Status: appsv1.StatefulSetStatus{
					CurrentRevision: "test-sts-abc123",
					UpdateRevision:  "",
				},
			},
			expectedResult: 0,
			expectZero:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(tt.revisions...)
			factory := &statefulSetRolloutFactory{
				client: client,
				cache:  newRolloutCache(30 * time.Second),
			}

			result := factory.getRolloutDuration(tt.statefulSet)

			if tt.expectZero {
				assert.Equal(t, float64(0), result)
			} else {
				// Allow some tolerance for time differences in tests
				assert.InDelta(t, tt.expectedResult, result, 10.0) // 10 second tolerance
			}
		})
	}
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
}

func TestStatefulSetRolloutFactory_Name(t *testing.T) {
	factory := &statefulSetRolloutFactory{}
	assert.Equal(t, "apps/v1, Resource=statefulsets_rollout", factory.Name())
}

func TestStatefulSetRolloutFactory_ExpectedType(t *testing.T) {
	factory := &statefulSetRolloutFactory{}
	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.StatefulSet)
	assert.True(t, ok)
}

func TestStatefulSetRolloutFactory_Caching(t *testing.T) {
	revision := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-sts-def456",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}

	client := fake.NewSimpleClientset(revision)
	factory := &statefulSetRolloutFactory{
		client: client,
		cache:  newRolloutCache(30 * time.Second),
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-sts-abc123",
			UpdateRevision:  "test-sts-def456",
		},
	}

	// First call should trigger API call
	result1 := factory.getRolloutDuration(statefulSet)
	assert.InDelta(t, 300.0, result1, 10.0) // ~5 minutes

	// Second call should use cache (verify by checking it's the same exact value)
	result2 := factory.getRolloutDuration(statefulSet)
	assert.Equal(t, result1, result2)

	// Cache should have one entry
	assert.Equal(t, 1, factory.cache.size())
}