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

}

// GetDeploymentRolloutDurationFromMaps calculates rollout duration using stored maps (used by transformers)
func GetDeploymentRolloutDurationFromMaps(namespace, deploymentName string) float64 {

	rolloutMutex.RLock()
	defer rolloutMutex.RUnlock()

	deploymentKey := namespace + "/" + deploymentName

	// Update access time for this deployment
	if _, exists := deploymentMap[deploymentKey]; exists {
		deploymentAccessTime[deploymentKey] = time.Now()
	}

	// Hybrid approach: try to use the newest ReplicaSet creation time, fall back to deployment start time
	var startTime time.Time

	// First, look for the newest ReplicaSet owned by this deployment
	var newestRS *ReplicaSetInfo
	for _, rsInfo := range replicaSetMap {
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
		deploymentStartTime, hasStartTime := deploymentStartTime[deploymentKey]
		if !hasStartTime || deploymentStartTime.IsZero() {
			return 0
		}
		startTime = deploymentStartTime
	}

	duration := time.Since(startTime)
	return duration.Seconds()
}

// StoreDeployment stores a deployment for rollout tracking
func StoreDeployment(dep *appsv1.Deployment) {

	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	key := dep.Namespace + "/" + dep.Name

	// Check if this is a new rollout (new deployment OR generation changed)
	existingDep, exists := deploymentMap[key]
	if !exists {
		deploymentStartTime[key] = time.Now()
	} else if existingDep.Generation != dep.Generation {
		deploymentStartTime[key] = time.Now()
	}

	deploymentMap[key] = dep.DeepCopy()
	deploymentAccessTime[key] = time.Now() // Track when we stored it
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


	// Remove deployment
	_, deploymentExisted := deploymentMap[key]
	delete(deploymentMap, key)
	delete(deploymentAccessTime, key) // Also remove access time tracking
	delete(deploymentStartTime, key)  // Also remove rollout start time

	// Remove associated ReplicaSets
	var cleanedRS int
	for rsKey, rsInfo := range replicaSetMap {
		if rsInfo.Namespace == dep.Namespace && rsInfo.OwnerName == dep.Name {
			delete(replicaSetMap, rsKey)
			cleanedRS++
		}
	}

}

// CleanupDeletedDeployment removes a deleted deployment and its ReplicaSets from tracking
func CleanupDeletedDeployment(namespace, name string) {

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


	// Remove deployment
	_, deploymentExisted := deploymentMap[key]
	delete(deploymentMap, key)
	delete(deploymentAccessTime, key) // Also remove access time tracking
	delete(deploymentStartTime, key)  // Also remove rollout start time

	// Remove associated ReplicaSets
	var cleanedRS int
	for rsKey, rsInfo := range replicaSetMap {
		if rsInfo.Namespace == namespace && rsInfo.OwnerName == name {
			delete(replicaSetMap, rsKey)
			cleanedRS++
		}
	}

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


	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	initialDeployments := len(deploymentMap)
	initialReplicaSets := len(replicaSetMap)

	// Clean up deployments that haven't been accessed recently
	var staleDeployments []string
	for key := range deploymentMap {
		lastAccess, hasAccess := deploymentAccessTime[key]
		if !hasAccess {
			// No access time recorded, this is an anomaly - clean it up
			staleDeployments = append(staleDeployments, key)
		} else if now.Sub(lastAccess) > maxDeploymentUnusedTime {
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
			orphanedReplicaSets = append(orphanedReplicaSets, rsKey)
		}
	}

	for _, key := range orphanedReplicaSets {
		delete(replicaSetMap, key)
	}

	lastCleanupTime = now

	cleanedDeployments := len(staleDeployments)
	cleanedReplicaSets := len(staleReplicaSets) + len(orphanedReplicaSets)

}

// GetMapStats returns current size of the tracking maps (for debugging)
func GetMapStats() (deployments int, replicaSets int) {
	rolloutMutex.RLock()
	defer rolloutMutex.RUnlock()
	return len(deploymentMap), len(replicaSetMap)
}
