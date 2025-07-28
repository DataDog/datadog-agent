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

	"github.com/stretchr/testify/assert"
)

func TestRolloutEventTracker_StatefulSetRolloutLifecycle(t *testing.T) {
	tracker := newRolloutEventTracker()

	// Initial StatefulSet - no rollout
	oldSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-sts-abc123",
			UpdateRevision:  "test-sts-abc123",
		},
	}

	// StatefulSet with rollout started
	newSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-sts-abc123",
			UpdateRevision:  "test-sts-def456", // New revision
		},
	}

	// Simulate rollout start event
	tracker.onStatefulSetUpdate(oldSts, newSts)

	// Should track the rollout
	assert.Equal(t, 1, len(tracker.activeRollouts))
	
	duration, found := tracker.getStatefulSetRolloutDuration(newSts)
	assert.True(t, found)
	assert.Greater(t, duration, 0.0)

	// Simulate rollout completion
	completedSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-sts-def456", // Updated
			UpdateRevision:  "test-sts-def456", // Same = completed
		},
	}

	tracker.onStatefulSetUpdate(newSts, completedSts)

	// Should remove from tracking
	assert.Equal(t, 0, len(tracker.activeRollouts))
	
	duration, found = tracker.getStatefulSetRolloutDuration(completedSts)
	assert.False(t, found)
	assert.Equal(t, 0.0, duration)
}

func TestRolloutEventTracker_DeploymentRolloutLifecycle(t *testing.T) {
	tracker := newRolloutEventTracker()

	// Initial Deployment - no rollout
	oldDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 1,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
		},
	}

	// Deployment with rollout started (generation incremented)
	newDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 2, // Incremented
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1, // Not yet observed
		},
	}

	// Simulate rollout start event
	tracker.onDeploymentUpdate(oldDep, newDep)

	// Should track the rollout
	assert.Equal(t, 1, len(tracker.activeRollouts))
	
	duration, found := tracker.getDeploymentRolloutDuration(newDep)
	assert.True(t, found)
	assert.Greater(t, duration, 0.0)

	// Simulate rollout completion
	completedDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2, // Now observed = completed
		},
	}

	tracker.onDeploymentUpdate(newDep, completedDep)

	// Should remove from tracking
	assert.Equal(t, 0, len(tracker.activeRollouts))
	
	duration, found = tracker.getDeploymentRolloutDuration(completedDep)
	assert.False(t, found)
	assert.Equal(t, 0.0, duration)
}

func TestRolloutEventTracker_StatefulSetDelete(t *testing.T) {
	tracker := newRolloutEventTracker()

	// Add a rollout manually
	tracker.activeRollouts["default/test-sts"] = &rolloutState{
		StartTime:   time.Now(),
		LastSeenAt:  time.Now(),
		CurrentRev:  "test-sts-abc123",
		UpdateRev:   "test-sts-def456",
		Source:      "event",
	}

	assert.Equal(t, 1, len(tracker.activeRollouts))

	// Simulate StatefulSet deletion
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
	}

	tracker.onStatefulSetDelete(sts)

	// Should remove from tracking
	assert.Equal(t, 0, len(tracker.activeRollouts))
}

func TestRolloutEventTracker_DeploymentDelete(t *testing.T) {
	tracker := newRolloutEventTracker()

	// Add a rollout manually
	tracker.activeRollouts["default/test-deployment"] = &rolloutState{
		StartTime:    time.Now(),
		LastSeenAt:   time.Now(),
		Generation:   2,
		ObservedGen:  1,
		Source:       "event",
	}

	assert.Equal(t, 1, len(tracker.activeRollouts))

	// Simulate Deployment deletion
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
	}

	tracker.onDeploymentDelete(dep)

	// Should remove from tracking
	assert.Equal(t, 0, len(tracker.activeRollouts))
}

func TestRolloutEventTracker_StateValidation(t *testing.T) {
	tracker := newRolloutEventTracker()

	// Add a rollout with specific state
	tracker.activeRollouts["default/test-sts"] = &rolloutState{
		StartTime:   time.Now().Add(-5 * time.Minute),
		LastSeenAt:  time.Now(),
		CurrentRev:  "test-sts-abc123",
		UpdateRev:   "test-sts-def456",
		Source:      "event",
	}

	// StatefulSet with matching state
	matchingSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-sts-abc123",
			UpdateRevision:  "test-sts-def456",
		},
	}

	duration, found := tracker.getStatefulSetRolloutDuration(matchingSts)
	assert.True(t, found)
	assert.InDelta(t, 300.0, duration, 10.0) // ~5 minutes

	// StatefulSet with mismatched state
	mismatchedSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "test-sts-xyz789", // Different
			UpdateRevision:  "test-sts-def456",
		},
	}

	duration, found = tracker.getStatefulSetRolloutDuration(mismatchedSts)
	assert.False(t, found)
	assert.Equal(t, 0.0, duration)
}

func TestRolloutEventTracker_Bootstrap(t *testing.T) {
	tracker := newRolloutEventTracker()

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
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts3",
				Namespace: "default",
			},
			Status: appsv1.StatefulSetStatus{
				CurrentRevision: "sts3-old",
				UpdateRevision:  "", // No update revision
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
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dep2",
				Namespace:  "default",
				Generation: 3,
			},
			Status: appsv1.DeploymentStatus{
				ObservedGeneration: 3, // No rollout
			},
		},
	}

	tracker.bootstrapExistingRollouts(statefulSets, deployments)

	// Should bootstrap only the resources with rollouts in progress
	assert.Equal(t, 2, len(tracker.activeRollouts))

	// Check StatefulSet rollout
	state := tracker.activeRollouts["default/sts1"]
	assert.NotNil(t, state)
	assert.Equal(t, "bootstrap", state.Source)
	assert.Equal(t, "sts1-abc", state.CurrentRev)
	assert.Equal(t, "sts1-def", state.UpdateRev)

	// Check Deployment rollout
	state = tracker.activeRollouts["default/dep1"]
	assert.NotNil(t, state)
	assert.Equal(t, "bootstrap", state.Source)
	assert.Equal(t, int64(2), state.Generation)
	assert.Equal(t, int64(1), state.ObservedGen)
}

func TestRolloutEventTracker_Cleanup(t *testing.T) {
	tracker := newRolloutEventTracker()

	now := time.Now()

	// Add fresh rollout
	tracker.activeRollouts["default/fresh"] = &rolloutState{
		StartTime:   now.Add(-1 * time.Minute),
		LastSeenAt:  now, // Fresh
		CurrentRev:  "fresh-abc",
		UpdateRev:   "fresh-def",
		Source:      "event",
	}

	// Add stale rollout
	tracker.activeRollouts["default/stale"] = &rolloutState{
		StartTime:   now.Add(-15 * time.Minute),
		LastSeenAt:  now.Add(-12 * time.Minute), // Stale
		CurrentRev:  "stale-abc",
		UpdateRev:   "stale-def",
		Source:      "event",
	}

	assert.Equal(t, 2, len(tracker.activeRollouts))

	// Cleanup with 10 minute max age
	tracker.cleanup(10 * time.Minute)

	// Should remove only the stale rollout
	assert.Equal(t, 1, len(tracker.activeRollouts))
	assert.NotNil(t, tracker.activeRollouts["default/fresh"])
	assert.Nil(t, tracker.activeRollouts["default/stale"])
}

func TestRolloutEventTracker_GetActiveRolloutCount(t *testing.T) {
	tracker := newRolloutEventTracker()

	assert.Equal(t, 0, tracker.getActiveRolloutCount())

	tracker.activeRollouts["default/sts1"] = &rolloutState{}
	assert.Equal(t, 1, tracker.getActiveRolloutCount())

	tracker.activeRollouts["default/dep1"] = &rolloutState{}
	assert.Equal(t, 2, tracker.getActiveRolloutCount())

	delete(tracker.activeRollouts, "default/sts1")
	assert.Equal(t, 1, tracker.getActiveRolloutCount())
}