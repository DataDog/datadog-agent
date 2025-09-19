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

	// Try to use the newest ReplicaSet creation time, fall back to deployment start time
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

// CleanupDeployment removes a deployment and its ReplicaSets from tracking
func CleanupDeployment(namespace, name string) {
	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	key := namespace + "/" + name

	// Remove deployment
	delete(deploymentMap, key)
	delete(deploymentAccessTime, key) // Also remove access time tracking
	delete(deploymentStartTime, key)  // Also remove rollout start time

	// Remove associated ReplicaSets
	for rsKey, rsInfo := range replicaSetMap {
		if rsInfo.Namespace == namespace && rsInfo.OwnerName == name {
			delete(replicaSetMap, rsKey)
		}
	}
}

// CleanupDeletedReplicaSet removes a deleted ReplicaSet from tracking
func CleanupDeletedReplicaSet(namespace, name string) {

	rolloutMutex.Lock()
	defer rolloutMutex.Unlock()

	key := namespace + "/" + name

	delete(replicaSetMap, key)

}
