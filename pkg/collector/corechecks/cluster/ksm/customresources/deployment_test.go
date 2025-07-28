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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestDeploymentRolloutFactory_getRolloutDuration(t *testing.T) {
	deploymentUID := types.UID("test-deployment-uid")
	
	tests := []struct {
		name           string
		deployment     *appsv1.Deployment
		replicaSets    []runtime.Object
		expectedResult float64
		expectZero     bool
	}{
		{
			name: "no rollout in progress - generations match",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-deployment",
					Namespace:  "default",
					UID:        deploymentUID,
					Generation: 1,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
				},
			},
			expectedResult: 0,
			expectZero:     true,
		},
		{
			name: "rollout in progress with ReplicaSet",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-deployment",
					Namespace:  "default",
					UID:        deploymentUID,
					Generation: 2,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
				},
			},
			replicaSets: []runtime.Object{
				&appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-deployment-old",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
						Labels: map[string]string{
							"app": "test",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "Deployment",
								Name: "test-deployment",
								UID:  deploymentUID,
							},
						},
					},
				},
				&appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-deployment-new",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
						Labels: map[string]string{
							"app": "test",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "Deployment",
								Name: "test-deployment",
								UID:  deploymentUID,
							},
						},
					},
				},
			},
			expectedResult: 300, // 5 minutes in seconds (newest ReplicaSet)
			expectZero:     false,
		},
		{
			name: "rollout in progress but no matching ReplicaSets",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-deployment",
					Namespace:  "default",
					UID:        deploymentUID,
					Generation: 2,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
				},
			},
			replicaSets:    []runtime.Object{},
			expectedResult: 0,
			expectZero:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(tt.replicaSets...)
			factory := &deploymentRolloutFactory{
				hybridProvider: newHybridRolloutProvider(client, 30*time.Second),
			}

			result := factory.hybridProvider.getDeploymentRolloutDuration(tt.deployment)

			if tt.expectZero {
				assert.Equal(t, float64(0), result)
			} else {
				// Allow some tolerance for time differences in tests
				assert.InDelta(t, tt.expectedResult, result, 10.0) // 10 second tolerance
			}
		})
	}
}

func TestIsOwnedByDeployment(t *testing.T) {
	deploymentUID := types.UID("test-deployment-uid")
	
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
			UID:  deploymentUID,
		},
	}

	client := fake.NewSimpleClientset()
	provider := newHybridRolloutProvider(client, 30*time.Second)

	tests := []struct {
		name     string
		rs       *appsv1.ReplicaSet
		expected bool
	}{
		{
			name: "owned by deployment",
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Deployment",
							Name: "test-deployment",
							UID:  deploymentUID,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "not owned by deployment - different name",
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Deployment",
							Name: "other-deployment",
							UID:  deploymentUID,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "not owned by deployment - different UID",
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Deployment",
							Name: "test-deployment",
							UID:  types.UID("different-uid"),
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "not owned by deployment - different kind",
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "StatefulSet",
							Name: "test-deployment",
							UID:  deploymentUID,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "no owner references",
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.isOwnedByDeployment(tt.rs, deployment)
			assert.Equal(t, tt.expected, result)
		})
	}
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
}

func TestDeploymentRolloutFactory_Name(t *testing.T) {
	factory := &deploymentRolloutFactory{}
	assert.Equal(t, "apps/v1, Resource=deployments_rollout", factory.Name())
}

func TestDeploymentRolloutFactory_ExpectedType(t *testing.T) {
	factory := &deploymentRolloutFactory{}
	expectedType := factory.ExpectedType()
	_, ok := expectedType.(*appsv1.Deployment)
	assert.True(t, ok)
}

func TestDeploymentRolloutFactory_Caching(t *testing.T) {
	deploymentUID := types.UID("test-deployment-uid")
	
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-deployment-abc123",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
			Labels: map[string]string{
				"app": "test",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Deployment",
					Name: "test-deployment",
					UID:  deploymentUID,
				},
			},
		},
	}

	client := fake.NewSimpleClientset(replicaSet)
	factory := &deploymentRolloutFactory{
		hybridProvider: newHybridRolloutProvider(client, 30*time.Second),
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			UID:        deploymentUID,
			Generation: 2,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
		},
	}

	// First call should trigger API call
	result1 := factory.hybridProvider.getDeploymentRolloutDuration(deployment)
	assert.InDelta(t, 300.0, result1, 10.0) // ~5 minutes

	// Second call should use cache (verify by checking it's the same exact value)
	result2 := factory.hybridProvider.getDeploymentRolloutDuration(deployment)
	assert.Equal(t, result1, result2)

	// Cache should have one entry
	assert.Equal(t, 1, factory.hybridProvider.cache.size())
}