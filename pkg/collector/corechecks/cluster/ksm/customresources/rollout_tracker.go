// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ReplicaSetInfo holds information about a ReplicaSet for rollout tracking
type ReplicaSetInfo struct {
	Name         string
	Namespace    string
	CreationTime time.Time
	OwnerUID     string // UID of owning Deployment
	OwnerName    string // Name of owning Deployment
}

// Global maps for tracking rollouts - accessed by both factories and transformers
var (
	deploymentMap        map[string]*appsv1.Deployment
	deploymentAccessTime map[string]time.Time // Track when each deployment was last accessed
	deploymentStartTime  map[string]time.Time // Track when each rollout started
	replicaSetMap        map[string]*ReplicaSetInfo
	rolloutMutex         sync.RWMutex
)

func init() {
	deploymentMap = make(map[string]*appsv1.Deployment)
	deploymentAccessTime = make(map[string]time.Time)
	deploymentStartTime = make(map[string]time.Time)
	replicaSetMap = make(map[string]*ReplicaSetInfo)
}

// StoreReplicaSet stores a ReplicaSet for deployment rollout tracking
func StoreReplicaSet(rs *appsv1.ReplicaSet, ownerName, ownerUID string) {
	log.Infof("ROLLOUT-TEST: StoreReplicaSet called for RS %s/%s owned by deployment %s (UID: %s), created at %s",
		rs.Namespace, rs.Name, ownerName, ownerUID, rs.CreationTimestamp.Time.Format(time.RFC3339))

	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	key := rs.Namespace + "/" + rs.Name
	replicaSetMap[key] = &ReplicaSetInfo{
		Name:         rs.Name,
		Namespace:    rs.Namespace,
		CreationTime: rs.CreationTimestamp.Time,
		OwnerUID:     ownerUID,
		OwnerName:    ownerName,
	}

	log.Infof("ROLLOUT-TEST: ReplicaSet stored. Total ReplicaSets in map: %d", len(replicaSetMap))
}

// GetDeploymentRolloutDurationFromMaps calculates rollout duration using stored maps (used by transformers)
func GetDeploymentRolloutDurationFromMaps(namespace, deploymentName string) float64 {
	log.Infof("ROLLOUT-TEST: GetDeploymentRolloutDurationFromMaps called for deployment %s/%s", namespace, deploymentName)

	rolloutMutex.RLock()
	defer rolloutMutex.RUnlock()

	deploymentKey := namespace + "/" + deploymentName

	// Update access time for this deployment
	if _, exists := deploymentMap[deploymentKey]; exists {
		deploymentAccessTime[deploymentKey] = time.Now()
	}

	// Use deployment rollout start time instead of ReplicaSet creation time
	startTime, hasStartTime := deploymentStartTime[deploymentKey]
	if !hasStartTime || startTime.IsZero() {
		log.Infof("ROLLOUT-TEST: No rollout start time found for deployment %s/%s, returning 0 duration", namespace, deploymentName)
		return 0
	}

	duration := time.Since(startTime)
	log.Infof("ROLLOUT-TEST: Calculated rollout duration for deployment %s/%s: %.2f seconds (from rollout start at %s)",
		namespace, deploymentName, duration.Seconds(), startTime.Format(time.RFC3339))
	return duration.Seconds()
}

// StoreDeployment stores a deployment for rollout tracking
func StoreDeployment(dep *appsv1.Deployment) {
	log.Infof("ROLLOUT-TEST: StoreDeployment called for deployment %s/%s (generation: %d, observedGeneration: %d, replicas: %d, readyReplicas: %d)",
		dep.Namespace, dep.Name, dep.Generation, dep.Status.ObservedGeneration,
		getReplicasValue(dep.Spec.Replicas), dep.Status.ReadyReplicas)

	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	key := dep.Namespace + "/" + dep.Name

	// Check if this is a new rollout (new deployment OR generation changed)
	existingDep, exists := deploymentMap[key]
	if !exists {
		deploymentStartTime[key] = time.Now()
		log.Infof("ROLLOUT-TEST: New rollout detected for deployment %s/%s (generation: %d), setting start time to %s",
			dep.Namespace, dep.Name, dep.Generation, time.Now().Format(time.RFC3339))
	} else if existingDep.Generation != dep.Generation {
		oldGeneration := existingDep.Generation
		deploymentStartTime[key] = time.Now()
		log.Infof("ROLLOUT-TEST: Generation change detected for deployment %s/%s (generation: %d->%d), RESETTING rollout start time to %s",
			dep.Namespace, dep.Name, oldGeneration, dep.Generation, time.Now().Format(time.RFC3339))
	} else {
		log.Infof("ROLLOUT-TEST: Same generation for deployment %s/%s (generation: %d), keeping existing rollout start time",
			dep.Namespace, dep.Name, dep.Generation)
	}

	deploymentMap[key] = dep.DeepCopy()
	deploymentAccessTime[key] = time.Now() // Track when we stored it

	log.Infof("ROLLOUT-TEST: Deployment stored. Total deployments in map: %d", len(deploymentMap))

	// Run periodic cleanup occasionally (less frequent than transformer calls)
	go func() {
		PeriodicCleanup()
	}()
}

// Helper function to safely get replicas value
func getReplicasValue(replicas *int32) int32 {
	if replicas == nil {
		return 0
	}
	return *replicas
}

// CleanupCompletedDeployment removes completed deployment and its ReplicaSets from tracking
func CleanupCompletedDeployment(dep *appsv1.Deployment) {
	log.Infof("ROLLOUT-TEST: CleanupCompletedDeployment called for deployment %s/%s", dep.Namespace, dep.Name)

	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	key := dep.Namespace + "/" + dep.Name

	// Count associated ReplicaSets before cleanup
	var rsCount int
	for _, rsInfo := range replicaSetMap {
		if rsInfo.Namespace == dep.Namespace && rsInfo.OwnerName == dep.Name {
			rsCount++
		}
	}

	log.Infof("ROLLOUT-TEST: Found %d ReplicaSets to clean up for deployment %s/%s", rsCount, dep.Namespace, dep.Name)

	// Remove deployment
	_, deploymentExisted := deploymentMap[key]
	delete(deploymentMap, key)
	delete(deploymentAccessTime, key) // Also remove access time tracking
	delete(deploymentStartTime, key)  // Also remove rollout start time

	// Remove associated ReplicaSets
	var cleanedRS int
	for rsKey, rsInfo := range replicaSetMap {
		if rsInfo.Namespace == dep.Namespace && rsInfo.OwnerName == dep.Name {
			log.Infof("ROLLOUT-TEST: Cleaning up ReplicaSet %s for deployment %s/%s", rsKey, dep.Namespace, dep.Name)
			delete(replicaSetMap, rsKey)
			cleanedRS++
		}
	}

	log.Infof("ROLLOUT-TEST: Cleanup completed for deployment %s/%s - removed deployment: %v, cleaned %d ReplicaSets. Remaining: %d deployments, %d ReplicaSets",
		dep.Namespace, dep.Name, deploymentExisted, cleanedRS, len(deploymentMap), len(replicaSetMap))
}

// CleanupDeletedDeployment removes a deleted deployment and its ReplicaSets from tracking
func CleanupDeletedDeployment(namespace, name string) {
	log.Infof("ROLLOUT-TEST: CleanupDeletedDeployment called for deployment %s/%s", namespace, name)

	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	key := namespace + "/" + name

	// Count associated ReplicaSets before cleanup
	var rsCount int
	for _, rsInfo := range replicaSetMap {
		if rsInfo.Namespace == namespace && rsInfo.OwnerName == name {
			rsCount++
		}
	}

	log.Infof("ROLLOUT-TEST: Found %d ReplicaSets to clean up for deleted deployment %s/%s", rsCount, namespace, name)

	// Remove deployment
	_, deploymentExisted := deploymentMap[key]
	delete(deploymentMap, key)
	delete(deploymentAccessTime, key) // Also remove access time tracking
	delete(deploymentStartTime, key)  // Also remove rollout start time

	// Remove associated ReplicaSets
	var cleanedRS int
	for rsKey, rsInfo := range replicaSetMap {
		if rsInfo.Namespace == namespace && rsInfo.OwnerName == name {
			log.Infof("ROLLOUT-TEST: Cleaning up ReplicaSet %s for deleted deployment %s/%s", rsKey, namespace, name)
			delete(replicaSetMap, rsKey)
			cleanedRS++
		}
	}

	log.Infof("ROLLOUT-TEST: Cleanup completed for deleted deployment %s/%s - removed deployment: %v, cleaned %d ReplicaSets. Remaining: %d deployments, %d ReplicaSets",
		namespace, name, deploymentExisted, cleanedRS, len(deploymentMap), len(replicaSetMap))
}

// Constants for cleanup thresholds - TESTING VALUES
const (
	// Max time since we last stored/accessed a deployment (testing deleted deployments)
	maxDeploymentUnusedTime = 120 * time.Second
	// Max age for ReplicaSets (from their creation time) - longer to handle slow rollouts
	maxReplicaSetAge = 120 * time.Second
	// How often to run cleanup (testing interval)
	cleanupInterval = 30 * time.Second

	// Conservative
	// maxDeploymentUnusedTime = 10 * time.Minute  // 10 minutes - safe for slow rollouts
	// maxReplicaSetAge = 2 * time.Hour            // 2 hours - handles very slow rollouts
	// cleanupInterval = 5 * time.Minute           // 5 minutes - balanced cleanup frequency

	// Aggressive
	// maxDeploymentUnusedTime = 5 * time.Minute   // 5 minutes - assumes fast rollouts
	// maxReplicaSetAge = 1 * time.Hour            // 1 hour - most rollouts complete faster
	// cleanupInterval = 2 * time.Minute           // 2 minutes - more responsive cleanup
)

var (
	lastCleanupTime time.Time
	cleanupMutex    sync.Mutex
)

// PeriodicCleanup removes stale entries from the maps
// This should be called periodically during metric collection
func PeriodicCleanup() {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()

	now := time.Now()
	if now.Sub(lastCleanupTime) < cleanupInterval {
		return // Too soon since last cleanup
	}

	log.Infof("ROLLOUT-TEST: Starting periodic cleanup of stale rollout entries")

	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	initialDeployments := len(deploymentMap)
	initialReplicaSets := len(replicaSetMap)

	// Clean up deployments that haven't been accessed recently
	var staleDeployments []string
	for key, _ := range deploymentMap {
		lastAccess, hasAccess := deploymentAccessTime[key]
		if !hasAccess {
			// No access time recorded, this is an anomaly - clean it up
			log.Infof("ROLLOUT-TEST: Found deployment %s with no access time - cleaning up", key)
			staleDeployments = append(staleDeployments, key)
		} else if now.Sub(lastAccess) > maxDeploymentUnusedTime {
			log.Infof("ROLLOUT-TEST: Found stale deployment %s (unused for: %v, max: %v)",
				key, now.Sub(lastAccess), maxDeploymentUnusedTime)
			staleDeployments = append(staleDeployments, key)
		}
	}

	for _, key := range staleDeployments {
		delete(deploymentMap, key)
		delete(deploymentAccessTime, key)
		delete(deploymentStartTime, key)
	}

	// Clean up old ReplicaSets
	var staleReplicaSets []string
	for key, rsInfo := range replicaSetMap {
		if now.Sub(rsInfo.CreationTime) > maxReplicaSetAge {
			log.Infof("ROLLOUT-TEST: Found stale ReplicaSet %s (age: %v, max: %v)",
				key, now.Sub(rsInfo.CreationTime), maxReplicaSetAge)
			staleReplicaSets = append(staleReplicaSets, key)
		}
	}

	for _, key := range staleReplicaSets {
		delete(replicaSetMap, key)
	}

	// Clean up orphaned ReplicaSets (no corresponding deployment)
	var orphanedReplicaSets []string
	for rsKey, rsInfo := range replicaSetMap {
		deploymentKey := rsInfo.Namespace + "/" + rsInfo.OwnerName
		if _, exists := deploymentMap[deploymentKey]; !exists {
			log.Infof("ROLLOUT-TEST: Found orphaned ReplicaSet %s (no deployment %s found)",
				rsKey, deploymentKey)
			orphanedReplicaSets = append(orphanedReplicaSets, rsKey)
		}
	}

	for _, key := range orphanedReplicaSets {
		delete(replicaSetMap, key)
	}

	lastCleanupTime = now

	cleanedDeployments := len(staleDeployments)
	cleanedReplicaSets := len(staleReplicaSets) + len(orphanedReplicaSets)

	log.Infof("ROLLOUT-TEST: Periodic cleanup completed - removed %d stale deployments, %d stale/orphaned ReplicaSets. Maps: %d->%d deployments, %d->%d ReplicaSets",
		cleanedDeployments, cleanedReplicaSets,
		initialDeployments, len(deploymentMap),
		initialReplicaSets, len(replicaSetMap))
}

// GetMapStats returns current size of the tracking maps (for debugging)
func GetMapStats() (deployments int, replicaSets int) {
	rolloutMutex.RLock()
	defer rolloutMutex.RUnlock()
	return len(deploymentMap), len(replicaSetMap)
}

// PrintMapContents logs the current contents of tracking maps for visibility
func PrintMapContents() {
	rolloutMutex.RLock()
	defer rolloutMutex.RUnlock()

	log.Infof("ROLLOUT-TEST: === CURRENT MAP CONTENTS ===")
	log.Infof("ROLLOUT-TEST: Deployments in tracking map (%d):", len(deploymentMap))
	for key, dep := range deploymentMap {
		startTime := "unknown"
		if st, exists := deploymentStartTime[key]; exists {
			startTime = st.Format(time.RFC3339)
		}
		log.Infof("ROLLOUT-TEST:   - %s (gen: %d, observed: %d, replicas: %d/%d, started: %s)",
			key, dep.Generation, dep.Status.ObservedGeneration,
			dep.Status.ReadyReplicas, getReplicasValue(dep.Spec.Replicas), startTime)
	}

	log.Infof("ROLLOUT-TEST: ReplicaSets in tracking map (%d):", len(replicaSetMap))
	for key, rs := range replicaSetMap {
		log.Infof("ROLLOUT-TEST:   - %s (owner: %s, created: %s)",
			key, rs.OwnerName, rs.CreationTime.Format(time.RFC3339))
	}
	log.Infof("ROLLOUT-TEST: === END MAP CONTENTS ===")
}
