// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
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
)

func TestStoreReplicaSet(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	deploymentName := "test-deployment"
	deploymentUID := "dep-123"
	namespace := "default"
	rsName := "test-rs-abc123"

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              rsName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
		},
	}

	StoreReplicaSet(rs, deploymentName, deploymentUID)

	rolloutMutex.RLock()
	rsInfo, exists := replicaSetMap[namespace+"/"+rsName]
	rolloutMutex.RUnlock()

	require.True(t, exists, "ReplicaSet should be stored")
	assert.Equal(t, rsName, rsInfo.Name)
	assert.Equal(t, namespace, rsInfo.Namespace)
	assert.Equal(t, deploymentName, rsInfo.OwnerName)
	assert.Equal(t, deploymentUID, rsInfo.OwnerUID)
	assert.Equal(t, rs.CreationTimestamp.Time, rsInfo.CreationTime)
}

func TestStoreDeployment(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
			UID:       types.UID("dep-123"),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &[]int32{3}[0],
		},
	}

	StoreDeployment(deployment)

	rolloutMutex.RLock()
	stored, exists := deploymentMap["default/test-deployment"]
	rolloutMutex.RUnlock()

	require.True(t, exists, "Deployment should be stored")
	assert.Equal(t, deployment.Name, stored.Name)
	assert.Equal(t, deployment.Namespace, stored.Namespace)
	assert.Equal(t, deployment.UID, stored.UID)

	// Verify it's a deep copy (different memory address)
	assert.NotSame(t, deployment, stored, "Should be a deep copy")
}

func TestGetDeploymentRolloutDurationFromMaps(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	namespace := "default"
	deploymentName := "test-deployment"
	deploymentKey := namespace + "/" + deploymentName

	// Add deployment with rollout start time
	rolloutStartTime := time.Now().Add(-2 * time.Minute)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
	}

	rolloutMutex.Lock()
	deploymentMap[deploymentKey] = deployment
	deploymentStartTime[deploymentKey] = rolloutStartTime
	rolloutMutex.Unlock()

	// Test getting duration - should return duration from deployment rollout start time
	duration := GetDeploymentRolloutDurationFromMaps(namespace, deploymentName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	// Allow for small timing differences in test
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on deployment rollout start time")
	assert.Greater(t, duration, 0.0, "Duration should be positive")
}

func TestGetDeploymentRolloutDurationFromMaps_NoDeployment(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	duration := GetDeploymentRolloutDurationFromMaps("default", "nonexistent-deployment")
	assert.Equal(t, 0.0, duration, "Should return 0 when no deployment found")
}

func TestGetDeploymentRolloutDurationFromMaps_DifferentDeployments(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	namespace := "default"
	deploymentName1 := "deployment-1"
	deploymentName2 := "deployment-2"
	key1 := namespace + "/" + deploymentName1

	// Add deployment-1 with rollout start time
	rolloutStartTime := time.Now().Add(-5 * time.Minute)
	deployment1 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName1,
			Namespace: namespace,
		},
	}

	rolloutMutex.Lock()
	deploymentMap[key1] = deployment1
	deploymentStartTime[key1] = rolloutStartTime
	rolloutMutex.Unlock()

	// Query for deployment-2 should return 0
	duration := GetDeploymentRolloutDurationFromMaps(namespace, deploymentName2)
	assert.Equal(t, 0.0, duration, "Should return 0 for different deployment")

	// Query for deployment-1 should return duration
	duration = GetDeploymentRolloutDurationFromMaps(namespace, deploymentName1)
	assert.Greater(t, duration, 0.0, "Should return duration for correct deployment")
}

func TestCleanupCompletedDeployment(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	namespace := "default"
	deploymentName := "test-deployment"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
	}

	// Add deployment and ReplicaSets
	rolloutMutex.Lock()
	deploymentMap[namespace+"/"+deploymentName] = deployment

	replicaSetMap[namespace+"/rs1"] = &ReplicaSetInfo{
		Name:      "rs1",
		Namespace: namespace,
		OwnerName: deploymentName,
		OwnerUID:  "dep-123",
	}

	replicaSetMap[namespace+"/rs2"] = &ReplicaSetInfo{
		Name:      "rs2",
		Namespace: namespace,
		OwnerName: deploymentName,
		OwnerUID:  "dep-123",
	}

	// Add unrelated ReplicaSet that should not be cleaned up
	replicaSetMap[namespace+"/other-rs"] = &ReplicaSetInfo{
		Name:      "other-rs",
		Namespace: namespace,
		OwnerName: "other-deployment",
		OwnerUID:  "dep-456",
	}
	rolloutMutex.Unlock()

	// Verify initial state
	rolloutMutex.RLock()
	assert.Equal(t, 1, len(deploymentMap))
	assert.Equal(t, 3, len(replicaSetMap))
	rolloutMutex.RUnlock()

	// Cleanup
	CleanupCompletedDeployment(deployment)

	// Verify cleanup
	rolloutMutex.RLock()
	_, deploymentExists := deploymentMap[namespace+"/"+deploymentName]
	_, rs1Exists := replicaSetMap[namespace+"/rs1"]
	_, rs2Exists := replicaSetMap[namespace+"/rs2"]
	_, otherRsExists := replicaSetMap[namespace+"/other-rs"]
	rolloutMutex.RUnlock()

	assert.False(t, deploymentExists, "Deployment should be removed")
	assert.False(t, rs1Exists, "Associated ReplicaSet should be removed")
	assert.False(t, rs2Exists, "Associated ReplicaSet should be removed")
	assert.True(t, otherRsExists, "Unrelated ReplicaSet should remain")
}

func TestCleanupDeletedDeployment(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	namespace := "default"
	deploymentName := "test-deployment"

	// Add deployment and ReplicaSets
	rolloutMutex.Lock()
	deploymentMap[namespace+"/"+deploymentName] = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
	}

	replicaSetMap[namespace+"/rs1"] = &ReplicaSetInfo{
		Name:      "rs1",
		Namespace: namespace,
		OwnerName: deploymentName,
	}

	replicaSetMap[namespace+"/rs2"] = &ReplicaSetInfo{
		Name:      "rs2",
		Namespace: namespace,
		OwnerName: deploymentName,
	}

	// Add unrelated ReplicaSet
	replicaSetMap[namespace+"/other-rs"] = &ReplicaSetInfo{
		Name:      "other-rs",
		Namespace: namespace,
		OwnerName: "other-deployment",
	}
	rolloutMutex.Unlock()

	// Cleanup by name
	CleanupDeletedDeployment(namespace, deploymentName)

	// Verify cleanup
	rolloutMutex.RLock()
	_, deploymentExists := deploymentMap[namespace+"/"+deploymentName]
	_, rs1Exists := replicaSetMap[namespace+"/rs1"]
	_, rs2Exists := replicaSetMap[namespace+"/rs2"]
	_, otherRsExists := replicaSetMap[namespace+"/other-rs"]
	rolloutMutex.RUnlock()

	assert.False(t, deploymentExists, "Deployment should be removed")
	assert.False(t, rs1Exists, "Associated ReplicaSet should be removed")
	assert.False(t, rs2Exists, "Associated ReplicaSet should be removed")
	assert.True(t, otherRsExists, "Unrelated ReplicaSet should remain")
}

func TestCleanupDeletedDeployment_NonExistent(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	// Cleanup non-existent deployment should not panic
	CleanupDeletedDeployment("default", "non-existent")

	// Maps should remain empty
	rolloutMutex.RLock()
	assert.Equal(t, 0, len(deploymentMap))
	assert.Equal(t, 0, len(replicaSetMap))
	rolloutMutex.RUnlock()
}

func TestPeriodicCleanup(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	now := time.Now()

	// Add some test data
	oldDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "old-deployment", Namespace: "default"},
	}
	recentDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "recent-deployment", Namespace: "default"},
	}

	rolloutMutex.Lock()
	// Old deployment (should be cleaned up) - unused for more than 120 seconds
	deploymentMap["default/old-deployment"] = oldDeployment
	deploymentAccessTime["default/old-deployment"] = now.Add(-200 * time.Second) // Older than 120s maxDeploymentUnusedTime
	deploymentStartTime["default/old-deployment"] = now.Add(-200 * time.Second)

	// Recent deployment (should be kept) - used within 120 seconds
	deploymentMap["default/recent-deployment"] = recentDeployment
	deploymentAccessTime["default/recent-deployment"] = now.Add(-60 * time.Second) // Recent (within 120s)
	deploymentStartTime["default/recent-deployment"] = now.Add(-60 * time.Second)

	// Old ReplicaSet (should be cleaned up) - older than 120 seconds
	replicaSetMap["default/old-rs"] = &ReplicaSetInfo{
		Name:         "old-rs",
		Namespace:    "default",
		CreationTime: now.Add(-200 * time.Second), // Older than 120s maxReplicaSetAge
		OwnerName:    "old-deployment",
	}

	// Recent ReplicaSet (should be kept) - newer than 120 seconds
	replicaSetMap["default/recent-rs"] = &ReplicaSetInfo{
		Name:         "recent-rs",
		Namespace:    "default",
		CreationTime: now.Add(-60 * time.Second), // Recent (within 120s)
		OwnerName:    "recent-deployment",
	}
	rolloutMutex.Unlock()

	// Force cleanup by resetting the last cleanup time
	cleanupMutex.Lock()
	lastCleanupTime = time.Time{} // Force cleanup to run
	cleanupMutex.Unlock()

	// Run cleanup
	PeriodicCleanup()

	// Verify results
	rolloutMutex.RLock()
	_, oldDepExists := deploymentMap["default/old-deployment"]
	_, recentDepExists := deploymentMap["default/recent-deployment"]
	_, oldRsExists := replicaSetMap["default/old-rs"]
	_, recentRsExists := replicaSetMap["default/recent-rs"]
	rolloutMutex.RUnlock()

	assert.False(t, oldDepExists, "Old deployment should be cleaned up")
	assert.True(t, recentDepExists, "Recent deployment should be kept")
	assert.False(t, oldRsExists, "Old ReplicaSet should be cleaned up")
	assert.True(t, recentRsExists, "Recent ReplicaSet should be kept")
}

func TestStoreDeployment_GenerationBasedRolloutDetection(t *testing.T) {
	// Clear global maps for test isolation
	rolloutMutex.Lock()
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
	rolloutMutex.Unlock()

	namespace := "default"
	deploymentName := "test-deployment"
	key := namespace + "/" + deploymentName

	// Store first deployment (generation 1)
	deployment1 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 1,
		},
	}

	StoreDeployment(deployment1)

	rolloutMutex.RLock()
	firstStartTime := deploymentStartTime[key]
	rolloutMutex.RUnlock()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Store same deployment with higher generation (new rollout)
	deployment2 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 2, // New generation
		},
	}

	StoreDeployment(deployment2)

	rolloutMutex.RLock()
	secondStartTime := deploymentStartTime[key]
	rolloutMutex.RUnlock()

	// Second rollout should have a newer start time
	assert.True(t, secondStartTime.After(firstStartTime), "New rollout should have updated start time")

	// Store same deployment again with same generation (no rollout change)
	deployment2Again := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 2, // Same generation
		},
	}

	StoreDeployment(deployment2Again)

	rolloutMutex.RLock()
	thirdStartTime := deploymentStartTime[key]
	rolloutMutex.RUnlock()

	// Same generation should NOT update start time
	assert.Equal(t, secondStartTime, thirdStartTime, "Same generation should not update start time")
}
