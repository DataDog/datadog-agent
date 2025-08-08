// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetRolloutStartTime(t *testing.T) {
	now := time.Now()
	oldTime := now.Add(-5 * time.Minute)

	tests := []struct {
		name           string
		deployment     *appsv1.Deployment
		replicaSets    []*appsv1.ReplicaSet
		expectedIsZero bool
		description    string
	}{
		{
			name: "completed rollout - generation match and ready",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-deployment",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 2,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      3,
				},
			},
			replicaSets: []*appsv1.ReplicaSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-deployment-abc123",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(oldTime),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "Deployment",
								Name: "test-deployment",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
			},
			expectedIsZero: true,
			description:    "Should return zero time for completed rollout",
		},
		{
			name: "ongoing rollout - generation mismatch",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-deployment",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 3,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      2,
				},
			},
			replicaSets: []*appsv1.ReplicaSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-deployment-def456",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(now),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "Deployment",
								Name: "test-deployment",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-deployment-abc123",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(oldTime),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "Deployment",
								Name: "test-deployment",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
			},
			expectedIsZero: false,
			description:    "Should return newest ReplicaSet creation time for ongoing rollout",
		},
		{
			name: "ongoing rollout - not all replicas ready",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-deployment",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 2,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      1,
				},
			},
			replicaSets: []*appsv1.ReplicaSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-deployment-abc123",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(oldTime),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "Deployment",
								Name: "test-deployment",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
			},
			expectedIsZero: false,
			description:    "Should return ReplicaSet creation time when not all replicas ready",
		},
		{
			name: "no replicasets found",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-deployment",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 2,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      0,
				},
			},
			replicaSets:    []*appsv1.ReplicaSet{},
			expectedIsZero: true,
			description:    "Should return zero time when no ReplicaSets found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			fakeClient := fake.NewSimpleClientset()

			// Add ReplicaSets to fake client
			for _, rs := range tt.replicaSets {
				_, err := fakeClient.AppsV1().ReplicaSets(rs.Namespace).Create(context.TODO(), rs, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			// Create factory
			factory := &extendedDeploymentFactory{
				client: fakeClient,
			}

			// Test getRolloutStartTime
			result := factory.getRolloutStartTime(tt.deployment)

			if tt.expectedIsZero {
				assert.True(t, result.IsZero(), tt.description)
			} else {
				assert.False(t, result.IsZero(), tt.description)
				// Should return the newest ReplicaSet creation time
				if len(tt.replicaSets) > 0 {
					expectedTime := tt.replicaSets[0].CreationTimestamp.Time
					for _, rs := range tt.replicaSets[1:] {
						if rs.CreationTimestamp.Time.After(expectedTime) {
							expectedTime = rs.CreationTimestamp.Time
						}
					}
					assert.Equal(t, expectedTime, result, "Should return newest ReplicaSet creation time")
				}
			}
		})
	}
}

func TestIsOwnedByDeployment(t *testing.T) {
	deploymentUID := types.UID("deployment-uid")
	otherUID := types.UID("other-uid")

	tests := []struct {
		name          string
		replicaSet    *appsv1.ReplicaSet
		deployment    *appsv1.Deployment
		expectedOwned bool
		description   string
	}{
		{
			name: "owned by deployment",
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Deployment",
							Name: "test-deployment",
							UID:  deploymentUID,
						},
					},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
					UID:       deploymentUID,
				},
			},
			expectedOwned: true,
			description:   "Should return true when ReplicaSet is owned by deployment",
		},
		{
			name: "not owned by deployment - different UID",
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Deployment",
							Name: "test-deployment",
							UID:  otherUID,
						},
					},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
					UID:       deploymentUID,
				},
			},
			expectedOwned: false,
			description:   "Should return false when ReplicaSet has different owner UID",
		},
		{
			name: "not owned by deployment - different name",
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Deployment",
							Name: "other-deployment",
							UID:  deploymentUID,
						},
					},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
					UID:       deploymentUID,
				},
			},
			expectedOwned: false,
			description:   "Should return false when ReplicaSet has different owner name",
		},
		{
			name: "no owner references",
			replicaSet: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "default",
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
					UID:       deploymentUID,
				},
			},
			expectedOwned: false,
			description:   "Should return false when ReplicaSet has no owner references",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create factory
			factory := &extendedDeploymentFactory{
				client: fake.NewSimpleClientset(),
			}

			// Test isOwnedByDeployment
			result := factory.isOwnedByDeployment(tt.replicaSet, tt.deployment)

			assert.Equal(t, tt.expectedOwned, result, tt.description)
		})
	}
}

func TestGetDesiredReplicas(t *testing.T) {
	tests := []struct {
		name             string
		deployment       *appsv1.Deployment
		expectedReplicas int32
		description      string
	}{
		{
			name: "explicit replica count",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(5),
				},
			},
			expectedReplicas: 5,
			description:      "Should return explicit replica count",
		},
		{
			name: "nil replica count - default to 1",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: nil,
				},
			},
			expectedReplicas: 1,
			description:      "Should default to 1 when replica count is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDesiredReplicas(tt.deployment)
			assert.Equal(t, tt.expectedReplicas, result, tt.description)
		})
	}
}

// Helper function to create int32 pointers
func int32Ptr(i int32) *int32 {
	return &i
}
