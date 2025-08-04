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

func TestGetStatefulSetRolloutStartTime(t *testing.T) {
	now := time.Now()
	oldTime := now.Add(-5 * time.Minute)

	tests := []struct {
		name                string
		statefulSet         *appsv1.StatefulSet
		controllerRevisions []*appsv1.ControllerRevision
		expectedIsZero      bool
		description         string
	}{
		{
			name: "completed rollout - generation match and all replicas ready",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-statefulset",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 2,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      3,
					UpdatedReplicas:    3,
				},
			},
			controllerRevisions: []*appsv1.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-statefulset-abc123",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(oldTime),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "StatefulSet",
								Name: "test-statefulset",
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
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-statefulset",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 3,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      2,
					UpdatedReplicas:    2,
				},
			},
			controllerRevisions: []*appsv1.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-statefulset-def456",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(now),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "StatefulSet",
								Name: "test-statefulset",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-statefulset-abc123",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(oldTime),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "StatefulSet",
								Name: "test-statefulset",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
			},
			expectedIsZero: false,
			description:    "Should return newest ControllerRevision creation time for ongoing rollout",
		},
		{
			name: "ongoing rollout - not all replicas updated",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-statefulset",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 2,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      3,
					UpdatedReplicas:    1,
				},
			},
			controllerRevisions: []*appsv1.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-statefulset-abc123",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(oldTime),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "StatefulSet",
								Name: "test-statefulset",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
			},
			expectedIsZero: false,
			description:    "Should return ControllerRevision creation time when not all replicas updated",
		},
		{
			name: "ongoing rollout - not all replicas ready",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-statefulset",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 2,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      1,
					UpdatedReplicas:    3,
				},
			},
			controllerRevisions: []*appsv1.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-statefulset-abc123",
						Namespace:         "default",
						CreationTimestamp: metav1.NewTime(oldTime),
						Labels:            map[string]string{"app": "test"},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "StatefulSet",
								Name: "test-statefulset",
								UID:  types.UID("test-uid"),
							},
						},
					},
				},
			},
			expectedIsZero: false,
			description:    "Should return ControllerRevision creation time when not all replicas ready",
		},
		{
			name: "no controller revisions found",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-statefulset",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 2,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      0,
					UpdatedReplicas:    0,
				},
			},
			controllerRevisions: []*appsv1.ControllerRevision{},
			expectedIsZero:      true,
			description:         "Should return zero time when no ControllerRevisions found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			fakeClient := fake.NewSimpleClientset()

			// Add ControllerRevisions to fake client
			for _, cr := range tt.controllerRevisions {
				_, err := fakeClient.AppsV1().ControllerRevisions(cr.Namespace).Create(context.TODO(), cr, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			// Create factory
			factory := &extendedStatefulSetFactory{
				client: fakeClient,
			}

			// Test getRolloutStartTime
			result := factory.getRolloutStartTime(tt.statefulSet)

			if tt.expectedIsZero {
				assert.True(t, result.IsZero(), tt.description)
			} else {
				assert.False(t, result.IsZero(), tt.description)
				// Should return the newest ControllerRevision creation time
				if len(tt.controllerRevisions) > 0 {
					expectedTime := tt.controllerRevisions[0].CreationTimestamp.Time
					for _, cr := range tt.controllerRevisions[1:] {
						if cr.CreationTimestamp.Time.After(expectedTime) {
							expectedTime = cr.CreationTimestamp.Time
						}
					}
					assert.Equal(t, expectedTime, result, "Should return newest ControllerRevision creation time")
				}
			}
		})
	}
}

func TestIsOwnedByStatefulSet(t *testing.T) {
	statefulSetUID := types.UID("statefulset-uid")
	otherUID := types.UID("other-uid")

	tests := []struct {
		name               string
		controllerRevision *appsv1.ControllerRevision
		statefulSet        *appsv1.StatefulSet
		expectedOwned      bool
		description        string
	}{
		{
			name: "owned by statefulset",
			controllerRevision: &appsv1.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cr",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "StatefulSet",
							Name: "test-statefulset",
							UID:  statefulSetUID,
						},
					},
				},
			},
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-statefulset",
					Namespace: "default",
					UID:       statefulSetUID,
				},
			},
			expectedOwned: true,
			description:   "Should return true when ControllerRevision is owned by statefulset",
		},
		{
			name: "not owned by statefulset - different UID",
			controllerRevision: &appsv1.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cr",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "StatefulSet",
							Name: "test-statefulset",
							UID:  otherUID,
						},
					},
				},
			},
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-statefulset",
					Namespace: "default",
					UID:       statefulSetUID,
				},
			},
			expectedOwned: false,
			description:   "Should return false when ControllerRevision has different owner UID",
		},
		{
			name: "not owned by statefulset - different name",
			controllerRevision: &appsv1.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cr",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "StatefulSet",
							Name: "other-statefulset",
							UID:  statefulSetUID,
						},
					},
				},
			},
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-statefulset",
					Namespace: "default",
					UID:       statefulSetUID,
				},
			},
			expectedOwned: false,
			description:   "Should return false when ControllerRevision has different owner name",
		},
		{
			name: "no owner references",
			controllerRevision: &appsv1.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cr",
					Namespace: "default",
				},
			},
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-statefulset",
					Namespace: "default",
					UID:       statefulSetUID,
				},
			},
			expectedOwned: false,
			description:   "Should return false when ControllerRevision has no owner references",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create factory
			factory := &extendedStatefulSetFactory{
				client: fake.NewSimpleClientset(),
			}

			// Test isOwnedByStatefulSet
			result := factory.isOwnedByStatefulSet(tt.controllerRevision, tt.statefulSet)

			assert.Equal(t, tt.expectedOwned, result, tt.description)
		})
	}
}

func TestGetDesiredStatefulSetReplicas(t *testing.T) {
	tests := []struct {
		name             string
		statefulSet      *appsv1.StatefulSet
		expectedReplicas int32
		description      string
	}{
		{
			name: "explicit replica count",
			statefulSet: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: int32Ptr(5),
				},
			},
			expectedReplicas: 5,
			description:      "Should return explicit replica count",
		},
		{
			name: "nil replica count - default to 1",
			statefulSet: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: nil,
				},
			},
			expectedReplicas: 1,
			description:      "Should default to 1 when replica count is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDesiredStatefulSetReplicas(tt.statefulSet)
			assert.Equal(t, tt.expectedReplicas, result, tt.description)
		})
	}
}