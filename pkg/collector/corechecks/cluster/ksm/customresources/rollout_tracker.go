// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// ReplicaSetInfo holds information about a ReplicaSet for Deployment rollout tracking
type ReplicaSetInfo struct {
	Name         string
	Namespace    string
	CreationTime time.Time
	OwnerUID     string // UID of owning Deployment
	OwnerName    string // Name of owning Deployment
}

// ControllerRevisionInfo holds information about a ControllerRevision for StatefulSet rollout tracking
type ControllerRevisionInfo struct {
	Name         string
	Namespace    string
	CreationTime time.Time
	Revision     int64  // Revision number
	OwnerUID     string // UID of owning StatefulSet
	OwnerName    string // Name of owning StatefulSet
}

// RolloutOperations interface defines operations for rollout tracking
type RolloutOperations interface {
	// Deployment operations
	StoreDeployment(dep *appsv1.Deployment)
	StoreReplicaSet(rs *appsv1.ReplicaSet, ownerName, ownerUID string)
	GetRolloutDuration(namespace, deploymentName string) float64
	CleanupDeployment(namespace, name string)
	CleanupReplicaSet(namespace, name string)
	HasActiveRollout(d *appsv1.Deployment) bool
	HasRolloutCondition(d *appsv1.Deployment) bool

	// StatefulSet operations
	StoreStatefulSet(sts *appsv1.StatefulSet)
	StoreControllerRevision(cr *appsv1.ControllerRevision, ownerName, ownerUID string)
	GetStatefulSetRolloutDuration(namespace, statefulSetName string) float64
	CleanupStatefulSet(namespace, name string)
	CleanupControllerRevision(namespace, name string)
	HasActiveStatefulSetRollout(sts *appsv1.StatefulSet) bool
	HasStatefulSetRolloutCondition(sts *appsv1.StatefulSet) bool
}

// RolloutTracker manages rollout state for a KSM check instance
type RolloutTracker struct {
	// Deployment tracking
	deploymentMap        map[string]*appsv1.Deployment
	deploymentAccessTime map[string]time.Time // Track when each deployment was last accessed
	deploymentStartTime  map[string]time.Time // Track when each rollout started
	replicaSetMap        map[string]*ReplicaSetInfo

	// StatefulSet tracking
	statefulSetMap        map[string]*appsv1.StatefulSet
	statefulSetAccessTime map[string]time.Time // Track when each StatefulSet was last accessed
	statefulSetStartTime  map[string]time.Time // Track when each rollout started
	controllerRevisionMap map[string]*ControllerRevisionInfo

	mutex sync.RWMutex
}

// NewRolloutTracker creates a new RolloutTracker instance
func NewRolloutTracker() *RolloutTracker {
	return &RolloutTracker{
		// Deployment maps
		deploymentMap:        make(map[string]*appsv1.Deployment),
		deploymentAccessTime: make(map[string]time.Time),
		deploymentStartTime:  make(map[string]time.Time),
		replicaSetMap:        make(map[string]*ReplicaSetInfo),

		// StatefulSet maps
		statefulSetMap:        make(map[string]*appsv1.StatefulSet),
		statefulSetAccessTime: make(map[string]time.Time),
		statefulSetStartTime:  make(map[string]time.Time),
		controllerRevisionMap: make(map[string]*ControllerRevisionInfo),
	}
}

// StoreReplicaSet stores a ReplicaSet for deployment rollout tracking
func (rt *RolloutTracker) StoreReplicaSet(rs *appsv1.ReplicaSet, ownerName, ownerUID string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := rs.Namespace + "/" + rs.Name
	rt.replicaSetMap[key] = &ReplicaSetInfo{
		Name:         rs.Name,
		Namespace:    rs.Namespace,
		CreationTime: rs.CreationTimestamp.Time,
		OwnerUID:     ownerUID,
		OwnerName:    ownerName,
	}

}

// GetRolloutDuration calculates rollout duration using stored maps
func (rt *RolloutTracker) GetRolloutDuration(namespace, deploymentName string) float64 {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	deploymentKey := namespace + "/" + deploymentName

	// Update access time for this deployment
	if _, exists := rt.deploymentMap[deploymentKey]; exists {
		rt.deploymentAccessTime[deploymentKey] = time.Now()
	}

	// Try to use the newest ReplicaSet creation time, fall back to deployment start time
	var startTime time.Time

	// First, look for the newest ReplicaSet owned by this deployment
	var newestRS *ReplicaSetInfo
	for _, rsInfo := range rt.replicaSetMap {
		if rsInfo.Namespace == namespace && rsInfo.OwnerName == deploymentName {
			if newestRS == nil || rsInfo.CreationTime.After(newestRS.CreationTime) {
				newestRS = rsInfo
			}
		}
	}

	if newestRS != nil {
		startTime = newestRS.CreationTime
	} else {
		// Fall back to deployment start time
		deploymentStartTime, hasStartTime := rt.deploymentStartTime[deploymentKey]
		if !hasStartTime || deploymentStartTime.IsZero() {
			return 0
		}
		startTime = deploymentStartTime
	}

	duration := time.Since(startTime)
	return duration.Seconds()
}

// StoreDeployment stores a deployment for rollout tracking
func (rt *RolloutTracker) StoreDeployment(dep *appsv1.Deployment) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := dep.Namespace + "/" + dep.Name

	// Check if this is a new rollout (new deployment OR generation changed)
	existingDep, exists := rt.deploymentMap[key]
	if !exists {
		rt.deploymentStartTime[key] = time.Now()
	} else if existingDep.Generation != dep.Generation {
		rt.deploymentStartTime[key] = time.Now()
	}

	rt.deploymentMap[key] = dep.DeepCopy()
	rt.deploymentAccessTime[key] = time.Now() // Track when we stored it
}

// CleanupDeployment removes a deployment and its ReplicaSets from tracking
func (rt *RolloutTracker) CleanupDeployment(namespace, name string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := namespace + "/" + name

	// Remove deployment
	delete(rt.deploymentMap, key)
	delete(rt.deploymentAccessTime, key) // Also remove access time tracking
	delete(rt.deploymentStartTime, key)  // Also remove rollout start time

	// Remove associated ReplicaSets
	removedReplicaSets := 0
	for rsKey, rsInfo := range rt.replicaSetMap {
		if rsInfo.Namespace == namespace && rsInfo.OwnerName == name {
			delete(rt.replicaSetMap, rsKey)
			removedReplicaSets++
		}
	}
}

// CleanupReplicaSet removes a deleted ReplicaSet from tracking
func (rt *RolloutTracker) CleanupReplicaSet(namespace, name string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := namespace + "/" + name

	delete(rt.replicaSetMap, key)
}

// HasActiveRollout checks if we're tracking a rollout for the given deployment's current generation
func (rt *RolloutTracker) HasActiveRollout(d *appsv1.Deployment) bool {
	key := d.Namespace + "/" + d.Name
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()

	storedDep, exists := rt.deploymentMap[key]
	if !exists {
		return false
	}

	// We're tracking a rollout if stored generation matches current generation
	return storedDep.Generation == d.Generation
}

// HasRolloutCondition checks if Kubernetes reports the deployment as progressing
func (rt *RolloutTracker) HasRolloutCondition(d *appsv1.Deployment) bool {
	for _, condition := range d.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			return condition.Status == corev1.ConditionTrue &&
				condition.Reason == "ReplicaSetUpdated"
		}
	}
	return false
}

// StoreControllerRevision stores a ControllerRevision for StatefulSet rollout tracking
func (rt *RolloutTracker) StoreControllerRevision(cr *appsv1.ControllerRevision, ownerName, ownerUID string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := cr.Namespace + "/" + cr.Name
	rt.controllerRevisionMap[key] = &ControllerRevisionInfo{
		Name:         cr.Name,
		Namespace:    cr.Namespace,
		CreationTime: cr.CreationTimestamp.Time,
		Revision:     cr.Revision,
		OwnerUID:     ownerUID,
		OwnerName:    ownerName,
	}

}

// GetStatefulSetRolloutDuration calculates StatefulSet rollout duration using stored maps
func (rt *RolloutTracker) GetStatefulSetRolloutDuration(namespace, statefulSetName string) float64 {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	statefulSetKey := namespace + "/" + statefulSetName

	// Update access time for this StatefulSet
	if _, exists := rt.statefulSetMap[statefulSetKey]; exists {
		rt.statefulSetAccessTime[statefulSetKey] = time.Now()
	}

	// Try to use the newest ControllerRevision creation time, fall back to StatefulSet start time
	var startTime time.Time

	// First, look for the newest ControllerRevision owned by this StatefulSet
	var newestCR *ControllerRevisionInfo
	for _, crInfo := range rt.controllerRevisionMap {
		if crInfo.Namespace == namespace && crInfo.OwnerName == statefulSetName {
			if newestCR == nil || crInfo.Revision > newestCR.Revision {
				newestCR = crInfo
			}
		}
	}

	if newestCR != nil {
		startTime = newestCR.CreationTime
	} else {
		// Fall back to StatefulSet start time
		statefulSetStartTime, hasStartTime := rt.statefulSetStartTime[statefulSetKey]
		if !hasStartTime || statefulSetStartTime.IsZero() {
			return 0
		}
		startTime = statefulSetStartTime
	}

	duration := time.Since(startTime)
	return duration.Seconds()
}

// StoreStatefulSet stores a StatefulSet for rollout tracking
func (rt *RolloutTracker) StoreStatefulSet(sts *appsv1.StatefulSet) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := sts.Namespace + "/" + sts.Name

	// Check if this is a new rollout (new StatefulSet OR generation changed)
	existingSts, exists := rt.statefulSetMap[key]
	if !exists {
		rt.statefulSetStartTime[key] = time.Now()
	} else if existingSts.Generation != sts.Generation {
		rt.statefulSetStartTime[key] = time.Now()
	}

	rt.statefulSetMap[key] = sts.DeepCopy()
	rt.statefulSetAccessTime[key] = time.Now() // Track when we stored it
}

// CleanupStatefulSet removes a StatefulSet and its ControllerRevisions from tracking
func (rt *RolloutTracker) CleanupStatefulSet(namespace, name string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := namespace + "/" + name

	// Remove StatefulSet
	delete(rt.statefulSetMap, key)
	delete(rt.statefulSetAccessTime, key) // Also remove access time tracking
	delete(rt.statefulSetStartTime, key)  // Also remove rollout start time

	// Remove associated ControllerRevisions
	for crKey, crInfo := range rt.controllerRevisionMap {
		if crInfo.Namespace == namespace && crInfo.OwnerName == name {
			delete(rt.controllerRevisionMap, crKey)
		}
	}
}

// CleanupControllerRevision removes a deleted ControllerRevision from tracking
func (rt *RolloutTracker) CleanupControllerRevision(namespace, name string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	key := namespace + "/" + name
	delete(rt.controllerRevisionMap, key)
}

// HasActiveStatefulSetRollout checks if we're tracking a rollout for the given StatefulSet's current generation
func (rt *RolloutTracker) HasActiveStatefulSetRollout(sts *appsv1.StatefulSet) bool {
	key := sts.Namespace + "/" + sts.Name
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()

	storedSts, exists := rt.statefulSetMap[key]
	if !exists {
		return false
	}

	// We're tracking a rollout if stored generation matches current generation
	return storedSts.Generation == sts.Generation
}

// HasStatefulSetRolloutCondition checks if Kubernetes reports the StatefulSet as updating
func (rt *RolloutTracker) HasStatefulSetRolloutCondition(sts *appsv1.StatefulSet) bool {
	desiredReplicas := int32(1)
	if sts.Spec.Replicas != nil {
		desiredReplicas = *sts.Spec.Replicas
	}

	// Special case: nil replicas with no updated replicas should be considered complete
	// BUT only if revisions match (no ongoing rollout)
	if sts.Spec.Replicas == nil && sts.Status.UpdatedReplicas == 0 &&
		sts.Status.UpdateRevision == sts.Status.CurrentRevision {
		return false
	}

	// OnDelete strategy means that the revision is updated, but the changes
	// aren't applied until the pods are manually deleted or restarted.
	// Should we consider this ongoing?
	if sts.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return sts.Status.UpdateRevision != sts.Status.CurrentRevision
	}

	// RollingUpdate strategy (default) - get partition value
	partition := int32(0)
	if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		partition = *sts.Spec.UpdateStrategy.RollingUpdate.Partition
	}

	// Check revision mismatch - but for partitions, verify if rollout is actually complete
	if sts.Status.UpdateRevision != sts.Status.CurrentRevision {
		// For partition rollouts, check if the partition rollout is actually complete
		if partition > 0 {
			expectedUpdatedPods := desiredReplicas - partition
			// Partition rollout complete if: expected pods updated AND all pods ready
			if sts.Status.UpdatedReplicas >= expectedUpdatedPods && sts.Status.ReadyReplicas >= desiredReplicas {
				return false
			}
		}
		return true
	}

	// At this point revisions match - only check replica readiness
	// Don't check UpdatedReplicas when revisions match as it causes false positives during node migration
	// Partitioning scenarios are handled above when revisions mismatch
	if sts.Status.ReadyReplicas < desiredReplicas {
		return true
	}
	return false
}
