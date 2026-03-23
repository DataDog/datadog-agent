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
	corev1 "k8s.io/api/core/v1"
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
	tracker.deploymentMutex.RLock()
	rsInfo, exists := tracker.replicaSetMap[namespace+"/"+rsName]
	tracker.deploymentMutex.RUnlock()

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

	tracker.deploymentMutex.RLock()
	stored, exists := tracker.deploymentMap["default/test-deployment"]
	tracker.deploymentMutex.RUnlock()

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

	tracker.deploymentMutex.Lock()
	tracker.deploymentMap[deploymentKey] = deployment
	tracker.deploymentStartTime[deploymentKey] = rolloutStartTime
	tracker.deploymentMutex.Unlock()

	// Test getting duration - should return duration from deployment rollout start time
	duration := tracker.GetRolloutDuration(namespace, deploymentName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	// Allow for small timing differences in test
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on deployment rollout start time")
	assert.GreaterOrEqual(t, duration, 0.0, "Duration should be non-negative")
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

	tracker.deploymentMutex.Lock()
	tracker.deploymentMap[key1] = deployment1
	tracker.deploymentStartTime[key1] = rolloutStartTime
	tracker.deploymentMutex.Unlock()

	// Query for deployment-2 should return 0
	duration := tracker.GetRolloutDuration(namespace, deploymentName2)
	assert.Equal(t, 0.0, duration, "Should return 0 for different deployment")

	// Query for deployment-1 should return duration
	duration = tracker.GetRolloutDuration(namespace, deploymentName1)
	assert.GreaterOrEqual(t, duration, 0.0, "Should return duration for correct deployment")
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
	tracker.deploymentMutex.Lock()
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
	tracker.deploymentMutex.Unlock()

	// Verify initial state
	tracker.deploymentMutex.RLock()
	assert.Equal(t, 1, len(tracker.deploymentMap))
	assert.Equal(t, 3, len(tracker.replicaSetMap))
	tracker.deploymentMutex.RUnlock()

	// Cleanup
	tracker.CleanupDeployment(deployment.Namespace, deployment.Name)

	// Verify cleanup
	tracker.deploymentMutex.RLock()
	_, deploymentExists := tracker.deploymentMap[namespace+"/"+deploymentName]
	_, rs1Exists := tracker.replicaSetMap[namespace+"/rs1"]
	_, rs2Exists := tracker.replicaSetMap[namespace+"/rs2"]
	_, otherRsExists := tracker.replicaSetMap[namespace+"/other-rs"]
	tracker.deploymentMutex.RUnlock()

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
	tracker.deploymentMutex.Lock()
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
	tracker.deploymentMutex.Unlock()

	// Cleanup by name
	tracker.CleanupDeployment(namespace, deploymentName)

	// Verify cleanup
	tracker.deploymentMutex.RLock()
	_, deploymentExists := tracker.deploymentMap[namespace+"/"+deploymentName]
	_, rs1Exists := tracker.replicaSetMap[namespace+"/rs1"]
	_, rs2Exists := tracker.replicaSetMap[namespace+"/rs2"]
	_, otherRsExists := tracker.replicaSetMap[namespace+"/other-rs"]
	tracker.deploymentMutex.RUnlock()

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
	tracker.deploymentMutex.RLock()
	assert.Equal(t, 0, len(tracker.deploymentMap))
	assert.Equal(t, 0, len(tracker.replicaSetMap))
	tracker.deploymentMutex.RUnlock()
}

func TestStoreDeployment_RevisionBasedRolloutDetection(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"
	key := namespace + "/" + deploymentName

	// Store first deployment (revision "1")
	deployment1 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 1,
			Annotations: map[string]string{
				RevisionAnnotationKey: "1",
			},
		},
	}

	tracker.StoreDeployment(deployment1)

	tracker.deploymentMutex.RLock()
	firstStartTime := tracker.deploymentStartTime[key]
	tracker.deploymentMutex.RUnlock()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Store same deployment with new revision (new rollout)
	deployment2 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 2,
			Annotations: map[string]string{
				RevisionAnnotationKey: "2", // New revision
			},
		},
	}

	tracker.StoreDeployment(deployment2)

	tracker.deploymentMutex.RLock()
	secondStartTime := tracker.deploymentStartTime[key]
	tracker.deploymentMutex.RUnlock()

	// Second rollout should have a newer start time (revision changed)
	assert.True(t, secondStartTime.After(firstStartTime), "New rollout should have updated start time")

	// Store same deployment again with same revision (no rollout change)
	deployment2Again := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 3, // Generation changes (e.g., scaling)
			Annotations: map[string]string{
				RevisionAnnotationKey: "2", // Same revision
			},
		},
	}

	tracker.StoreDeployment(deployment2Again)

	tracker.deploymentMutex.RLock()
	thirdStartTime := tracker.deploymentStartTime[key]
	tracker.deploymentMutex.RUnlock()

	// Same revision should NOT update start time (even if generation changed)
	assert.Equal(t, secondStartTime, thirdStartTime, "Same revision should not update start time")
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
	tracker.statefulSetMutex.RLock()
	crInfo, exists := tracker.controllerRevisionMap[namespace+"/"+crName]
	tracker.statefulSetMutex.RUnlock()

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

	tracker.statefulSetMutex.RLock()
	stored, exists := tracker.statefulSetMap["default/test-statefulset"]
	tracker.statefulSetMutex.RUnlock()

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

	tracker.statefulSetMutex.Lock()
	tracker.statefulSetMap[statefulSetKey] = statefulSet
	tracker.statefulSetStartTime[statefulSetKey] = rolloutStartTime
	tracker.statefulSetMutex.Unlock()

	// Test getting duration - should return duration from StatefulSet rollout start time
	duration := tracker.GetStatefulSetRolloutDuration(namespace, statefulSetName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	// Allow for small timing differences in test
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on StatefulSet rollout start time")
	assert.GreaterOrEqual(t, duration, 0.0, "Duration should be non-negative")
}

func TestGetStatefulSetRolloutDurationFromMaps_NoStatefulSet(t *testing.T) {
	tracker := NewRolloutTracker()

	duration := tracker.GetStatefulSetRolloutDuration("default", "nonexistent-statefulset")
	assert.Equal(t, 0.0, duration, "Should return 0 when no StatefulSet found")
}

func TestGetStatefulSetRolloutDurationFromMaps_UsesStoredStartTime(t *testing.T) {
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

	// Add ControllerRevision - should NOT be used for duration (rollback fix)
	crStartTime := time.Now().Add(-3 * time.Minute)
	crInfo := &ControllerRevisionInfo{
		Name:         "test-cr-rev2",
		Namespace:    namespace,
		OwnerName:    statefulSetName,
		OwnerUID:     "sts-123",
		Revision:     2,
		CreationTime: crStartTime,
	}

	tracker.statefulSetMutex.Lock()
	tracker.statefulSetMap[statefulSetKey] = statefulSet
	tracker.statefulSetStartTime[statefulSetKey] = rolloutStartTime
	tracker.controllerRevisionMap[namespace+"/test-cr-rev2"] = crInfo
	tracker.statefulSetMutex.Unlock()

	// Test getting duration - should use STORED start time, NOT ControllerRevision time
	// This is important for rollback scenarios where CR creation time would be wrong
	duration := tracker.GetStatefulSetRolloutDuration(namespace, statefulSetName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on stored start time, not CR creation time")
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
	tracker.statefulSetMutex.Lock()
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
	tracker.statefulSetMutex.Unlock()

	// Verify initial state
	tracker.statefulSetMutex.RLock()
	assert.Equal(t, 1, len(tracker.statefulSetMap))
	assert.Equal(t, 3, len(tracker.controllerRevisionMap))
	tracker.statefulSetMutex.RUnlock()

	// Cleanup
	tracker.CleanupStatefulSet(statefulSet.Namespace, statefulSet.Name)

	// Verify cleanup
	tracker.statefulSetMutex.RLock()
	_, statefulSetExists := tracker.statefulSetMap[namespace+"/"+statefulSetName]
	_, cr1Exists := tracker.controllerRevisionMap[namespace+"/cr1"]
	_, cr2Exists := tracker.controllerRevisionMap[namespace+"/cr2"]
	_, otherCrExists := tracker.controllerRevisionMap[namespace+"/other-cr"]
	tracker.statefulSetMutex.RUnlock()

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
	tracker.statefulSetMutex.Lock()
	tracker.controllerRevisionMap[namespace+"/"+crName] = &ControllerRevisionInfo{
		Name:      crName,
		Namespace: namespace,
	}
	tracker.statefulSetMutex.Unlock()

	// Verify initial state
	tracker.statefulSetMutex.RLock()
	assert.Equal(t, 1, len(tracker.controllerRevisionMap))
	tracker.statefulSetMutex.RUnlock()

	// Cleanup
	tracker.CleanupControllerRevision(namespace, crName)

	// Verify cleanup
	tracker.statefulSetMutex.RLock()
	_, exists := tracker.controllerRevisionMap[namespace+"/"+crName]
	tracker.statefulSetMutex.RUnlock()

	assert.False(t, exists, "ControllerRevision should be removed")
}

func TestStoreStatefulSet_RevisionBasedRolloutDetection(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	statefulSetName := "test-statefulset"
	key := namespace + "/" + statefulSetName

	// Store first StatefulSet (updateRevision "sts-rev1")
	statefulSet1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 1,
		},
		Status: appsv1.StatefulSetStatus{
			UpdateRevision: "sts-rev1",
		},
	}

	tracker.StoreStatefulSet(statefulSet1)

	tracker.statefulSetMutex.RLock()
	firstStartTime := tracker.statefulSetStartTime[key]
	tracker.statefulSetMutex.RUnlock()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Store same StatefulSet with new updateRevision (new rollout)
	statefulSet2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 2,
		},
		Status: appsv1.StatefulSetStatus{
			UpdateRevision: "sts-rev2", // New revision
		},
	}

	tracker.StoreStatefulSet(statefulSet2)

	tracker.statefulSetMutex.RLock()
	secondStartTime := tracker.statefulSetStartTime[key]
	tracker.statefulSetMutex.RUnlock()

	// Second rollout should have a newer start time (updateRevision changed)
	assert.True(t, secondStartTime.After(firstStartTime), "New rollout should have updated start time")

	// Store same StatefulSet again with same updateRevision (no rollout change - e.g., scaling)
	statefulSet2Again := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       statefulSetName,
			Namespace:  namespace,
			Generation: 3, // Generation changes (e.g., scaling)
		},
		Status: appsv1.StatefulSetStatus{
			UpdateRevision: "sts-rev2", // Same revision
		},
	}

	tracker.StoreStatefulSet(statefulSet2Again)

	tracker.statefulSetMutex.RLock()
	thirdStartTime := tracker.statefulSetStartTime[key]
	tracker.statefulSetMutex.RUnlock()

	// Same updateRevision should NOT update start time (even if generation changed)
	assert.Equal(t, secondStartTime, thirdStartTime, "Same updateRevision should not update start time")
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
	tracker.daemonSetMutex.RLock()
	crInfo, exists := tracker.daemonSetControllerRevisionMap[namespace+"/"+crName]
	tracker.daemonSetMutex.RUnlock()

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

	tracker.daemonSetMutex.RLock()
	stored, exists := tracker.daemonSetMap["default/test-daemonset"]
	tracker.daemonSetMutex.RUnlock()

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

	tracker.daemonSetMutex.Lock()
	tracker.daemonSetMap[daemonSetKey] = daemonSet
	tracker.daemonSetStartTime[daemonSetKey] = rolloutStartTime
	tracker.daemonSetMutex.Unlock()

	// Test getting duration - should return duration from StatefulSet rollout start time
	duration := tracker.GetDaemonSetRolloutDuration(namespace, daemonSetName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	// Allow for small timing differences in test
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on StatefulSet rollout start time")
	assert.GreaterOrEqual(t, duration, 0.0, "Duration should be non-negative")
}

func TestGetDaemonSetRolloutDurationFromMaps_NoDaemonSet(t *testing.T) {
	tracker := NewRolloutTracker()

	duration := tracker.GetDaemonSetRolloutDuration("default", "nonexistent-daemonset")
	assert.Equal(t, 0.0, duration, "Should return 0 when no DaemonSet found")
}

func TestGetDaemonSetRolloutDurationFromMaps_UsesStoredStartTime(t *testing.T) {
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

	// Add ControllerRevision - should NOT be used for duration (rollback fix)
	crStartTime := time.Now().Add(-3 * time.Minute)
	crInfo := &ControllerRevisionInfo{
		Name:         "test-cr-rev2",
		Namespace:    namespace,
		OwnerName:    daemonSetName,
		OwnerUID:     "ds-123",
		Revision:     2,
		CreationTime: crStartTime,
	}

	tracker.daemonSetMutex.Lock()
	tracker.daemonSetMap[daemonSetKey] = daemonSet
	tracker.daemonSetStartTime[daemonSetKey] = rolloutStartTime
	tracker.daemonSetControllerRevisionMap[namespace+"/test-cr-rev2"] = crInfo
	tracker.daemonSetMutex.Unlock()

	// Test getting duration - should use STORED start time, NOT ControllerRevision time
	// This is important for rollback scenarios where CR creation time would be wrong
	duration := tracker.GetDaemonSetRolloutDuration(namespace, daemonSetName)

	expectedDuration := time.Since(rolloutStartTime).Seconds()
	assert.InDelta(t, expectedDuration, duration, 1.0, "Duration should be based on stored start time, not CR creation time")
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
	tracker.daemonSetMutex.Lock()
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
	tracker.daemonSetMutex.Unlock()

	// Verify initial state
	tracker.daemonSetMutex.RLock()
	assert.Equal(t, 1, len(tracker.daemonSetMap))
	assert.Equal(t, 3, len(tracker.daemonSetControllerRevisionMap))
	tracker.daemonSetMutex.RUnlock()

	// Cleanup
	tracker.CleanupDaemonSet(daemonSet.Namespace, daemonSet.Name)

	// Verify cleanup
	tracker.daemonSetMutex.RLock()
	_, daemonSetExists := tracker.daemonSetMap[namespace+"/"+daemonSetName]
	_, cr1Exists := tracker.controllerRevisionMap[namespace+"/cr1"]
	_, cr2Exists := tracker.daemonSetControllerRevisionMap[namespace+"/cr2"]
	_, otherCrExists := tracker.daemonSetControllerRevisionMap[namespace+"/other-cr"]
	tracker.daemonSetMutex.RUnlock()

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
	tracker.daemonSetMutex.Lock()
	tracker.daemonSetControllerRevisionMap[namespace+"/"+crName] = &ControllerRevisionInfo{
		Name:      crName,
		Namespace: namespace,
	}
	tracker.daemonSetMutex.Unlock()

	// Verify initial state
	tracker.daemonSetMutex.RLock()
	assert.Equal(t, 1, len(tracker.daemonSetControllerRevisionMap))
	tracker.daemonSetMutex.RUnlock()

	// Cleanup
	tracker.CleanupDaemonSetControllerRevision(namespace, crName)

	// Verify cleanup
	tracker.daemonSetMutex.RLock()
	_, exists := tracker.daemonSetControllerRevisionMap[namespace+"/"+crName]
	tracker.daemonSetMutex.RUnlock()

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

	tracker.daemonSetMutex.RLock()
	firstStartTime := tracker.daemonSetStartTime[key]
	tracker.daemonSetMutex.RUnlock()

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

	tracker.daemonSetMutex.RLock()
	secondStartTime := tracker.daemonSetStartTime[key]
	tracker.daemonSetMutex.RUnlock()

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

	tracker.daemonSetMutex.RLock()
	thirdStartTime := tracker.daemonSetStartTime[key]
	tracker.daemonSetMutex.RUnlock()

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

// TestDeploymentRollbackScenario tests that rollbacks are correctly detected and tracked
func TestDeploymentRollbackScenario(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"
	key := namespace + "/" + deploymentName

	// Step 1: Deploy v1 (revision "1")
	deploymentV1 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 1,
			Annotations: map[string]string{
				RevisionAnnotationKey: "1",
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
		},
	}

	// First rollout - should be tracked
	assert.True(t, tracker.HasRevisionChanged(namespace, deploymentName, "1"), "First deployment should be detected as new")
	tracker.StoreDeployment(deploymentV1)
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "1")

	v1StartTime := tracker.deploymentStartTime[key]
	assert.False(t, v1StartTime.IsZero(), "v1 start time should be set")

	// Simulate rollout completion
	tracker.CleanupDeployment(namespace, deploymentName)

	// Verify lastSeenRevision is preserved after cleanup
	tracker.deploymentMutex.RLock()
	lastRev := tracker.lastSeenRevision[key]
	tracker.deploymentMutex.RUnlock()
	assert.Equal(t, "1", lastRev, "lastSeenRevision should be preserved after cleanup")

	// Step 2: Deploy v2 (revision "2")
	time.Sleep(10 * time.Millisecond) // Ensure time difference

	deploymentV2 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 2,
			Annotations: map[string]string{
				RevisionAnnotationKey: "2",
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1, // Not yet caught up
		},
	}

	assert.True(t, tracker.HasRevisionChanged(namespace, deploymentName, "2"), "v2 should be detected as revision change")
	tracker.StoreDeployment(deploymentV2)
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "2")

	v2StartTime := tracker.deploymentStartTime[key]
	assert.True(t, v2StartTime.After(v1StartTime), "v2 start time should be after v1")

	// Complete v2 rollout
	tracker.CleanupDeployment(namespace, deploymentName)

	// Step 3: Rollback to v1 (revision increments to "3", but uses old RS)
	time.Sleep(10 * time.Millisecond)

	deploymentRollback := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 3, // Generation increases
			Annotations: map[string]string{
				RevisionAnnotationKey: "3", // Revision increments on rollback
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2, // Not yet caught up
		},
	}

	// This is the key test: rollback should be detected as a new rollout
	assert.True(t, tracker.HasRevisionChanged(namespace, deploymentName, "3"), "Rollback should be detected as revision change")
	tracker.StoreDeployment(deploymentRollback)
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "3")

	rollbackStartTime := tracker.deploymentStartTime[key]
	assert.True(t, rollbackStartTime.After(v2StartTime), "Rollback start time should be after v2")

	// Duration should be calculated from rollback start, not from old RS creation time
	duration := tracker.GetRolloutDuration(namespace, deploymentName)
	assert.GreaterOrEqual(t, duration, 0.0, "Duration should be non-negative")
	assert.Less(t, duration, 1.0, "Duration should be small (just started)")
}

// TestScalingAfterRollbackDoesNotTriggerRollout tests that scaling after a completed rollback
// does not trigger a new rollout detection
func TestScalingAfterRollbackDoesNotTriggerRollout(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"

	// Simulate completed rollback state
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "3")

	// Key test: scaling should NOT be detected as a revision change
	// (same revision "3" as before, even though generation would change)
	assert.False(t, tracker.HasRevisionChanged(namespace, deploymentName, "3"),
		"Scaling (same revision) should NOT be detected as revision change")
}

// TestPauseResumeDoesNotTriggerRollout tests that pause/resume operations
// do not trigger new rollout detection
func TestPauseResumeDoesNotTriggerRollout(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"

	// Set up initial state
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "2")

	// Pause operation
	deploymentPaused := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 3, // Generation changed due to pause
			Annotations: map[string]string{
				RevisionAnnotationKey: "2", // Revision unchanged
			},
		},
		Spec: appsv1.DeploymentSpec{
			Paused: true,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2,
		},
	}

	assert.False(t, tracker.HasRevisionChanged(namespace, deploymentName, "2"),
		"Pause should NOT be detected as revision change")

	// Resume operation
	deploymentResumed := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 4, // Generation changed again
			Annotations: map[string]string{
				RevisionAnnotationKey: "2", // Revision still unchanged
			},
		},
		Spec: appsv1.DeploymentSpec{
			Paused: false,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
		},
	}

	_ = deploymentPaused  // Used to set up context
	_ = deploymentResumed // Used to set up context

	assert.False(t, tracker.HasRevisionChanged(namespace, deploymentName, "2"),
		"Resume should NOT be detected as revision change")
}

// TestLastSeenRevisionPreservedAfterCleanup verifies that lastSeenRevision
// is preserved when CleanupDeployment is called
func TestLastSeenRevisionPreservedAfterCleanup(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"
	key := namespace + "/" + deploymentName

	// Store a deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 1,
			Annotations: map[string]string{
				RevisionAnnotationKey: "5",
			},
		},
	}

	tracker.StoreDeployment(deployment)
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "5")

	// Verify initial state
	tracker.deploymentMutex.RLock()
	_, hasDeployment := tracker.deploymentMap[key]
	_, hasStartTime := tracker.deploymentStartTime[key]
	lastRev := tracker.lastSeenRevision[key]
	tracker.deploymentMutex.RUnlock()

	assert.True(t, hasDeployment, "Deployment should be in map")
	assert.True(t, hasStartTime, "Start time should be set")
	assert.Equal(t, "5", lastRev, "lastSeenRevision should be set")

	// Cleanup
	tracker.CleanupDeployment(namespace, deploymentName)

	// Verify cleanup preserved lastSeenRevision
	tracker.deploymentMutex.RLock()
	_, hasDeployment = tracker.deploymentMap[key]
	_, hasStartTime = tracker.deploymentStartTime[key]
	lastRevAfterCleanup := tracker.lastSeenRevision[key]
	tracker.deploymentMutex.RUnlock()

	assert.False(t, hasDeployment, "Deployment should be removed from map")
	assert.False(t, hasStartTime, "Start time should be removed")
	assert.Equal(t, "5", lastRevAfterCleanup, "lastSeenRevision should be PRESERVED after cleanup")
}

// TestStatefulSetRollbackScenario tests that StatefulSet rollbacks are correctly detected
func TestStatefulSetRollbackScenario(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	stsName := "test-statefulset"
	key := namespace + "/" + stsName

	// Deploy v1
	stsV1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       stsName,
			Namespace:  namespace,
			Generation: 1,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			UpdateRevision:     "sts-rev-1",
			CurrentRevision:    "sts-rev-1",
		},
	}

	assert.True(t, tracker.HasStatefulSetRevisionChanged(namespace, stsName, "sts-rev-1"))
	tracker.StoreStatefulSet(stsV1)
	tracker.UpdateLastSeenStatefulSetRevision(namespace, stsName, "sts-rev-1")
	tracker.CleanupStatefulSet(namespace, stsName)

	// Deploy v2
	time.Sleep(10 * time.Millisecond)
	assert.True(t, tracker.HasStatefulSetRevisionChanged(namespace, stsName, "sts-rev-2"))
	stsV2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       stsName,
			Namespace:  namespace,
			Generation: 2,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			UpdateRevision:     "sts-rev-2",
			CurrentRevision:    "sts-rev-1",
		},
	}
	tracker.StoreStatefulSet(stsV2)
	tracker.UpdateLastSeenStatefulSetRevision(namespace, stsName, "sts-rev-2")
	v2StartTime := tracker.statefulSetStartTime[key]
	tracker.CleanupStatefulSet(namespace, stsName)

	// Rollback to v1 - updateRevision changes back
	time.Sleep(10 * time.Millisecond)
	assert.True(t, tracker.HasStatefulSetRevisionChanged(namespace, stsName, "sts-rev-1"),
		"Rollback to v1 should be detected as revision change")

	stsRollback := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       stsName,
			Namespace:  namespace,
			Generation: 3,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2,
			UpdateRevision:     "sts-rev-1", // Rolling back to rev-1
			CurrentRevision:    "sts-rev-2",
		},
	}

	tracker.StoreStatefulSet(stsRollback)
	tracker.UpdateLastSeenStatefulSetRevision(namespace, stsName, "sts-rev-1")

	rollbackStartTime := tracker.statefulSetStartTime[key]
	assert.True(t, rollbackStartTime.After(v2StartTime), "Rollback should have new start time")
}

// TestStatefulSetScalingAfterRollback tests that scaling doesn't trigger rollout detection
func TestStatefulSetScalingAfterRollback(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	stsName := "test-statefulset"

	// Set up state after rollback
	tracker.UpdateLastSeenStatefulSetRevision(namespace, stsName, "sts-rev-1")

	// Scaling operation - same updateRevision
	assert.False(t, tracker.HasStatefulSetRevisionChanged(namespace, stsName, "sts-rev-1"),
		"Scaling should NOT be detected as revision change")
}

// TestStatefulSetPartitionChangeDoesNotResetDuration tests that partition changes
// don't reset rollout duration
func TestStatefulSetPartitionChangeDoesNotResetDuration(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	stsName := "test-statefulset"
	key := namespace + "/" + stsName

	// Initial rollout with partition=2
	partition := int32(2)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       stsName,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &partition,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			UpdateRevision:     "sts-rev-2",
			CurrentRevision:    "sts-rev-1",
		},
	}

	tracker.StoreStatefulSet(sts)
	tracker.UpdateLastSeenStatefulSetRevision(namespace, stsName, "sts-rev-2")
	initialStartTime := tracker.statefulSetStartTime[key]

	time.Sleep(10 * time.Millisecond)

	// Change partition to 1 - same updateRevision
	partition = int32(1)
	stsPartition1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       stsName,
			Namespace:  namespace,
			Generation: 2, // Generation changes
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &partition,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			UpdateRevision:     "sts-rev-2", // Same revision
			CurrentRevision:    "sts-rev-1",
		},
	}

	// Partition change should NOT be detected as revision change
	assert.False(t, tracker.HasStatefulSetRevisionChanged(namespace, stsName, "sts-rev-2"),
		"Partition change should NOT be detected as revision change")

	tracker.StoreStatefulSet(stsPartition1)

	// Start time should NOT change
	assert.Equal(t, initialStartTime, tracker.statefulSetStartTime[key],
		"Start time should NOT change on partition change")
}

// TestDaemonSetRollbackScenario tests that DaemonSet rollbacks are correctly detected
// Note: DaemonSets don't expose UpdateRevision in their status, so we use generation-based tracking
func TestDaemonSetRollbackScenario(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	dsName := "test-daemonset"
	key := namespace + "/" + dsName

	// Deploy v1
	dsV1 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       dsName,
			Namespace:  namespace,
			Generation: 1,
		},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration: 1,
		},
	}

	tracker.StoreDaemonSet(dsV1)
	v1StartTime := tracker.daemonSetStartTime[key]
	assert.False(t, v1StartTime.IsZero(), "v1 start time should be set")
	tracker.CleanupDaemonSet(namespace, dsName)

	// Deploy v2
	time.Sleep(10 * time.Millisecond)
	dsV2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       dsName,
			Namespace:  namespace,
			Generation: 2,
		},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration: 1,
		},
	}

	tracker.StoreDaemonSet(dsV2)
	v2StartTime := tracker.daemonSetStartTime[key]
	assert.True(t, v2StartTime.After(v1StartTime), "v2 start time should be after v1")
	tracker.CleanupDaemonSet(namespace, dsName)

	// Rollback (generation changes again)
	time.Sleep(10 * time.Millisecond)
	dsRollback := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       dsName,
			Namespace:  namespace,
			Generation: 3, // Generation increases on rollback
		},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration: 2,
		},
	}

	tracker.StoreDaemonSet(dsRollback)
	rollbackStartTime := tracker.daemonSetStartTime[key]
	assert.True(t, rollbackStartTime.After(v2StartTime), "Rollback should have new start time")

	// Duration should use stored start time
	duration := tracker.GetDaemonSetRolloutDuration(namespace, dsName)
	assert.GreaterOrEqual(t, duration, 0.0, "Duration should be non-negative")
	assert.Less(t, duration, 1.0, "Duration should be small (just started)")
}

// TestGetRolloutDurationUsesStoredStartTime verifies that duration calculation
// uses stored start time and not ReplicaSet creation time
func TestGetRolloutDurationUsesStoredStartTime(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"
	key := namespace + "/" + deploymentName

	// Set a known start time
	knownStartTime := time.Now().Add(-5 * time.Minute)

	tracker.deploymentMutex.Lock()
	tracker.deploymentStartTime[key] = knownStartTime
	// Add an old ReplicaSet that should NOT be used for duration
	tracker.replicaSetMap[namespace+"/old-rs"] = &ReplicaSetInfo{
		Name:         "old-rs",
		Namespace:    namespace,
		OwnerName:    deploymentName,
		CreationTime: time.Now().Add(-1 * time.Hour), // Much older
	}
	tracker.deploymentMutex.Unlock()

	duration := tracker.GetRolloutDuration(namespace, deploymentName)

	// Duration should be ~5 minutes (from stored start time), not 1 hour (from RS)
	expectedDuration := time.Since(knownStartTime).Seconds()
	assert.InDelta(t, expectedDuration, duration, 1.0,
		"Duration should be based on stored start time, not ReplicaSet creation time")
	assert.Less(t, duration, 600.0, "Duration should be less than 10 minutes")
}

// TestRevisionAnnotationKey verifies the constant is correctly defined
func TestRevisionAnnotationKey(t *testing.T) {
	assert.Equal(t, "deployment.kubernetes.io/revision", RevisionAnnotationKey,
		"RevisionAnnotationKey should match Kubernetes standard")
}

// TestFastReconciliationDeployment tests that we detect rollouts even when
// Kubernetes reconciles the generation quickly (within the 15s scrape interval)
// but the rollout condition is still active (pods still rolling).
// This was a bug where we required generationMismatch && revisionChanged,
// but if generation catches up fast, we'd miss the rollout entirely.
func TestFastReconciliationDeployment(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "fast-reconcile-deploy"

	// Simulate: We've seen revision "1" before
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "1")

	// Scenario: A rollout was triggered, but by the time we check:
	// - Generation has already caught up (observedGeneration == generation)
	// - But revision changed from "1" to "2"
	// - And rollout condition is still active (pods still rolling)

	// Check that revision changed is detected
	assert.True(t, tracker.HasRevisionChanged(namespace, deploymentName, "2"),
		"Should detect revision changed from 1 to 2")

	// The deployment with fast reconciliation
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 2,
			Annotations: map[string]string{
				RevisionAnnotationKey: "2",
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2, // Already caught up!
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
					Reason: "ReplicaSetUpdated", // Still rolling
				},
			},
		},
	}

	// Verify the conditions
	generationMismatch := deployment.Generation != deployment.Status.ObservedGeneration
	assert.False(t, generationMismatch, "Generation should have caught up (no mismatch)")

	hasRolloutCondition := tracker.HasRolloutCondition(deployment)
	assert.True(t, hasRolloutCondition, "Should have active rollout condition")

	revisionChanged := tracker.HasRevisionChanged(namespace, deploymentName, "2")
	assert.True(t, revisionChanged, "Revision should have changed")

	// With the OLD logic: (generationMismatch && revisionChanged) = (false && true) = false
	// With the NEW logic: (revisionChanged && (generationMismatch || hasRolloutCondition)) = (true && (false || true)) = true
	isActivelyTracked := tracker.HasActiveRollout(deployment)
	newLogicIsOngoing := (revisionChanged && (generationMismatch || hasRolloutCondition)) ||
		(isActivelyTracked && hasRolloutCondition)
	assert.True(t, newLogicIsOngoing, "New logic should detect rollout even with fast reconciliation")

	// Store the deployment (simulating what the metric generator does)
	tracker.StoreDeployment(deployment)

	// Verify it's now tracked
	assert.True(t, tracker.HasActiveRollout(deployment), "Deployment should now be actively tracked")

	// Verify duration is being tracked
	duration := tracker.GetRolloutDuration(namespace, deploymentName)
	assert.GreaterOrEqual(t, duration, 0.0, "Duration should be non-negative")
}

// TestFastReconciliationStatefulSet tests the same scenario for StatefulSets
func TestFastReconciliationStatefulSet(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	stsName := "fast-reconcile-sts"

	// Simulate: We've seen updateRevision "sts-rev-1" before
	tracker.UpdateLastSeenStatefulSetRevision(namespace, stsName, "sts-rev-1")

	// Check that revision changed is detected
	assert.True(t, tracker.HasStatefulSetRevisionChanged(namespace, stsName, "sts-rev-2"),
		"Should detect updateRevision changed from sts-rev-1 to sts-rev-2")

	// StatefulSet with fast reconciliation
	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       stsName,
			Namespace:  namespace,
			Generation: 2,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2, // Already caught up!
			UpdateRevision:     "sts-rev-2",
			CurrentRevision:    "sts-rev-1", // Still rolling (current != update)
			ReadyReplicas:      1,           // Only 1 of 3 ready
			UpdatedReplicas:    1,           // Only 1 updated
		},
	}

	// Verify the conditions
	generationMismatch := sts.Generation != sts.Status.ObservedGeneration
	assert.False(t, generationMismatch, "Generation should have caught up (no mismatch)")

	hasRolloutCondition := tracker.HasStatefulSetRolloutCondition(sts)
	assert.True(t, hasRolloutCondition, "Should have active rollout condition (revisions mismatch)")

	revisionChanged := tracker.HasStatefulSetRevisionChanged(namespace, stsName, "sts-rev-2")
	assert.True(t, revisionChanged, "Revision should have changed")

	// Verify new logic works
	isActivelyTracked := tracker.HasActiveStatefulSetRollout(sts)
	newLogicIsOngoing := (revisionChanged && (generationMismatch || hasRolloutCondition)) ||
		(isActivelyTracked && hasRolloutCondition)
	assert.True(t, newLogicIsOngoing, "New logic should detect rollout even with fast reconciliation")

	// Store the StatefulSet
	tracker.StoreStatefulSet(sts)

	// Verify it's now tracked
	assert.True(t, tracker.HasActiveStatefulSetRollout(sts), "StatefulSet should now be actively tracked")

	// Verify duration is being tracked
	duration := tracker.GetStatefulSetRolloutDuration(namespace, stsName)
	assert.GreaterOrEqual(t, duration, 0.0, "Duration should be non-negative")
}

// TestVeryFastRolloutCompletesBeforeCheck verifies that if a rollout completes
// entirely between checks (< 15s), we do NOT report it. This is acceptable behavior.
func TestVeryFastRolloutCompletesBeforeCheck(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "super-fast-deploy"

	// Simulate: We've seen revision "1" before
	tracker.UpdateLastSeenRevision(namespace, deploymentName, "1")

	// Scenario: A rollout was triggered and COMPLETED before we could check:
	// - Generation already caught up
	// - Revision changed from "1" to "2"
	// - BUT rollout condition is FALSE (already complete)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 2,
			Annotations: map[string]string{
				RevisionAnnotationKey: "2",
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
					Reason: "NewReplicaSetAvailable", // Rollout complete!
				},
			},
		},
	}

	generationMismatch := deployment.Generation != deployment.Status.ObservedGeneration
	hasRolloutCondition := tracker.HasRolloutCondition(deployment)
	revisionChanged := tracker.HasRevisionChanged(namespace, deploymentName, "2")
	isActivelyTracked := tracker.HasActiveRollout(deployment)

	assert.False(t, generationMismatch, "Generation caught up")
	assert.False(t, hasRolloutCondition, "Rollout condition should be false (complete)")
	assert.True(t, revisionChanged, "Revision did change")
	assert.False(t, isActivelyTracked, "Not actively tracked")

	// With new logic: (revisionChanged && (generationMismatch || hasRolloutCondition))
	// = (true && (false || false)) = false
	// This is correct - we don't report rollouts that complete between checks
	newLogicIsOngoing := (revisionChanged && (generationMismatch || hasRolloutCondition)) ||
		(isActivelyTracked && hasRolloutCondition)
	assert.False(t, newLogicIsOngoing,
		"Should NOT track rollout that completed before we checked - this is acceptable")
}

// TestAgentRestartDuringRollout verifies that if the agent restarts during an
// ongoing rollout, we can still detect and track it (as long as rollout condition is active)
func TestAgentRestartDuringRollout(t *testing.T) {
	// Fresh tracker simulates agent restart - no previous state
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "restart-test-deploy"

	// Deployment mid-rollout when agent starts fresh:
	// - We've never seen this deployment before (no lastSeenRevision)
	// - Generation already caught up
	// - Rollout condition is active

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       deploymentName,
			Namespace:  namespace,
			Generation: 5,
			Annotations: map[string]string{
				RevisionAnnotationKey: "5",
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 5, // Already caught up
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
					Reason: "ReplicaSetUpdated", // Still rolling
				},
			},
		},
	}

	// Never seen before - revision is "changed" (new)
	revisionChanged := tracker.HasRevisionChanged(namespace, deploymentName, "5")
	assert.True(t, revisionChanged, "Never-seen revision should be treated as changed")

	generationMismatch := deployment.Generation != deployment.Status.ObservedGeneration
	hasRolloutCondition := tracker.HasRolloutCondition(deployment)
	isActivelyTracked := tracker.HasActiveRollout(deployment)

	assert.False(t, generationMismatch, "Generation caught up")
	assert.True(t, hasRolloutCondition, "Rollout still in progress")
	assert.False(t, isActivelyTracked, "Not tracked (just started)")

	// With new logic: (revisionChanged && (generationMismatch || hasRolloutCondition))
	// = (true && (false || true)) = true
	newLogicIsOngoing := (revisionChanged && (generationMismatch || hasRolloutCondition)) ||
		(isActivelyTracked && hasRolloutCondition)
	assert.True(t, newLogicIsOngoing,
		"Should detect ongoing rollout after agent restart")

	// Store and verify tracking
	tracker.StoreDeployment(deployment)
	assert.True(t, tracker.HasActiveRollout(deployment), "Should now be actively tracked")
}

// =============================================================================
// Tests for RecentCreationThreshold heuristic
// =============================================================================

// TestDetermineDeploymentStartTime_RecentRS tests that a recent ReplicaSet's
// creation time is used for new deployments (agent restart during forward rollout)
func TestDetermineDeploymentStartTime_RecentRS(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"

	// Add a ReplicaSet created 2 minutes ago (within threshold)
	recentRSCreationTime := time.Now().Add(-2 * time.Minute)
	tracker.deploymentMutex.Lock()
	tracker.replicaSetMap[namespace+"/test-rs"] = &ReplicaSetInfo{
		Name:         "test-rs",
		Namespace:    namespace,
		OwnerName:    deploymentName,
		OwnerUID:     "dep-123",
		CreationTime: recentRSCreationTime,
	}
	tracker.deploymentMutex.Unlock()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
	}

	// Call determineDeploymentStartTime
	tracker.deploymentMutex.Lock()
	startTime := tracker.determineDeploymentStartTime(deployment)
	tracker.deploymentMutex.Unlock()

	// Should use the RS creation time since it's recent
	assert.Equal(t, recentRSCreationTime, startTime,
		"Should use recent RS creation time for start time")
}

// TestDetermineDeploymentStartTime_OldRS tests that when the RS is old,
// we fall back to Progressing condition or now (agent restart during rollback)
func TestDetermineDeploymentStartTime_OldRS(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"

	// Add a ReplicaSet created 10 minutes ago (outside threshold)
	oldRSCreationTime := time.Now().Add(-10 * time.Minute)
	tracker.deploymentMutex.Lock()
	tracker.replicaSetMap[namespace+"/test-rs"] = &ReplicaSetInfo{
		Name:         "test-rs",
		Namespace:    namespace,
		OwnerName:    deploymentName,
		OwnerUID:     "dep-123",
		CreationTime: oldRSCreationTime,
	}
	tracker.deploymentMutex.Unlock()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
	}

	beforeCall := time.Now()
	tracker.deploymentMutex.Lock()
	startTime := tracker.determineDeploymentStartTime(deployment)
	tracker.deploymentMutex.Unlock()
	afterCall := time.Now()

	// Should NOT use the old RS creation time
	assert.NotEqual(t, oldRSCreationTime, startTime,
		"Should NOT use old RS creation time")

	// Should use current time (or close to it) since no Progressing condition
	assert.True(t, startTime.After(beforeCall) || startTime.Equal(beforeCall),
		"Start time should be at or after the call")
	assert.True(t, startTime.Before(afterCall) || startTime.Equal(afterCall),
		"Start time should be at or before call completed")
}

// TestDetermineDeploymentStartTime_WithProgressingCondition tests that
// when RS is old but Progressing condition exists, we use it
func TestDetermineDeploymentStartTime_WithProgressingCondition(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"

	// Add an old ReplicaSet (outside threshold)
	tracker.deploymentMutex.Lock()
	tracker.replicaSetMap[namespace+"/test-rs"] = &ReplicaSetInfo{
		Name:         "test-rs",
		Namespace:    namespace,
		OwnerName:    deploymentName,
		OwnerUID:     "dep-123",
		CreationTime: time.Now().Add(-10 * time.Minute),
	}
	tracker.deploymentMutex.Unlock()

	// Deployment with Progressing condition
	progressingTime := time.Now().Add(-3 * time.Minute)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentProgressing,
					Status:             corev1.ConditionTrue,
					Reason:             "ReplicaSetUpdated",
					LastTransitionTime: metav1.Time{Time: progressingTime},
				},
			},
		},
	}

	tracker.deploymentMutex.Lock()
	startTime := tracker.determineDeploymentStartTime(deployment)
	tracker.deploymentMutex.Unlock()

	// Should use the Progressing condition time
	assert.Equal(t, progressingTime, startTime,
		"Should use Progressing condition LastTransitionTime")
}

// TestDetermineStatefulSetStartTime_RecentCR tests that a recent ControllerRevision's
// creation time is used for new StatefulSets
func TestDetermineStatefulSetStartTime_RecentCR(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	stsName := "test-sts"

	// Add a ControllerRevision created 2 minutes ago (within threshold)
	recentCRCreationTime := time.Now().Add(-2 * time.Minute)
	tracker.statefulSetMutex.Lock()
	tracker.controllerRevisionMap[namespace+"/test-cr"] = &ControllerRevisionInfo{
		Name:         "test-cr",
		Namespace:    namespace,
		OwnerName:    stsName,
		OwnerUID:     "sts-123",
		Revision:     1,
		CreationTime: recentCRCreationTime,
	}
	tracker.statefulSetMutex.Unlock()

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stsName,
			Namespace: namespace,
		},
	}

	tracker.statefulSetMutex.Lock()
	startTime := tracker.determineStatefulSetStartTime(sts)
	tracker.statefulSetMutex.Unlock()

	// Should use the CR creation time since it's recent
	assert.Equal(t, recentCRCreationTime, startTime,
		"Should use recent CR creation time for start time")
}

// TestDetermineStatefulSetStartTime_OldCR tests that when CR is old,
// we use current time (agent restart during rollback)
func TestDetermineStatefulSetStartTime_OldCR(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	stsName := "test-sts"

	// Add a ControllerRevision created 10 minutes ago (outside threshold)
	oldCRCreationTime := time.Now().Add(-10 * time.Minute)
	tracker.statefulSetMutex.Lock()
	tracker.controllerRevisionMap[namespace+"/test-cr"] = &ControllerRevisionInfo{
		Name:         "test-cr",
		Namespace:    namespace,
		OwnerName:    stsName,
		OwnerUID:     "sts-123",
		Revision:     1,
		CreationTime: oldCRCreationTime,
	}
	tracker.statefulSetMutex.Unlock()

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stsName,
			Namespace: namespace,
		},
	}

	beforeCall := time.Now()
	tracker.statefulSetMutex.Lock()
	startTime := tracker.determineStatefulSetStartTime(sts)
	tracker.statefulSetMutex.Unlock()
	afterCall := time.Now()

	// Should NOT use the old CR creation time
	assert.NotEqual(t, oldCRCreationTime, startTime,
		"Should NOT use old CR creation time")

	// Should use current time
	assert.True(t, startTime.After(beforeCall) || startTime.Equal(beforeCall),
		"Start time should be at or after the call")
	assert.True(t, startTime.Before(afterCall) || startTime.Equal(afterCall),
		"Start time should be at or before call completed")
}

// TestDetermineDaemonSetStartTime_RecentCR tests that a recent ControllerRevision's
// creation time is used for new DaemonSets
func TestDetermineDaemonSetStartTime_RecentCR(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	dsName := "test-ds"

	// Add a ControllerRevision created 2 minutes ago (within threshold)
	recentCRCreationTime := time.Now().Add(-2 * time.Minute)
	tracker.daemonSetMutex.Lock()
	tracker.daemonSetControllerRevisionMap[namespace+"/test-cr"] = &ControllerRevisionInfo{
		Name:         "test-cr",
		Namespace:    namespace,
		OwnerName:    dsName,
		OwnerUID:     "ds-123",
		Revision:     1,
		CreationTime: recentCRCreationTime,
	}
	tracker.daemonSetMutex.Unlock()

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dsName,
			Namespace: namespace,
		},
	}

	tracker.daemonSetMutex.Lock()
	startTime := tracker.determineDaemonSetStartTime(ds)
	tracker.daemonSetMutex.Unlock()

	// Should use the CR creation time since it's recent
	assert.Equal(t, recentCRCreationTime, startTime,
		"Should use recent CR creation time for start time")
}

// TestDetermineDaemonSetStartTime_OldCR tests that when CR is old,
// we use current time (agent restart during rollback)
func TestDetermineDaemonSetStartTime_OldCR(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	dsName := "test-ds"

	// Add a ControllerRevision created 10 minutes ago (outside threshold)
	oldCRCreationTime := time.Now().Add(-10 * time.Minute)
	tracker.daemonSetMutex.Lock()
	tracker.daemonSetControllerRevisionMap[namespace+"/test-cr"] = &ControllerRevisionInfo{
		Name:         "test-cr",
		Namespace:    namespace,
		OwnerName:    dsName,
		OwnerUID:     "ds-123",
		Revision:     1,
		CreationTime: oldCRCreationTime,
	}
	tracker.daemonSetMutex.Unlock()

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dsName,
			Namespace: namespace,
		},
	}

	beforeCall := time.Now()
	tracker.daemonSetMutex.Lock()
	startTime := tracker.determineDaemonSetStartTime(ds)
	tracker.daemonSetMutex.Unlock()
	afterCall := time.Now()

	// Should NOT use the old CR creation time
	assert.NotEqual(t, oldCRCreationTime, startTime,
		"Should NOT use old CR creation time")

	// Should use current time
	assert.True(t, startTime.After(beforeCall) || startTime.Equal(beforeCall),
		"Start time should be at or after the call")
	assert.True(t, startTime.Before(afterCall) || startTime.Equal(afterCall),
		"Start time should be at or before call completed")
}

// TestStoreDeployment_UsesRecentRSOnFirstSee tests that when a deployment is first
// seen and has a recent RS, the RS creation time is used as start time
func TestStoreDeployment_UsesRecentRSOnFirstSee(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"
	key := namespace + "/" + deploymentName

	// Pre-populate a recent ReplicaSet (simulating RS being stored before Deployment)
	recentRSCreationTime := time.Now().Add(-2 * time.Minute)
	tracker.StoreReplicaSet(&appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-rs",
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: recentRSCreationTime},
		},
	}, deploymentName, "dep-123")

	// Now store the deployment for the first time
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Annotations: map[string]string{
				RevisionAnnotationKey: "1",
			},
		},
	}

	tracker.StoreDeployment(deployment)

	// Verify the start time was set to the RS creation time
	tracker.deploymentMutex.RLock()
	startTime := tracker.deploymentStartTime[key]
	tracker.deploymentMutex.RUnlock()

	assert.Equal(t, recentRSCreationTime, startTime,
		"First-time deployment with recent RS should use RS creation time")
}

// TestStoreDeployment_UsesNowWhenRSIsOld tests that when a deployment is first
// seen but RS is old (rollback scenario), current time is used
func TestStoreDeployment_UsesNowWhenRSIsOld(t *testing.T) {
	tracker := NewRolloutTracker()

	namespace := "default"
	deploymentName := "test-deployment"
	key := namespace + "/" + deploymentName

	// Pre-populate an old ReplicaSet (simulating rollback - reusing old RS)
	oldRSCreationTime := time.Now().Add(-10 * time.Minute)
	tracker.StoreReplicaSet(&appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-rs",
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: oldRSCreationTime},
		},
	}, deploymentName, "dep-123")

	beforeStore := time.Now()

	// Now store the deployment for the first time
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Annotations: map[string]string{
				RevisionAnnotationKey: "1",
			},
		},
	}

	tracker.StoreDeployment(deployment)

	afterStore := time.Now()

	// Verify the start time was NOT set to the old RS creation time
	tracker.deploymentMutex.RLock()
	startTime := tracker.deploymentStartTime[key]
	tracker.deploymentMutex.RUnlock()

	assert.NotEqual(t, oldRSCreationTime, startTime,
		"Should NOT use old RS creation time")
	assert.True(t, startTime.After(beforeStore) || startTime.Equal(beforeStore),
		"Start time should be at or after store call")
	assert.True(t, startTime.Before(afterStore) || startTime.Equal(afterStore),
		"Start time should be at or before store completed")
}

// TestRecentCreationThreshold verifies the threshold constant is correctly defined
func TestRecentCreationThreshold(t *testing.T) {
	assert.Equal(t, 5*time.Minute, RecentCreationThreshold,
		"RecentCreationThreshold should be 5 minutes")
}
