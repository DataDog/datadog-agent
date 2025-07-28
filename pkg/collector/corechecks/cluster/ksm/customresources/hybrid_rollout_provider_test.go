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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
)

func TestHybridRolloutProvider_StatefulSet_EventTracking(t *testing.T) {
	revision := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-sts-def456",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}

	client := fake.NewSimpleClientset(revision)
	provider := newHybridRolloutProvider(client, 30*time.Second)

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

	// Simulate event tracking by manually adding rollout state
	provider.eventTracker.activeRollouts["default/test-sts"] = &rolloutState{
		StartTime:   time.Now().Add(-3 * time.Minute),
		LastSeenAt:  time.Now(),
		CurrentRev:  "test-sts-abc123",
		UpdateRev:   "test-sts-def456",
		Source:      "event",
	}

	// Should get duration from event tracker (~3 minutes)
	duration := provider.getStatefulSetRolloutDuration(statefulSet)
	assert.InDelta(t, 180.0, duration, 10.0) // ~3 minutes

	// Verify telemetry
	eventHits, cacheHits, apiCalls := provider.getTelemetryStats()
	assert.Equal(t, int64(1), eventHits)
	assert.Equal(t, int64(0), cacheHits)
	assert.Equal(t, int64(0), apiCalls)
}

func TestHybridRolloutProvider_StatefulSet_CacheFallback(t *testing.T) {
	revision := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-sts-def456",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}

	client := fake.NewSimpleClientset(revision)
	provider := newHybridRolloutProvider(client, 30*time.Second)

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

	// Pre-populate cache
	cacheKey := "statefulset:default/test-sts:test-sts-def456"
	provider.cache.set(cacheKey, 240.0) // 4 minutes

	// Should get duration from cache
	duration := provider.getStatefulSetRolloutDuration(statefulSet)
	assert.Equal(t, 240.0, duration)

	// Verify telemetry
	eventHits, cacheHits, apiCalls := provider.getTelemetryStats()
	assert.Equal(t, int64(0), eventHits)
	assert.Equal(t, int64(1), cacheHits)
	assert.Equal(t, int64(0), apiCalls)
}

func TestHybridRolloutProvider_StatefulSet_APIFallback(t *testing.T) {
	revision := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-sts-def456",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}

	client := fake.NewSimpleClientset(revision)
	provider := newHybridRolloutProvider(client, 30*time.Second)

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

	// Should fall back to API call (~5 minutes)
	duration := provider.getStatefulSetRolloutDuration(statefulSet)
	assert.InDelta(t, 300.0, duration, 10.0) // ~5 minutes

	// Verify telemetry
	eventHits, cacheHits, apiCalls := provider.getTelemetryStats()
	assert.Equal(t, int64(0), eventHits)
	assert.Equal(t, int64(0), cacheHits)
	assert.Equal(t, int64(1), apiCalls)
}

func TestHybridRolloutProvider_Deployment_EventTracking(t *testing.T) {
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
	provider := newHybridRolloutProvider(client, 30*time.Second)

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

	// Simulate event tracking
	provider.eventTracker.activeRollouts["default/test-deployment"] = &rolloutState{
		StartTime:    time.Now().Add(-2 * time.Minute),
		LastSeenAt:   time.Now(),
		Generation:   2,
		ObservedGen:  1,
		Source:       "event",
	}

	// Should get duration from event tracker (~2 minutes)
	duration := provider.getDeploymentRolloutDuration(deployment)
	assert.InDelta(t, 120.0, duration, 10.0) // ~2 minutes

	// Verify telemetry
	eventHits, cacheHits, apiCalls := provider.getTelemetryStats()
	assert.Equal(t, int64(1), eventHits)
	assert.Equal(t, int64(0), cacheHits)
	assert.Equal(t, int64(0), apiCalls)
}

func TestHybridRolloutProvider_NoRollout(t *testing.T) {
	client := fake.NewSimpleClientset()
	provider := newHybridRolloutProvider(client, 30*time.Second)

	// StatefulSet with no rollout
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-sts-abc123",
			UpdateRevision:  "test-sts-abc123", // Same = no rollout
		},
	}

	duration := provider.getStatefulSetRolloutDuration(statefulSet)
	assert.Equal(t, float64(0), duration)

	// Deployment with no rollout
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 1,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1, // Same = no rollout
		},
	}

	duration = provider.getDeploymentRolloutDuration(deployment)
	assert.Equal(t, float64(0), duration)

	// No telemetry should be recorded for no-rollout cases
	eventHits, cacheHits, apiCalls := provider.getTelemetryStats()
	assert.Equal(t, int64(0), eventHits)
	assert.Equal(t, int64(0), cacheHits)
	assert.Equal(t, int64(0), apiCalls)
}

func TestHybridRolloutProvider_Bootstrap(t *testing.T) {
	client := fake.NewSimpleClientset()
	provider := newHybridRolloutProvider(client, 30*time.Second)

	statefulSets := []*appsv1.StatefulSet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts1",
				Namespace: "default",
			},
			Status: appsv1.StatefulSetStatus{
				CurrentRevision: "sts1-abc",
				UpdateRevision:  "sts1-def", // Rollout in progress
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts2",
				Namespace: "default",
			},
			Status: appsv1.StatefulSetStatus{
				CurrentRevision: "sts2-xyz",
				UpdateRevision:  "sts2-xyz", // No rollout
			},
		},
	}

	deployments := []*appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dep1",
				Namespace:  "default",
				Generation: 2,
			},
			Status: appsv1.DeploymentStatus{
				ObservedGeneration: 1, // Rollout in progress
			},
		},
	}

	provider.bootstrap(statefulSets, deployments)

	// Should have bootstrapped 2 rollouts (1 StatefulSet + 1 Deployment)
	assert.Equal(t, 2, provider.eventTracker.getActiveRolloutCount())

	// Verify states are set correctly
	state := provider.eventTracker.activeRollouts["default/sts1"]
	assert.NotNil(t, state)
	assert.Equal(t, "bootstrap", state.Source)
	assert.Equal(t, "sts1-abc", state.CurrentRev)
	assert.Equal(t, "sts1-def", state.UpdateRev)

	state = provider.eventTracker.activeRollouts["default/dep1"]
	assert.NotNil(t, state)
	assert.Equal(t, "bootstrap", state.Source)
	assert.Equal(t, int64(2), state.Generation)
	assert.Equal(t, int64(1), state.ObservedGen)
}

func TestHybridRolloutProvider_TelemetryReset(t *testing.T) {
	client := fake.NewSimpleClientset()
	provider := newHybridRolloutProvider(client, 30*time.Second)

	// Manually set some telemetry values
	provider.eventHits = 10
	provider.cacheHits = 20
	provider.apiCalls = 5

	eventHits, cacheHits, apiCalls := provider.getTelemetryStats()
	assert.Equal(t, int64(10), eventHits)
	assert.Equal(t, int64(20), cacheHits)
	assert.Equal(t, int64(5), apiCalls)

	provider.resetTelemetryStats()

	eventHits, cacheHits, apiCalls = provider.getTelemetryStats()
	assert.Equal(t, int64(0), eventHits)
	assert.Equal(t, int64(0), cacheHits)
	assert.Equal(t, int64(0), apiCalls)
}