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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RevisionAnnotationKey is the annotation key used by Kubernetes to track deployment revisions
const RevisionAnnotationKey = "deployment.kubernetes.io/revision"

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
	HasRevisionChanged(namespace, name, currentRevision string) bool
	UpdateLastSeenRevision(namespace, name, revision string)

	// StatefulSet operations
	StoreStatefulSet(sts *appsv1.StatefulSet)
	StoreControllerRevision(cr *appsv1.ControllerRevision, ownerName, ownerUID string)
	GetStatefulSetRolloutDuration(namespace, statefulSetName string) float64
	CleanupStatefulSet(namespace, name string)
	CleanupControllerRevision(namespace, name string)
	HasActiveStatefulSetRollout(sts *appsv1.StatefulSet) bool
	HasStatefulSetRolloutCondition(sts *appsv1.StatefulSet) bool
	HasStatefulSetRevisionChanged(namespace, name, currentUpdateRevision string) bool
	UpdateLastSeenStatefulSetRevision(namespace, name, updateRevision string)

	// DaemonSet operations
	// Note: DaemonSets don't expose UpdateRevision in their status like StatefulSets do,
	// so we use generation-based tracking for DaemonSets.
	StoreDaemonSet(ds *appsv1.DaemonSet)
	StoreDaemonSetControllerRevision(cr *appsv1.ControllerRevision, ownerName, ownerUID string)
	GetDaemonSetRolloutDuration(namespace, daemonSetName string) float64
	CleanupDaemonSet(namespace, name string)
	CleanupDaemonSetControllerRevision(namespace, name string)
	HasActiveDaemonSetRollout(ds *appsv1.DaemonSet) bool
	HasDaemonSetRolloutCondition(ds *appsv1.DaemonSet) bool
}

// RolloutTracker manages rollout state for a KSM check instance
type RolloutTracker struct {
	// Deployment tracking - active rollout state (cleared on completion)
	deploymentMap       map[string]*appsv1.Deployment
	deploymentStartTime map[string]time.Time // Track when each rollout started
	replicaSetMap       map[string]*ReplicaSetInfo
	// Persistent revision tracking (preserved across rollout completions to detect new rollouts vs scaling)
	lastSeenRevision map[string]string
	deploymentMutex  sync.RWMutex

	// StatefulSet tracking - active rollout state (cleared on completion)
	statefulSetMap        map[string]*appsv1.StatefulSet
	statefulSetStartTime  map[string]time.Time // Track when each rollout started
	controllerRevisionMap map[string]*ControllerRevisionInfo
	// Persistent revision tracking
	lastSeenUpdateRevision map[string]string
	statefulSetMutex       sync.RWMutex

	// DaemonSet tracking - active rollout state (cleared on completion)
	// Note: DaemonSets don't expose UpdateRevision in their status, so we use generation-based tracking
	daemonSetMap                   map[string]*appsv1.DaemonSet
	daemonSetStartTime             map[string]time.Time // Track when each rollout started
	daemonSetControllerRevisionMap map[string]*ControllerRevisionInfo
	daemonSetMutex                 sync.RWMutex
}

// NewRolloutTracker creates a new RolloutTracker instance
func NewRolloutTracker() *RolloutTracker {
	return &RolloutTracker{
		// Deployment maps
		deploymentMap:       make(map[string]*appsv1.Deployment),
		deploymentStartTime: make(map[string]time.Time),
		replicaSetMap:       make(map[string]*ReplicaSetInfo),
		lastSeenRevision:    make(map[string]string),

		// StatefulSet maps
		statefulSetMap:         make(map[string]*appsv1.StatefulSet),
		statefulSetStartTime:   make(map[string]time.Time),
		controllerRevisionMap:  make(map[string]*ControllerRevisionInfo),
		lastSeenUpdateRevision: make(map[string]string),

		// DaemonSet maps
		daemonSetMap:                   make(map[string]*appsv1.DaemonSet),
		daemonSetStartTime:             make(map[string]time.Time),
		daemonSetControllerRevisionMap: make(map[string]*ControllerRevisionInfo),
	}
}

// StoreReplicaSet stores a ReplicaSet for deployment rollout tracking
func (rt *RolloutTracker) StoreReplicaSet(rs *appsv1.ReplicaSet, ownerName, ownerUID string) {
	rt.deploymentMutex.Lock()
	defer rt.deploymentMutex.Unlock()

	key := rs.Namespace + "/" + rs.Name
	rt.replicaSetMap[key] = &ReplicaSetInfo{
		Name:         rs.Name,
		Namespace:    rs.Namespace,
		CreationTime: rs.CreationTimestamp.Time,
		OwnerUID:     ownerUID,
		OwnerName:    ownerName,
	}

}

// GetRolloutDuration calculates rollout duration using stored start time.
// This method uses the stored start time which is set when a new rollout is detected
// (based on revision changes). It does NOT use ReplicaSet creation time because
// during rollbacks, Kubernetes reuses existing ReplicaSets which would give incorrect durations.
func (rt *RolloutTracker) GetRolloutDuration(namespace, deploymentName string) float64 {
	rt.deploymentMutex.RLock()
	defer rt.deploymentMutex.RUnlock()

	deploymentKey := namespace + "/" + deploymentName

	// Use stored start time - set when deployment revision changes (new rollout or rollback)
	deploymentStartTime, hasStartTime := rt.deploymentStartTime[deploymentKey]
	if !hasStartTime || deploymentStartTime.IsZero() {
		log.Debugf("[RolloutTracker] Deployment %s: no start time found, returning 0 duration", deploymentKey)
		return 0
	}

	duration := time.Since(deploymentStartTime)
	log.Debugf("[RolloutTracker] Deployment %s: duration=%.2fs (startTime=%v)", deploymentKey, duration.Seconds(), deploymentStartTime)
	return duration.Seconds()
}

// StoreDeployment stores a deployment for rollout tracking.
// Start time is reset when the revision annotation changes (actual rollout or rollback),
// NOT when generation changes (which also happens for scaling, pause, etc.).
func (rt *RolloutTracker) StoreDeployment(dep *appsv1.Deployment) {
	rt.deploymentMutex.Lock()
	defer rt.deploymentMutex.Unlock()

	key := dep.Namespace + "/" + dep.Name
	currentRevision := dep.Annotations[RevisionAnnotationKey]
	storedRevision := rt.lastSeenRevision[key]

	_, exists := rt.deploymentMap[key]
	if !exists {
		// New deployment being tracked - set start time
		// Try to use Progressing condition timestamp for restart resilience
		startTime := getProgressingConditionTime(dep, time.Now())
		rt.deploymentStartTime[key] = startTime
		log.Infof("[RolloutTracker] Deployment %s: NEW tracking started, revision=%s, gen=%d, observedGen=%d, startTime=%v",
			key, currentRevision, dep.Generation, dep.Status.ObservedGeneration, startTime)
	} else if storedRevision != currentRevision {
		// Revision changed - this is a new rollout or rollback
		rt.deploymentStartTime[key] = time.Now()
		log.Infof("[RolloutTracker] Deployment %s: REVISION CHANGED from %s to %s (rollout/rollback detected), gen=%d, observedGen=%d, startTime reset to now",
			key, storedRevision, currentRevision, dep.Generation, dep.Status.ObservedGeneration)
	} else {
		log.Debugf("[RolloutTracker] Deployment %s: continuing rollout, revision=%s unchanged, gen=%d, observedGen=%d",
			key, currentRevision, dep.Generation, dep.Status.ObservedGeneration)
	}
	// Note: Generation-only changes (scaling, pause) don't reset start time

	rt.deploymentMap[key] = dep.DeepCopy()
	// Always update the stored revision for active tracking
	rt.lastSeenRevision[key] = currentRevision
}

// getProgressingConditionTime extracts the LastTransitionTime from the Progressing condition
// when it indicates an active rollout. This provides restart resilience.
func getProgressingConditionTime(dep *appsv1.Deployment, fallback time.Time) time.Time {
	for _, cond := range dep.Status.Conditions {
		if cond.Type == appsv1.DeploymentProgressing {
			if cond.Status == corev1.ConditionTrue && cond.Reason == "ReplicaSetUpdated" {
				if !cond.LastTransitionTime.IsZero() {
					return cond.LastTransitionTime.Time
				}
			}
		}
	}
	return fallback
}

// CleanupDeployment removes a deployment from active rollout tracking.
// The lastSeenRevision is preserved to distinguish future rollouts from scaling operations.
func (rt *RolloutTracker) CleanupDeployment(namespace, name string) {
	rt.deploymentMutex.Lock()
	defer rt.deploymentMutex.Unlock()

	key := namespace + "/" + name
	lastRev := rt.lastSeenRevision[key]

	// Remove from active tracking
	delete(rt.deploymentMap, key)
	delete(rt.deploymentStartTime, key)

	// Remove associated ReplicaSets
	for rsKey, rsInfo := range rt.replicaSetMap {
		if rsInfo.Namespace == namespace && rsInfo.OwnerName == name {
			delete(rt.replicaSetMap, rsKey)
		}
	}

	log.Infof("[RolloutTracker] Deployment %s: CLEANUP (rollout complete), lastSeenRevision=%s preserved for future detection", key, lastRev)

	// NOTE: Do NOT delete rt.lastSeenRevision[key]
	// This is preserved to detect actual rollouts vs scaling after cleanup
}

// CleanupReplicaSet removes a deleted ReplicaSet from tracking
func (rt *RolloutTracker) CleanupReplicaSet(namespace, name string) {
	rt.deploymentMutex.Lock()
	defer rt.deploymentMutex.Unlock()

	key := namespace + "/" + name

	delete(rt.replicaSetMap, key)
}

// HasActiveRollout checks if we're actively tracking a rollout for the given deployment
func (rt *RolloutTracker) HasActiveRollout(d *appsv1.Deployment) bool {
	key := d.Namespace + "/" + d.Name
	rt.deploymentMutex.RLock()
	defer rt.deploymentMutex.RUnlock()

	_, exists := rt.deploymentMap[key]
	return exists
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

// HasRevisionChanged checks if the deployment's revision annotation has changed
// compared to the last seen value. This distinguishes actual rollouts from
// scaling/pause operations which don't change the revision.
func (rt *RolloutTracker) HasRevisionChanged(namespace, name, currentRevision string) bool {
	rt.deploymentMutex.RLock()
	defer rt.deploymentMutex.RUnlock()

	key := namespace + "/" + name
	lastRevision, seen := rt.lastSeenRevision[key]

	// If never seen, it's a new deployment - consider it a new rollout
	if !seen {
		log.Debugf("[RolloutTracker] Deployment %s: revision never seen before (current=%s), treating as changed", key, currentRevision)
		return true
	}

	changed := lastRevision != currentRevision
	if changed {
		log.Debugf("[RolloutTracker] Deployment %s: revision CHANGED from %s to %s", key, lastRevision, currentRevision)
	} else {
		log.Debugf("[RolloutTracker] Deployment %s: revision unchanged (%s)", key, currentRevision)
	}
	return changed
}

// UpdateLastSeenRevision updates the last seen revision for a deployment.
// This should be called after processing a deployment to track its revision.
func (rt *RolloutTracker) UpdateLastSeenRevision(namespace, name, revision string) {
	rt.deploymentMutex.Lock()
	defer rt.deploymentMutex.Unlock()

	key := namespace + "/" + name
	rt.lastSeenRevision[key] = revision
}

// StoreControllerRevision stores a ControllerRevision for StatefulSet rollout tracking
func (rt *RolloutTracker) StoreControllerRevision(cr *appsv1.ControllerRevision, ownerName, ownerUID string) {
	rt.statefulSetMutex.Lock()
	defer rt.statefulSetMutex.Unlock()

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

// GetStatefulSetRolloutDuration calculates StatefulSet rollout duration using stored start time.
// This method uses the stored start time which is set when a new rollout is detected
// (based on updateRevision changes). It does NOT use ControllerRevision creation time because
// during rollbacks, Kubernetes reuses existing ControllerRevisions which would give incorrect durations.
func (rt *RolloutTracker) GetStatefulSetRolloutDuration(namespace, statefulSetName string) float64 {
	rt.statefulSetMutex.RLock()
	defer rt.statefulSetMutex.RUnlock()

	statefulSetKey := namespace + "/" + statefulSetName

	// Use stored start time - set when updateRevision changes (new rollout or rollback)
	statefulSetStartTime, hasStartTime := rt.statefulSetStartTime[statefulSetKey]
	if !hasStartTime || statefulSetStartTime.IsZero() {
		log.Debugf("[RolloutTracker] StatefulSet %s: no start time found, returning 0 duration", statefulSetKey)
		return 0
	}

	duration := time.Since(statefulSetStartTime)
	log.Debugf("[RolloutTracker] StatefulSet %s: duration=%.2fs (startTime=%v)", statefulSetKey, duration.Seconds(), statefulSetStartTime)
	return duration.Seconds()
}

// StoreStatefulSet stores a StatefulSet for rollout tracking.
// Start time is reset when the updateRevision changes (actual rollout or rollback),
// NOT when generation changes (which also happens for scaling, partition changes, etc.).
func (rt *RolloutTracker) StoreStatefulSet(sts *appsv1.StatefulSet) {
	rt.statefulSetMutex.Lock()
	defer rt.statefulSetMutex.Unlock()

	key := sts.Namespace + "/" + sts.Name
	currentUpdateRevision := sts.Status.UpdateRevision
	storedRevision := rt.lastSeenUpdateRevision[key]

	_, exists := rt.statefulSetMap[key]
	if !exists {
		// New StatefulSet being tracked - set start time
		startTime := time.Now()
		rt.statefulSetStartTime[key] = startTime
		log.Infof("[RolloutTracker] StatefulSet %s: NEW tracking started, updateRevision=%s, currentRevision=%s, gen=%d, observedGen=%d, startTime=%v",
			key, currentUpdateRevision, sts.Status.CurrentRevision, sts.Generation, sts.Status.ObservedGeneration, startTime)
	} else if storedRevision != currentUpdateRevision {
		// UpdateRevision changed - this is a new rollout or rollback
		rt.statefulSetStartTime[key] = time.Now()
		log.Infof("[RolloutTracker] StatefulSet %s: UPDATE_REVISION CHANGED from %s to %s (rollout/rollback detected), gen=%d, observedGen=%d, startTime reset to now",
			key, storedRevision, currentUpdateRevision, sts.Generation, sts.Status.ObservedGeneration)
	} else {
		log.Debugf("[RolloutTracker] StatefulSet %s: continuing rollout, updateRevision=%s unchanged, gen=%d, observedGen=%d",
			key, currentUpdateRevision, sts.Generation, sts.Status.ObservedGeneration)
	}
	// Note: Generation-only changes (scaling, partition changes) don't reset start time

	rt.statefulSetMap[key] = sts.DeepCopy()
	// Always update the stored revision for active tracking
	rt.lastSeenUpdateRevision[key] = currentUpdateRevision
}

// CleanupStatefulSet removes a StatefulSet from active rollout tracking.
// The lastSeenUpdateRevision is preserved to distinguish future rollouts from scaling operations.
func (rt *RolloutTracker) CleanupStatefulSet(namespace, name string) {
	rt.statefulSetMutex.Lock()
	defer rt.statefulSetMutex.Unlock()

	key := namespace + "/" + name
	lastRev := rt.lastSeenUpdateRevision[key]

	// Remove from active tracking
	delete(rt.statefulSetMap, key)
	delete(rt.statefulSetStartTime, key)

	// Remove associated ControllerRevisions
	for crKey, crInfo := range rt.controllerRevisionMap {
		if crInfo.Namespace == namespace && crInfo.OwnerName == name {
			delete(rt.controllerRevisionMap, crKey)
		}
	}

	log.Infof("[RolloutTracker] StatefulSet %s: CLEANUP (rollout complete), lastSeenUpdateRevision=%s preserved for future detection", key, lastRev)

	// NOTE: Do NOT delete rt.lastSeenUpdateRevision[key]
	// This is preserved to detect actual rollouts vs scaling after cleanup
}

// CleanupControllerRevision removes a deleted ControllerRevision from tracking
func (rt *RolloutTracker) CleanupControllerRevision(namespace, name string) {
	rt.statefulSetMutex.Lock()
	defer rt.statefulSetMutex.Unlock()

	key := namespace + "/" + name
	delete(rt.controllerRevisionMap, key)
}

// HasActiveStatefulSetRollout checks if we're actively tracking a rollout for the given StatefulSet
func (rt *RolloutTracker) HasActiveStatefulSetRollout(sts *appsv1.StatefulSet) bool {
	key := sts.Namespace + "/" + sts.Name
	rt.statefulSetMutex.RLock()
	defer rt.statefulSetMutex.RUnlock()

	_, exists := rt.statefulSetMap[key]
	return exists
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

// HasStatefulSetRevisionChanged checks if the StatefulSet's updateRevision has changed
// compared to the last seen value. This distinguishes actual rollouts from
// scaling/partition operations which don't change the updateRevision.
func (rt *RolloutTracker) HasStatefulSetRevisionChanged(namespace, name, currentUpdateRevision string) bool {
	rt.statefulSetMutex.RLock()
	defer rt.statefulSetMutex.RUnlock()

	key := namespace + "/" + name
	lastRevision, seen := rt.lastSeenUpdateRevision[key]

	// If never seen, it's a new StatefulSet - consider it a new rollout
	if !seen {
		log.Debugf("[RolloutTracker] StatefulSet %s: updateRevision never seen before (current=%s), treating as changed", key, currentUpdateRevision)
		return true
	}

	changed := lastRevision != currentUpdateRevision
	if changed {
		log.Debugf("[RolloutTracker] StatefulSet %s: updateRevision CHANGED from %s to %s", key, lastRevision, currentUpdateRevision)
	} else {
		log.Debugf("[RolloutTracker] StatefulSet %s: updateRevision unchanged (%s)", key, currentUpdateRevision)
	}
	return changed
}

// UpdateLastSeenStatefulSetRevision updates the last seen updateRevision for a StatefulSet.
// This should be called after processing a StatefulSet to track its revision.
func (rt *RolloutTracker) UpdateLastSeenStatefulSetRevision(namespace, name, updateRevision string) {
	rt.statefulSetMutex.Lock()
	defer rt.statefulSetMutex.Unlock()

	key := namespace + "/" + name
	rt.lastSeenUpdateRevision[key] = updateRevision
}

// StoreDaemonSet stores a DaemonSet for rollout tracking.
// Note: DaemonSets don't expose UpdateRevision in their status like StatefulSets do,
// so we use generation-based tracking. Start time is reset when generation changes.
func (rt *RolloutTracker) StoreDaemonSet(ds *appsv1.DaemonSet) {
	rt.daemonSetMutex.Lock()
	defer rt.daemonSetMutex.Unlock()

	key := ds.Namespace + "/" + ds.Name

	// Check if this is a new rollout (new DaemonSet OR generation changed)
	existingDs, exists := rt.daemonSetMap[key]
	if !exists {
		// New DaemonSet being tracked - set start time
		startTime := time.Now()
		rt.daemonSetStartTime[key] = startTime
		log.Infof("[RolloutTracker] DaemonSet %s: NEW tracking started, gen=%d, observedGen=%d, startTime=%v",
			key, ds.Generation, ds.Status.ObservedGeneration, startTime)
	} else if existingDs.Generation != ds.Generation {
		// Generation changed - this is a new rollout
		rt.daemonSetStartTime[key] = time.Now()
		log.Infof("[RolloutTracker] DaemonSet %s: GENERATION CHANGED from %d to %d (rollout detected), startTime reset to now",
			key, existingDs.Generation, ds.Generation)
	} else {
		log.Debugf("[RolloutTracker] DaemonSet %s: continuing rollout, gen=%d unchanged, observedGen=%d",
			key, ds.Generation, ds.Status.ObservedGeneration)
	}

	rt.daemonSetMap[key] = ds.DeepCopy()
}

// StoreDaemonSetControllerRevision stores a ControllerRevision for DaemonSet rollout tracking
func (rt *RolloutTracker) StoreDaemonSetControllerRevision(cr *appsv1.ControllerRevision, ownerName, ownerUID string) {
	rt.daemonSetMutex.Lock()
	defer rt.daemonSetMutex.Unlock()

	key := cr.Namespace + "/" + cr.Name
	rt.daemonSetControllerRevisionMap[key] = &ControllerRevisionInfo{
		Name:         cr.Name,
		Namespace:    cr.Namespace,
		CreationTime: cr.CreationTimestamp.Time,
		Revision:     cr.Revision,
		OwnerUID:     ownerUID,
		OwnerName:    ownerName,
	}

}

// GetDaemonSetRolloutDuration calculates DaemonSet rollout duration using stored start time.
// This method uses the stored start time which is set when a new rollout is detected
// (based on generation changes for DaemonSets). It does NOT use ControllerRevision creation time because
// during rollbacks, Kubernetes reuses existing ControllerRevisions which would give incorrect durations.
func (rt *RolloutTracker) GetDaemonSetRolloutDuration(namespace, daemonSetName string) float64 {
	rt.daemonSetMutex.RLock()
	defer rt.daemonSetMutex.RUnlock()

	daemonSetKey := namespace + "/" + daemonSetName

	// Use stored start time - set when generation changes (new rollout)
	daemonSetStartTime, hasStartTime := rt.daemonSetStartTime[daemonSetKey]
	if !hasStartTime || daemonSetStartTime.IsZero() {
		log.Debugf("[RolloutTracker] DaemonSet %s: no start time found, returning 0 duration", daemonSetKey)
		return 0
	}

	duration := time.Since(daemonSetStartTime)
	log.Debugf("[RolloutTracker] DaemonSet %s: duration=%.2fs (startTime=%v)", daemonSetKey, duration.Seconds(), daemonSetStartTime)
	return duration.Seconds()
}

// CleanupDaemonSet removes a DaemonSet from active rollout tracking.
func (rt *RolloutTracker) CleanupDaemonSet(namespace, name string) {
	rt.daemonSetMutex.Lock()
	defer rt.daemonSetMutex.Unlock()

	key := namespace + "/" + name
	// Remove from active tracking
	delete(rt.daemonSetMap, key)
	delete(rt.daemonSetStartTime, key)

	// Remove associated ControllerRevisions
	for crKey, crInfo := range rt.daemonSetControllerRevisionMap {
		if crInfo.Namespace == namespace && crInfo.OwnerName == name {
			delete(rt.daemonSetControllerRevisionMap, crKey)
		}
	}

	log.Infof("[RolloutTracker] DaemonSet %s: CLEANUP (rollout complete)", key)
}

// CleanupDaemonSetControllerRevision removes a deleted ControllerRevision from tracking
func (rt *RolloutTracker) CleanupDaemonSetControllerRevision(namespace, name string) {
	rt.daemonSetMutex.Lock()
	defer rt.daemonSetMutex.Unlock()

	key := namespace + "/" + name
	delete(rt.daemonSetControllerRevisionMap, key)
}

// HasActiveDaemonSetRollout checks if we're actively tracking a rollout for the given DaemonSet
func (rt *RolloutTracker) HasActiveDaemonSetRollout(ds *appsv1.DaemonSet) bool {
	key := ds.Namespace + "/" + ds.Name
	rt.daemonSetMutex.RLock()
	defer rt.daemonSetMutex.RUnlock()

	_, exists := rt.daemonSetMap[key]
	return exists
}

// HasDaemonSetRolloutCondition checks if Kubernetes reports the DaemonSet as updating
func (rt *RolloutTracker) HasDaemonSetRolloutCondition(ds *appsv1.DaemonSet) bool {
	desiredPods := ds.Status.DesiredNumberScheduled

	// If there are no desired pods, nothing to roll out
	if desiredPods == 0 {
		return false
	}

	// Check if update is in progress (applies to both OnDelete and RollingUpdate)
	// - OnDelete: UpdatedNumberScheduled < desiredPods means pods haven't been manually deleted yet
	// - RollingUpdate: UpdatedNumberScheduled < desiredPods means Kubernetes is actively updating
	if ds.Status.UpdatedNumberScheduled < desiredPods {
		return true
	}

	// All pods are on the new revision, but check if they're all available
	// NumberAvailable: pods that are ready and available for at least minReadySeconds
	if ds.Status.NumberAvailable < desiredPods {
		return true
	}

	// Check if there are unavailable pods
	if ds.Status.NumberUnavailable > 0 {
		return true
	}

	return false
}

