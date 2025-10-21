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
)

func TestStoreReplicaSet(t *testing.T) {
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

	tracker := NewRolloutTracker()
	tracker.StoreReplicaSet(rs, deploymentName, deploymentUID)

	// Test that the ReplicaSet was stored by accessing internal fields (for testing only)
	tracker.mutex.RLock()
	rsInfo, exists := tracker.replicaSetMap[namespace+"/"+rsName]
	tracker.mutex.RUnlock()

	require.True(t, exists, "ReplicaSet should be stored")
	assert.Equal(t, rsName, rsInfo.Name)
	assert.Equal(t, namespace, rsInfo.Namespace)
	assert.Equal(t, deploymentName, rsInfo.OwnerName)
	assert.Equal(t, deploymentUID, rsInfo.OwnerUID)
	assert.Equal(t, rs.CreationTimestamp.Time, rsInfo.CreationTime)
}

func TestStoreDeployment(t *testing.T) {
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

	tracker := NewRolloutTracker()
	tracker.StoreDeployment(deployment)

	tracker.mutex.RLock()
	stored, exists := tracker.deploymentMap["default/test-deployment"]
	tracker.mutex.RUnlock()

	require.True(t, exists, "Deployment should be stored")
	assert.Equal(t, deployment.Name, stored.Name)
	assert.Equal(t, deployment.Namespace, stored.Namespace)
	assert.Equal(t, deployment.UID, stored.UID)

	// Verify it's a deep copy (different memory address)
	assert.NotSame(t, deployment, stored, "Should be a deep copy")
}

func TestGetDeploymentRolloutDurationFromMaps(t *testing.T) {
	tracker := NewRolloutTracker()

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

	tracker.mutex.Lock()
	tracker.deploymentMap[deploymentKey] = deployment
	tracker.deploymentStartTime[deploymentKey] = rolloutStartTime
	tracker.mutex.Unlock()

	// Test getting duration - should return duration from deployment rollout start time
	duration := tracker.GetRolloutDuration(namespace, deploymentName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	// Allow for small timing differences in test
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on deployment rollout start time")
	assert.Greater(t, duration, 0.0, "Duration should be positive")
}

func TestGetDeploymentRolloutDurationFromMaps_NoDeployment(t *testing.T) {
	tracker := NewRolloutTracker()

	duration := tracker.GetRolloutDuration("default", "nonexistent-deployment")
	assert.Equal(t, 0.0, duration, "Should return 0 when no deployment found")
}

func TestGetDeploymentRolloutDurationFromMaps_DifferentDeployments(t *testing.T) {
	tracker := NewRolloutTracker()

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

	tracker.mutex.Lock()
	tracker.deploymentMap[key1] = deployment1
	tracker.deploymentStartTime[key1] = rolloutStartTime
	tracker.mutex.Unlock()

	// Query for deployment-2 should return 0
	duration := tracker.GetRolloutDuration(namespace, deploymentName2)
	assert.Equal(t, 0.0, duration, "Should return 0 for different deployment")

	// Query for deployment-1 should return duration
	duration = tracker.GetRolloutDuration(namespace, deploymentName1)
	assert.Greater(t, duration, 0.0, "Should return duration for correct deployment")
}

func TestCleanupCompletedDeployment(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
	}

	// Add deployment and ReplicaSets
	tracker.mutex.Lock()
	tracker.deploymentMap[namespace+"/"+deploymentName] = deployment

	tracker.replicaSetMap[namespace+"/rs1"] = &ReplicaSetInfo{
		Name:      "rs1",
		Namespace: namespace,
		OwnerName: deploymentName,
		OwnerUID:  "dep-123",
	}

	tracker.replicaSetMap[namespace+"/rs2"] = &ReplicaSetInfo{
		Name:      "rs2",
		Namespace: namespace,
		OwnerName: deploymentName,
		OwnerUID:  "dep-123",
	}

	// Add unrelated ReplicaSet that should not be cleaned up
	tracker.replicaSetMap[namespace+"/other-rs"] = &ReplicaSetInfo{
		Name:      "other-rs",
		Namespace: namespace,
		OwnerName: "other-deployment",
		OwnerUID:  "dep-456",
	}
	tracker.mutex.Unlock()

	// Verify initial state
	tracker.mutex.RLock()
	assert.Equal(t, 1, len(tracker.deploymentMap))
	assert.Equal(t, 3, len(tracker.replicaSetMap))
	tracker.mutex.RUnlock()

	// Cleanup
	tracker.CleanupDeployment(deployment.Namespace, deployment.Name)

	// Verify cleanup
	tracker.mutex.RLock()
	_, deploymentExists := tracker.deploymentMap[namespace+"/"+deploymentName]
	_, rs1Exists := tracker.replicaSetMap[namespace+"/rs1"]
	_, rs2Exists := tracker.replicaSetMap[namespace+"/rs2"]
	_, otherRsExists := tracker.replicaSetMap[namespace+"/other-rs"]
	tracker.mutex.RUnlock()

	assert.False(t, deploymentExists, "Deployment should be removed")
	assert.False(t, rs1Exists, "Associated ReplicaSet should be removed")
	assert.False(t, rs2Exists, "Associated ReplicaSet should be removed")
	assert.True(t, otherRsExists, "Unrelated ReplicaSet should remain")
}

func TestCleanupDeletedDeployment(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"

	// Add deployment and ReplicaSets
	tracker.mutex.Lock()
	tracker.deploymentMap[namespace+"/"+deploymentName] = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
	}

	tracker.replicaSetMap[namespace+"/rs1"] = &ReplicaSetInfo{
		Name:      "rs1",
		Namespace: namespace,
		OwnerName: deploymentName,
	}

	tracker.replicaSetMap[namespace+"/rs2"] = &ReplicaSetInfo{
		Name:      "rs2",
		Namespace: namespace,
		OwnerName: deploymentName,
	}

	// Add unrelated ReplicaSet
	tracker.replicaSetMap[namespace+"/other-rs"] = &ReplicaSetInfo{
		Name:      "other-rs",
		Namespace: namespace,
		OwnerName: "other-deployment",
	}
	tracker.mutex.Unlock()

	// Cleanup by name
	tracker.CleanupDeployment(namespace, deploymentName)

	// Verify cleanup
	tracker.mutex.RLock()
	_, deploymentExists := tracker.deploymentMap[namespace+"/"+deploymentName]
	_, rs1Exists := tracker.replicaSetMap[namespace+"/rs1"]
	_, rs2Exists := tracker.replicaSetMap[namespace+"/rs2"]
	_, otherRsExists := tracker.replicaSetMap[namespace+"/other-rs"]
	tracker.mutex.RUnlock()

	assert.False(t, deploymentExists, "Deployment should be removed")
	assert.False(t, rs1Exists, "Associated ReplicaSet should be removed")
	assert.False(t, rs2Exists, "Associated ReplicaSet should be removed")
	assert.True(t, otherRsExists, "Unrelated ReplicaSet should remain")
}

func TestCleanupDeletedDeployment_NonExistent(t *testing.T) {
	tracker := NewRolloutTracker()

	// Cleanup non-existent deployment should not panic
	tracker.CleanupDeployment("default", "non-existent")

	// Maps should remain empty
	tracker.mutex.RLock()
	assert.Equal(t, 0, len(tracker.deploymentMap))
	assert.Equal(t, 0, len(tracker.replicaSetMap))
	tracker.mutex.RUnlock()
}

func TestStoreDeployment_GenerationBasedRolloutDetection(t *testing.T) {
	tracker := NewRolloutTracker()

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

	tracker.StoreDeployment(deployment1)

	tracker.mutex.RLock()
	firstStartTime := tracker.deploymentStartTime[key]
	tracker.mutex.RUnlock()

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

	tracker.StoreDeployment(deployment2)

	tracker.mutex.RLock()
	secondStartTime := tracker.deploymentStartTime[key]
	tracker.mutex.RUnlock()

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

	tracker.StoreDeployment(deployment2Again)

	tracker.mutex.RLock()
	thirdStartTime := tracker.deploymentStartTime[key]
	tracker.mutex.RUnlock()

	// Same generation should NOT update start time
	assert.Equal(t, secondStartTime, thirdStartTime, "Same generation should not update start time")
}

func TestStoreControllerRevision(t *testing.T) {
	// Clear global maps for test isolation
	tracker := NewRolloutTracker()

	statefulSetName := "test-statefulset"
	statefulSetUID := "sts-123"
	namespace := "default"
	crName := "test-sts-revision-1"

	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              crName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
		},
		Revision: 1,
	}

	tracker.StoreControllerRevision(cr, statefulSetName, statefulSetUID)

	// Test that the ControllerRevision was stored by accessing internal fields (for testing only)
	tracker.mutex.RLock()
	crInfo, exists := tracker.controllerRevisionMap[namespace+"/"+crName]
	tracker.mutex.RUnlock()

	require.True(t, exists, "ControllerRevision should be stored")
	assert.Equal(t, crName, crInfo.Name)
	assert.Equal(t, namespace, crInfo.Namespace)
	assert.Equal(t, statefulSetName, crInfo.OwnerName)
	assert.Equal(t, statefulSetUID, crInfo.OwnerUID)
	assert.Equal(t, int64(1), crInfo.Revision)
	assert.Equal(t, cr.CreationTimestamp.Time, crInfo.CreationTime)
}

func TestStoreStatefulSet(t *testing.T) {
	tracker := NewRolloutTracker()

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "default",
			UID:       types.UID("sts-123"),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{3}[0],
		},
	}

	tracker.StoreStatefulSet(statefulSet)

	tracker.mutex.RLock()
	stored, exists := tracker.statefulSetMap["default/test-statefulset"]
	tracker.mutex.RUnlock()

	require.True(t, exists, "StatefulSet should be stored")
	assert.Equal(t, statefulSet.Name, stored.Name)
	assert.Equal(t, statefulSet.Namespace, stored.Namespace)
	assert.Equal(t, statefulSet.UID, stored.UID)

	// Verify it's a deep copy (different memory address)
	assert.NotSame(t, statefulSet, stored, "Should be a deep copy")
}

func TestGetStatefulSetRolloutDurationFromMaps(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	statefulSetName := "test-statefulset"
	statefulSetKey := namespace + "/" + statefulSetName

	// Add StatefulSet with rollout start time
	rolloutStartTime := time.Now().Add(-2 * time.Minute)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
		},
	}

	tracker.mutex.Lock()
	tracker.statefulSetMap[statefulSetKey] = statefulSet
	tracker.statefulSetStartTime[statefulSetKey] = rolloutStartTime
	tracker.mutex.Unlock()

	// Test getting duration - should return duration from StatefulSet rollout start time
	duration := tracker.GetStatefulSetRolloutDuration(namespace, statefulSetName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	// Allow for small timing differences in test
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on StatefulSet rollout start time")
	assert.Greater(t, duration, 0.0, "Duration should be positive")
}

func TestGetStatefulSetRolloutDurationFromMaps_NoStatefulSet(t *testing.T) {
	tracker := NewRolloutTracker()

	duration := tracker.GetStatefulSetRolloutDuration("default", "nonexistent-statefulset")
	assert.Equal(t, 0.0, duration, "Should return 0 when no StatefulSet found")
}

func TestGetStatefulSetRolloutDurationFromMaps_WithControllerRevision(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	statefulSetName := "test-statefulset"
	statefulSetKey := namespace + "/" + statefulSetName

	// Add StatefulSet with rollout start time
	rolloutStartTime := time.Now().Add(-5 * time.Minute)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
		},
	}

	// Add ControllerRevision that should be used instead of StatefulSet start time
	crStartTime := time.Now().Add(-3 * time.Minute)
	crInfo := &ControllerRevisionInfo{
		Name:         "test-cr-rev2",
		Namespace:    namespace,
		OwnerName:    statefulSetName,
		OwnerUID:     "sts-123",
		Revision:     2,
		CreationTime: crStartTime,
	}

	tracker.mutex.Lock()
	tracker.statefulSetMap[statefulSetKey] = statefulSet
	tracker.statefulSetStartTime[statefulSetKey] = rolloutStartTime
	tracker.controllerRevisionMap[namespace+"/test-cr-rev2"] = crInfo
	tracker.mutex.Unlock()

	// Test getting duration - should use ControllerRevision time, not StatefulSet start time
	duration := tracker.GetStatefulSetRolloutDuration(namespace, statefulSetName)

	expectedDuration := time.Since(crStartTime).Seconds()
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on newest ControllerRevision time")
}

func TestCleanupStatefulSet(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	statefulSetName := "test-statefulset"

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
		},
	}

	// Add StatefulSet and ControllerRevisions
	tracker.mutex.Lock()
	tracker.statefulSetMap[namespace+"/"+statefulSetName] = statefulSet

	tracker.controllerRevisionMap[namespace+"/cr1"] = &ControllerRevisionInfo{
		Name:      "cr1",
		Namespace: namespace,
		OwnerName: statefulSetName,
		OwnerUID:  "sts-123",
	}

	tracker.controllerRevisionMap[namespace+"/cr2"] = &ControllerRevisionInfo{
		Name:      "cr2",
		Namespace: namespace,
		OwnerName: statefulSetName,
		OwnerUID:  "sts-123",
	}

	// Add unrelated ControllerRevision that should not be cleaned up
	tracker.controllerRevisionMap[namespace+"/other-cr"] = &ControllerRevisionInfo{
		Name:      "other-cr",
		Namespace: namespace,
		OwnerName: "other-statefulset",
		OwnerUID:  "sts-456",
	}
	tracker.mutex.Unlock()

	// Verify initial state
	tracker.mutex.RLock()
	assert.Equal(t, 1, len(tracker.statefulSetMap))
	assert.Equal(t, 3, len(tracker.controllerRevisionMap))
	tracker.mutex.RUnlock()

	// Cleanup
	tracker.CleanupStatefulSet(statefulSet.Namespace, statefulSet.Name)

	// Verify cleanup
	tracker.mutex.RLock()
	_, statefulSetExists := tracker.statefulSetMap[namespace+"/"+statefulSetName]
	_, cr1Exists := tracker.controllerRevisionMap[namespace+"/cr1"]
	_, cr2Exists := tracker.controllerRevisionMap[namespace+"/cr2"]
	_, otherCrExists := tracker.controllerRevisionMap[namespace+"/other-cr"]
	tracker.mutex.RUnlock()

	assert.False(t, statefulSetExists, "StatefulSet should be removed")
	assert.False(t, cr1Exists, "Associated ControllerRevision should be removed")
	assert.False(t, cr2Exists, "Associated ControllerRevision should be removed")
	assert.True(t, otherCrExists, "Unrelated ControllerRevision should remain")
}

func TestCleanupControllerRevision(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	crName := "test-cr"

	// Add ControllerRevision
	tracker.mutex.Lock()
	tracker.controllerRevisionMap[namespace+"/"+crName] = &ControllerRevisionInfo{
		Name:      crName,
		Namespace: namespace,
	}
	tracker.mutex.Unlock()

	// Verify initial state
	tracker.mutex.RLock()
	assert.Equal(t, 1, len(tracker.controllerRevisionMap))
	tracker.mutex.RUnlock()

	// Cleanup
	tracker.CleanupControllerRevision(namespace, crName)

	// Verify cleanup
	tracker.mutex.RLock()
	_, exists := tracker.controllerRevisionMap[namespace+"/"+crName]
	tracker.mutex.RUnlock()

	assert.False(t, exists, "ControllerRevision should be removed")
}

func TestStoreStatefulSet_GenerationBasedRolloutDetection(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	statefulSetName := "test-statefulset"
	key := namespace + "/" + statefulSetName

	// Store first StatefulSet (generation 1)
	statefulSet1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 1,
		},
	}

	tracker.StoreStatefulSet(statefulSet1)

	tracker.mutex.RLock()
	firstStartTime := tracker.statefulSetStartTime[key]
	tracker.mutex.RUnlock()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Store same StatefulSet with higher generation (new rollout)
	statefulSet2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 2, // New generation
		},
	}

	tracker.StoreStatefulSet(statefulSet2)

	tracker.mutex.RLock()
	secondStartTime := tracker.statefulSetStartTime[key]
	tracker.mutex.RUnlock()

	// Second rollout should have a newer start time
	assert.True(t, secondStartTime.After(firstStartTime), "New rollout should have updated start time")

	// Store same StatefulSet again with same generation (no rollout change)
	statefulSet2Again := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 2, // Same generation
		},
	}

	tracker.StoreStatefulSet(statefulSet2Again)

	tracker.mutex.RLock()
	thirdStartTime := tracker.statefulSetStartTime[key]
	tracker.mutex.RUnlock()

	// Same generation should NOT update start time
	assert.Equal(t, secondStartTime, thirdStartTime, "Same generation should not update start time")
}

func TestHasStatefulSetRolloutCondition(t *testing.T) {
	tracker := NewRolloutTracker()

	tests := []struct {
		name           string
		statefulSet    *appsv1.StatefulSet
		expectedResult bool
		description    string
	}{
		{
			name: "revision_mismatch",
			statefulSet: &appsv1.StatefulSet{
				Status: appsv1.StatefulSetStatus{
					UpdateRevision:  "rev-2",
					CurrentRevision: "rev-1", // Different revisions
				},
			},
			expectedResult: true,
			description:    "Should detect rollout when update and current revisions differ",
		},
		{
			name: "replicas_not_ready",
			statefulSet: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{3}[0],
				},
				Status: appsv1.StatefulSetStatus{
					UpdateRevision:  "rev-1",
					CurrentRevision: "rev-1", // Same revisions
					UpdatedReplicas: 2,       // Not all replicas updated (not checked when revisions match)
					ReadyReplicas:   0,       // No replicas ready
				},
			},
			expectedResult: true,
			description:    "Should detect rollout when not all replicas are ready (revisions match but pods not ready)",
		},
		{
			name: "rollout_complete",
			statefulSet: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{3}[0],
				},
				Status: appsv1.StatefulSetStatus{
					UpdateRevision:  "rev-1",
					CurrentRevision: "rev-1", // Same revisions
					UpdatedReplicas: 3,       // All replicas updated
					ReadyReplicas:   3,       // All replicas ready
				},
			},
			expectedResult: false,
			description:    "Should not detect rollout when all conditions are satisfied",
		},
		{
			name: "nil_replicas",
			statefulSet: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: nil,
				},
				Status: appsv1.StatefulSetStatus{
					UpdateRevision:  "rev-1",
					CurrentRevision: "rev-1",
					UpdatedReplicas: 0,
					ReadyReplicas:   1, // Default 1 replica ready
				},
			},
			expectedResult: false,
			description:    "Should handle nil replicas gracefully",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := tracker.HasStatefulSetRolloutCondition(test.statefulSet)
			assert.Equal(t, test.expectedResult, result, test.description)
		})
	}
}

func TestStoreDaemonSetControllerRevision(t *testing.T) {
	tracker := NewRolloutTracker()

	daemonSetName := "test-daemonset"
	daemonSetUID := "ds-123"
	namespace := "default"
	crName := "test-ds-revision-1"

	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              crName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
		},
		Revision: 1,
	}

	tracker.StoreDaemonSetControllerRevision(cr, daemonSetName, daemonSetUID)

	// Test that the ControllerRevision was stored by accessing internal fields (for testing only)
	tracker.mutex.RLock()
	crInfo, exists := tracker.daemonSetControllerRevisionMap[namespace+"/"+crName]
	tracker.mutex.RUnlock()

	require.True(t, exists, "ControllerRevision should be stored")
	assert.Equal(t, crName, crInfo.Name)
	assert.Equal(t, namespace, crInfo.Namespace)
	assert.Equal(t, daemonSetName, crInfo.OwnerName)
	assert.Equal(t, daemonSetUID, crInfo.OwnerUID)
	assert.Equal(t, int64(1), crInfo.Revision)
	assert.Equal(t, cr.CreationTimestamp.Time, crInfo.CreationTime)
}

func TestStoreDaemonSet(t *testing.T) {
	tracker := NewRolloutTracker()

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "default",
			UID:       types.UID("ds-123"),
		},
		Spec: appsv1.DaemonSetSpec{},
	}

	tracker.StoreDaemonSet(daemonSet)

	tracker.mutex.RLock()
	stored, exists := tracker.daemonSetMap["default/test-daemonset"]
	tracker.mutex.RUnlock()

	require.True(t, exists, "DaemonSet should be stored")
	assert.Equal(t, daemonSet.Name, stored.Name)
	assert.Equal(t, daemonSet.Namespace, stored.Namespace)
	assert.Equal(t, daemonSet.UID, stored.UID)

	// Verify it's a deep copy (different memory address)
	assert.NotSame(t, daemonSet, stored, "Should be a deep copy")
}

func TestGetDaemonSetRolloutDurationFromMaps(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	daemonSetName := "test-daemonset"
	daemonSetKey := namespace + "/" + daemonSetName

	// Add StatefulSet with rollout start time
	rolloutStartTime := time.Now().Add(-2 * time.Minute)
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonSetName,
			Namespace: namespace,
		},
	}

	tracker.mutex.Lock()
	tracker.daemonSetMap[daemonSetKey] = daemonSet
	tracker.daemonSetStartTime[daemonSetKey] = rolloutStartTime
	tracker.mutex.Unlock()

	// Test getting duration - should return duration from StatefulSet rollout start time
	duration := tracker.GetDaemonSetRolloutDuration(namespace, daemonSetName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	// Allow for small timing differences in test
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on StatefulSet rollout start time")
	assert.Greater(t, duration, 0.0, "Duration should be positive")
}

func TestGetDaemonSetRolloutDurationFromMaps_NoDaemonSet(t *testing.T) {
	tracker := NewRolloutTracker()

	duration := tracker.GetDaemonSetRolloutDuration("default", "nonexistent-daemonset")
	assert.Equal(t, 0.0, duration, "Should return 0 when no DaemonSet found")
}

func TestGetDaemonSetRolloutDurationFromMaps_WithControllerRevision(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	daemonSetName := "test-daemonset"
	daemonSetKey := namespace + "/" + daemonSetName

	// Add DaemonSet with rollout start time
	rolloutStartTime := time.Now().Add(-5 * time.Minute)
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonSetName,
			Namespace: namespace,
		},
	}

	// Add ControllerRevision that should be used instead of DaemonSet start time
	crStartTime := time.Now().Add(-3 * time.Minute)
	crInfo := &ControllerRevisionInfo{
		Name:         "test-cr-rev2",
		Namespace:    namespace,
		OwnerName:    daemonSetName,
		OwnerUID:     "ds-123",
		Revision:     2,
		CreationTime: crStartTime,
	}

	tracker.mutex.Lock()
	tracker.daemonSetMap[daemonSetKey] = daemonSet
	tracker.daemonSetStartTime[daemonSetKey] = rolloutStartTime
	tracker.daemonSetControllerRevisionMap[namespace+"/test-cr-rev2"] = crInfo
	tracker.mutex.Unlock()

	// Test getting duration - should use ControllerRevision time, not DaemonSet start time
	duration := tracker.GetDaemonSetRolloutDuration(namespace, daemonSetName)

	expectedDuration := time.Since(crStartTime).Seconds()
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on newest ControllerRevision time")
}

func TestCleanupDaemonSet(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	daemonSetName := "test-daemonset"

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonSetName,
			Namespace: namespace,
		},
	}

	// Add StatefulSet and ControllerRevisions
	tracker.mutex.Lock()
	tracker.daemonSetMap[namespace+"/"+daemonSetName] = daemonSet

	tracker.daemonSetControllerRevisionMap[namespace+"/cr1"] = &ControllerRevisionInfo{
		Name:      "cr1",
		Namespace: namespace,
		OwnerName: daemonSetName,
		OwnerUID:  "ds-123",
	}

	tracker.daemonSetControllerRevisionMap[namespace+"/cr2"] = &ControllerRevisionInfo{
		Name:      "cr2",
		Namespace: namespace,
		OwnerName: daemonSetName,
		OwnerUID:  "ds-123",
	}

	// Add unrelated ControllerRevision that should not be cleaned up
	tracker.daemonSetControllerRevisionMap[namespace+"/other-cr"] = &ControllerRevisionInfo{
		Name:      "other-cr",
		Namespace: namespace,
		OwnerName: "other-daemonset",
		OwnerUID:  "ds-456",
	}
	tracker.mutex.Unlock()

	// Verify initial state
	tracker.mutex.RLock()
	assert.Equal(t, 1, len(tracker.daemonSetMap))
	assert.Equal(t, 3, len(tracker.daemonSetControllerRevisionMap))
	tracker.mutex.RUnlock()

	// Cleanup
	tracker.CleanupDaemonSet(daemonSet.Namespace, daemonSet.Name)

	// Verify cleanup
	tracker.mutex.RLock()
	_, daemonSetExists := tracker.daemonSetMap[namespace+"/"+daemonSetName]
	_, cr1Exists := tracker.controllerRevisionMap[namespace+"/cr1"]
	_, cr2Exists := tracker.daemonSetControllerRevisionMap[namespace+"/cr2"]
	_, otherCrExists := tracker.daemonSetControllerRevisionMap[namespace+"/other-cr"]
	tracker.mutex.RUnlock()

	assert.False(t, daemonSetExists, "DaemonSet should be removed")
	assert.False(t, cr1Exists, "Associated ControllerRevision should be removed")
	assert.False(t, cr2Exists, "Associated ControllerRevision should be removed")
	assert.True(t, otherCrExists, "Unrelated ControllerRevision should remain")
}

func TestCleanupDaemonSetControllerRevision(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	crName := "test-cr"

	// Add ControllerRevision
	tracker.mutex.Lock()
	tracker.daemonSetControllerRevisionMap[namespace+"/"+crName] = &ControllerRevisionInfo{
		Name:      crName,
		Namespace: namespace,
	}
	tracker.mutex.Unlock()

	// Verify initial state
	tracker.mutex.RLock()
	assert.Equal(t, 1, len(tracker.daemonSetControllerRevisionMap))
	tracker.mutex.RUnlock()

	// Cleanup
	tracker.CleanupDaemonSetControllerRevision(namespace, crName)

	// Verify cleanup
	tracker.mutex.RLock()
	_, exists := tracker.daemonSetControllerRevisionMap[namespace+"/"+crName]
	tracker.mutex.RUnlock()

	assert.False(t, exists, "ControllerRevision should be removed")
}

func TestStoreDaemonSet_GenerationBasedRolloutDetection(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	daemonSetName := "test-daemonset"
	key := namespace + "/" + daemonSetName

	// Store first DaemonSet (generation 1)
	daemonSet1 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       daemonSetName,
			Namespace:  namespace,
			Generation: 1,
		},
	}

	tracker.StoreDaemonSet(daemonSet1)

	tracker.mutex.RLock()
	firstStartTime := tracker.daemonSetStartTime[key]
	tracker.mutex.RUnlock()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Store same DaemonSet with higher generation (new rollout)
	daemonSet2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       daemonSetName,
			Namespace:  namespace,
			Generation: 2, // New generation
		},
	}

	tracker.StoreDaemonSet(daemonSet2)

	tracker.mutex.RLock()
	secondStartTime := tracker.daemonSetStartTime[key]
	tracker.mutex.RUnlock()

	// Second rollout should have a newer start time
	assert.True(t, secondStartTime.After(firstStartTime), "New rollout should have updated start time")

	// Store same DaemonSet again with same generation (no rollout change)
	daemonSet2Again := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       daemonSetName,
			Namespace:  namespace,
			Generation: 2, // Same generation
		},
	}

	tracker.StoreDaemonSet(daemonSet2Again)

	tracker.mutex.RLock()
	thirdStartTime := tracker.daemonSetStartTime[key]
	tracker.mutex.RUnlock()

	// Same generation should NOT update start time
	assert.Equal(t, secondStartTime, thirdStartTime, "Same generation should not update start time")
}

func TestHasDaemonSetRolloutCondition(t *testing.T) {
	tracker := NewRolloutTracker()

	tests := []struct {
		name           string
		daemonSet      *appsv1.DaemonSet
		expectedResult bool
		description    string
	}{
		{
			name: "no_desired_pods",
			daemonSet: &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 0,
				},
			},
			expectedResult: false,
			description:    "Should not detect rollout when no desired pods",
		},
		{
			name: "updated_number_scheduled_less_than_desired",
			daemonSet: &appsv1.DaemonSet{
				Spec: appsv1.DaemonSetSpec{},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					UpdatedNumberScheduled: 2, // Less than desired
				},
			},
			expectedResult: true,
			description:    "Should detect rollout when updated number scheduled is less than desired",
		},
		{
			name: "rollout_complete",
			daemonSet: &appsv1.DaemonSet{
				Spec: appsv1.DaemonSetSpec{},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberAvailable:        3,
					UpdatedNumberScheduled: 3,
				},
			},
			expectedResult: false,
			description:    "Should not detect rollout when all conditions are satisfied",
		},
		{
			name: "number_available_less_than_desired",
			daemonSet: &appsv1.DaemonSet{
				Spec: appsv1.DaemonSetSpec{},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberAvailable:        2,
				},
			},
			expectedResult: true,
			description:    "Should detect rollout when number available is less than desired",
		},
		{
			name: "number_unavailable_greater_than_zero",
			daemonSet: &appsv1.DaemonSet{
				Spec: appsv1.DaemonSetSpec{},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberUnavailable:      1,
				},
			},
			expectedResult: true,
			description:    "Should detect rollout when number unavailable is greater than zero",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := tracker.HasDaemonSetRolloutCondition(test.daemonSet)
			assert.Equal(t, test.expectedResult, result, test.description)
		})
	}
}

// TestNodeMigrationScenario tests that pod restarts from node migration don't trigger false positive rollout detection
func TestNodeMigrationScenario(t *testing.T) {
	tracker := NewRolloutTracker()

	// Simulate a StatefulSet that was deployed days ago
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-sts",
			Namespace:  "default",
			Generation: 5, // Stable generation from days ago
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{3}[0],
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 5,         // Matches generation - no new rollout
			UpdateRevision:     "rev-123", // Same revision
			CurrentRevision:    "rev-123", // Same revision
			UpdatedReplicas:    3,         // All updated
			ReadyReplicas:      3,         // All ready (simulating completed rollout)
		},
	}

	key := sts.Namespace + "/" + sts.Name
	tracker.statefulSetMap[key] = sts.DeepCopy()
	tracker.statefulSetStartTime[key] = time.Now().Add(-2 * time.Hour)

	// Test: Should NOT detect as ongoing rollout (all conditions stable)
	// This is a completed/stable scenario: all pods ready, revisions match, no config changes
	ongoing := tracker.HasActiveStatefulSetRollout(sts) &&
		(sts.Status.ReadyReplicas < *sts.Spec.Replicas || tracker.HasStatefulSetRolloutCondition(sts))

	assert.False(t, ongoing, "Completed/stable StatefulSet should not be detected as ongoing rollout")

	// Verify the individual components
	assert.True(t, tracker.HasActiveStatefulSetRollout(sts), "Should have active tracking")
	assert.False(t, sts.Status.ReadyReplicas < *sts.Spec.Replicas, "Should not have replica mismatch (rollout completed)")
	rolloutCondition := tracker.HasStatefulSetRolloutCondition(sts)
	t.Logf("HasStatefulSetRolloutCondition returned: %t (should be false)", rolloutCondition)
	assert.False(t, rolloutCondition, "Should not have rollout condition (revisions match)")

	// Test proposed fix: Only consider revision mismatches for tracked rollouts
	// Replica readiness issues without revision mismatches = temporary pod issues, not rollouts

	isNewRollout := sts.Generation != sts.Status.ObservedGeneration
	hasActive := tracker.HasActiveStatefulSetRollout(sts)
	hasReplicaMismatch := sts.Status.ReadyReplicas < *sts.Spec.Replicas
	hasRevisionMismatch := sts.Status.UpdateRevision != sts.Status.CurrentRevision

	// Proposed logic: Only track rollouts based on revision mismatch
	isTrackedRollout := hasActive && hasRevisionMismatch
	isOngoingWithProposedLogic := isNewRollout || isTrackedRollout

	assert.False(t, isNewRollout, "Should not be new rollout (generation matches)")
	assert.True(t, hasActive, "Should have active tracking")
	assert.False(t, hasReplicaMismatch, "Should not have replica mismatch (rollout completed)")
	assert.False(t, hasRevisionMismatch, "Should not have revision mismatch")
	assert.False(t, isTrackedRollout, "Should not be tracked rollout (no revision mismatch)")
	assert.False(t, isOngoingWithProposedLogic, "With proposed logic, this should not be ongoing - problem solved!")

	// This test demonstrates the solution:
	// 1. Legitimate ongoing rollouts have revision mismatches
	// 2. Completed rollouts have matching revisions and all replicas ready
	// 3. Only consider revision mismatches for ongoing rollout detection
}

// TestStatefulSetNodeMigrationScenario tests that StatefulSet pod restarts during node migration ARE considered ongoing rollouts (because state matters)
func TestStatefulSetNodeMigrationScenario(t *testing.T) {
	tracker := NewRolloutTracker()

	// Simulate a StatefulSet experiencing pod restarts during node migration
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-statefulset",
			Namespace:  "default",
			Generation: 5, // Stable generation - no new rollout
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{3}[0],
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 5,         // Matches generation - no new rollout
			UpdateRevision:     "rev-123", // Same revision
			CurrentRevision:    "rev-123", // Same revision - no ongoing rollout in terms of updates
			UpdatedReplicas:    3,         // All updated (but this isn't checked when revisions match)
			ReadyReplicas:      1,         // Only 1 ready due to node migration pod restarts
		},
	}

	// Store the StatefulSet as if it was tracked from a real rollout
	key := sts.Namespace + "/" + sts.Name
	tracker.statefulSetMap[key] = sts.DeepCopy()
	tracker.statefulSetStartTime[key] = time.Now().Add(-2 * time.Hour)

	// Test: Should detect as ongoing rollout for StatefulSets during node migration
	// Unlike Deployments, StatefulSet state matters even during infrastructure changes
	ongoing := tracker.HasActiveStatefulSetRollout(sts) &&
		(sts.Status.ReadyReplicas < *sts.Spec.Replicas || tracker.HasStatefulSetRolloutCondition(sts))

	assert.True(t, ongoing, "StatefulSet with pods not ready should be detected as ongoing rollout (state matters)")

	// Verify the individual components
	assert.True(t, tracker.HasActiveStatefulSetRollout(sts), "Should have active tracking")
	assert.True(t, sts.Status.ReadyReplicas < *sts.Spec.Replicas, "Should have replica readiness mismatch")
	rolloutCondition := tracker.HasStatefulSetRolloutCondition(sts)
	t.Logf("HasStatefulSetRolloutCondition returned: %t (should be true due to ReadyReplicas < Replicas)", rolloutCondition)
	assert.True(t, rolloutCondition, "Should have rollout condition (not all replicas ready)")
}

// TestDeploymentNodeMigrationScenario tests that deployment pod restarts from node migration don't trigger false rollout detection
func TestDeploymentNodeMigrationScenario(t *testing.T) {
	tracker := NewRolloutTracker()

	// Simulate a Deployment that was deployed days ago
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-deployment",
			Namespace:  "default",
			Generation: 5, // Stable generation from days ago
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &[]int32{3}[0],
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 5, // Matches generation - no new rollout
			ReadyReplicas:      1, // Only 1 ready due to node migration restart
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: "False", // No rollout in progress
					Reason: "NewReplicaSetAvailable",
				},
			},
		},
	}

	// Store the Deployment as if it was tracked from a real rollout days ago
	key := deployment.Namespace + "/" + deployment.Name
	tracker.deploymentMap[key] = deployment.DeepCopy()
	// Simulate old rollout start time (2 hours ago - for duration calculation)
	tracker.deploymentStartTime[key] = time.Now().Add(-2 * time.Hour)

	// Test proposed fix: Only consider rollout conditions for tracked rollouts
	isNewRollout := deployment.Generation != deployment.Status.ObservedGeneration
	hasActive := tracker.HasActiveRollout(deployment)
	hasReplicaMismatch := deployment.Status.ReadyReplicas < *deployment.Spec.Replicas
	hasRolloutCondition := tracker.HasRolloutCondition(deployment)

	// Proposed logic: Only track rollouts based on rollout condition
	isTrackedRollout := hasActive && hasRolloutCondition
	isOngoingWithProposedLogic := isNewRollout || isTrackedRollout

	assert.False(t, isNewRollout, "Should not be new rollout (generation matches)")
	assert.True(t, hasActive, "Should have active tracking")
	assert.True(t, hasReplicaMismatch, "Should have replica mismatch")
	assert.False(t, hasRolloutCondition, "Should not have rollout condition (deployment not progressing)")
	assert.False(t, isTrackedRollout, "Should not be tracked rollout (no rollout condition)")
	assert.False(t, isOngoingWithProposedLogic, "With proposed logic, this should not be ongoing - problem solved!")

	// Verify the rollout condition logic
	t.Logf("Deployment has rollout condition: %t", hasRolloutCondition)
}
