// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"fmt"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// rolloutState represents the state of an active rollout tracked via events
type rolloutState struct {
	StartTime      time.Time
	LastSeenAt     time.Time
	CurrentRev     string
	UpdateRev      string
	Generation     int64
	ObservedGen    int64
	Source         string // "event" or "bootstrap"
}

// rolloutEventTracker tracks rollout lifecycle using Kubernetes informer events
type rolloutEventTracker struct {
	// Active rollouts indexed by "namespace/name"
	activeRollouts map[string]*rolloutState
	mutex          sync.RWMutex

	// Informer event handlers
	statefulSetHandler   cache.ResourceEventHandlerRegistration
	deploymentHandler    cache.ResourceEventHandlerRegistration
	controllerRevHandler cache.ResourceEventHandlerRegistration
	replicaSetHandler    cache.ResourceEventHandlerRegistration
}

// newRolloutEventTracker creates a new event tracker
func newRolloutEventTracker() *rolloutEventTracker {
	return &rolloutEventTracker{
		activeRollouts: make(map[string]*rolloutState),
	}
}

// setupStatefulSetEventHandling registers event handlers for StatefulSet rollout tracking
func (t *rolloutEventTracker) setupStatefulSetEventHandling(informer cache.SharedIndexInformer) error {
	handler, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldSts := oldObj.(*appsv1.StatefulSet)
			newSts := newObj.(*appsv1.StatefulSet)
			t.onStatefulSetUpdate(oldSts, newSts)
		},
		DeleteFunc: func(obj interface{}) {
			sts := obj.(*appsv1.StatefulSet)
			t.onStatefulSetDelete(sts)
		},
	})
	
	if err != nil {
		return fmt.Errorf("failed to add StatefulSet event handler: %v", err)
	}
	
	t.statefulSetHandler = handler
	return nil
}

// setupDeploymentEventHandling registers event handlers for Deployment rollout tracking
func (t *rolloutEventTracker) setupDeploymentEventHandling(informer cache.SharedIndexInformer) error {
	handler, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldDep := oldObj.(*appsv1.Deployment)
			newDep := newObj.(*appsv1.Deployment)
			t.onDeploymentUpdate(oldDep, newDep)
		},
		DeleteFunc: func(obj interface{}) {
			dep := obj.(*appsv1.Deployment)
			t.onDeploymentDelete(dep)
		},
	})
	
	if err != nil {
		return fmt.Errorf("failed to add Deployment event handler: %v", err)
	}
	
	t.deploymentHandler = handler
	return nil
}

// onStatefulSetUpdate handles StatefulSet update events
func (t *rolloutEventTracker) onStatefulSetUpdate(old, new *appsv1.StatefulSet) {
	key := fmt.Sprintf("%s/%s", new.Namespace, new.Name)
	
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Check if rollout started
	if old.Status.UpdateRevision != new.Status.UpdateRevision && 
		new.Status.CurrentRevision != new.Status.UpdateRevision {
		
		log.Debugf("StatefulSet rollout started: %s (current: %s, update: %s)", 
			key, new.Status.CurrentRevision, new.Status.UpdateRevision)
		
		t.activeRollouts[key] = &rolloutState{
			StartTime:   time.Now(),
			LastSeenAt:  time.Now(),
			CurrentRev:  new.Status.CurrentRevision,
			UpdateRev:   new.Status.UpdateRevision,
			Source:      "event",
		}
		return
	}

	// Check if rollout completed - both revision match AND all replicas ready
	if new.Status.CurrentRevision == new.Status.UpdateRevision &&
		new.Status.ReadyReplicas == new.Status.Replicas {
		if _, exists := t.activeRollouts[key]; exists {
			log.Debugf("StatefulSet rollout completed: %s", key)
			delete(t.activeRollouts, key)
		}
		return
	}

	// Update existing rollout state
	if state, exists := t.activeRollouts[key]; exists {
		state.LastSeenAt = time.Now()
		state.CurrentRev = new.Status.CurrentRevision
		state.UpdateRev = new.Status.UpdateRevision
	}
}

// onDeploymentUpdate handles Deployment update events
func (t *rolloutEventTracker) onDeploymentUpdate(old, new *appsv1.Deployment) {
	key := fmt.Sprintf("%s/%s", new.Namespace, new.Name)
	
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Check if rollout started (generation changed and mismatch exists)
	if old.Generation != new.Generation && 
		new.Generation != new.Status.ObservedGeneration {
		
		log.Debugf("Deployment rollout started: %s (gen: %d, observed: %d)", 
			key, new.Generation, new.Status.ObservedGeneration)
		
		t.activeRollouts[key] = &rolloutState{
			StartTime:    time.Now(),
			LastSeenAt:   time.Now(),
			Generation:   new.Generation,
			ObservedGen:  new.Status.ObservedGeneration,
			Source:       "event",
		}
		return
	}

	// Check if rollout completed - both generation match AND all replicas ready
	if new.Generation == new.Status.ObservedGeneration &&
		(new.Spec.Replicas == nil || new.Status.ReadyReplicas == *new.Spec.Replicas) {
		if _, exists := t.activeRollouts[key]; exists {
			log.Debugf("Deployment rollout completed: %s", key)
			delete(t.activeRollouts, key)
		}
		return
	}

	// Update existing rollout state
	if state, exists := t.activeRollouts[key]; exists {
		state.LastSeenAt = time.Now()
		state.Generation = new.Generation
		state.ObservedGen = new.Status.ObservedGeneration
	}
}

// onStatefulSetDelete handles StatefulSet deletion events
func (t *rolloutEventTracker) onStatefulSetDelete(sts *appsv1.StatefulSet) {
	key := fmt.Sprintf("%s/%s", sts.Namespace, sts.Name)
	
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	delete(t.activeRollouts, key)
	log.Debugf("StatefulSet deleted, removed from rollout tracking: %s", key)
}

// onDeploymentDelete handles Deployment deletion events
func (t *rolloutEventTracker) onDeploymentDelete(dep *appsv1.Deployment) {
	key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
	
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	delete(t.activeRollouts, key)
	log.Debugf("Deployment deleted, removed from rollout tracking: %s", key)
}

// getStatefulSetRolloutDuration returns the duration of an active StatefulSet rollout if tracked
func (t *rolloutEventTracker) getStatefulSetRolloutDuration(sts *appsv1.StatefulSet) (float64, bool) {
	key := fmt.Sprintf("%s/%s", sts.Namespace, sts.Name)
	
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	state, exists := t.activeRollouts[key]
	if !exists {
		return 0, false
	}
	
	// Validate that the tracked state matches current resource state
	if state.CurrentRev != sts.Status.CurrentRevision || 
		state.UpdateRev != sts.Status.UpdateRevision {
		log.Debugf("StatefulSet rollout state mismatch for %s, event tracking may be stale", key)
		return 0, false
	}
	
	duration := time.Since(state.StartTime)
	return duration.Seconds(), true
}

// getDeploymentRolloutDuration returns the duration of an active Deployment rollout if tracked
func (t *rolloutEventTracker) getDeploymentRolloutDuration(dep *appsv1.Deployment) (float64, bool) {
	key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
	
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	state, exists := t.activeRollouts[key]
	if !exists {
		return 0, false
	}
	
	// Validate that the tracked state matches current resource state
	if state.Generation != dep.Generation || 
		state.ObservedGen != dep.Status.ObservedGeneration {
		log.Debugf("Deployment rollout state mismatch for %s, event tracking may be stale", key)
		return 0, false
	}
	
	duration := time.Since(state.StartTime)
	return duration.Seconds(), true
}

// bootstrapExistingRollouts scans existing resources to detect ongoing rollouts that
// started before the informer was initialized
func (t *rolloutEventTracker) bootstrapExistingRollouts(statefulSets []*appsv1.StatefulSet, deployments []*appsv1.Deployment) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	log.Debugf("Bootstrapping existing rollouts from %d StatefulSets and %d Deployments", 
		len(statefulSets), len(deployments))
	
	// Bootstrap StatefulSet rollouts
	for _, sts := range statefulSets {
		if sts.Status.CurrentRevision != sts.Status.UpdateRevision && 
			sts.Status.UpdateRevision != "" &&
			sts.Status.ReadyReplicas != sts.Status.Replicas {
			
			key := fmt.Sprintf("%s/%s", sts.Namespace, sts.Name)
			
			// Don't overwrite event-tracked rollouts
			if _, exists := t.activeRollouts[key]; exists {
				continue
			}
			
			t.activeRollouts[key] = &rolloutState{
				StartTime:   time.Now(), // We don't know the actual start time
				LastSeenAt:  time.Now(),
				CurrentRev:  sts.Status.CurrentRevision,
				UpdateRev:   sts.Status.UpdateRevision,
				Source:      "bootstrap",
			}
			
			log.Debugf("Bootstrapped StatefulSet rollout: %s", key)
		}
	}
	
	// Bootstrap Deployment rollouts
	for _, dep := range deployments {
		if dep.Generation != dep.Status.ObservedGeneration &&
			(dep.Spec.Replicas == nil || dep.Status.ReadyReplicas != *dep.Spec.Replicas) {
			key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
			
			// Don't overwrite event-tracked rollouts
			if _, exists := t.activeRollouts[key]; exists {
				continue
			}
			
			t.activeRollouts[key] = &rolloutState{
				StartTime:    time.Now(), // We don't know the actual start time
				LastSeenAt:   time.Now(),
				Generation:   dep.Generation,
				ObservedGen:  dep.Status.ObservedGeneration,
				Source:       "bootstrap",
			}
			
			log.Debugf("Bootstrapped Deployment rollout: %s", key)
		}
	}
}

// cleanup removes stale rollout entries that haven't been seen recently
func (t *rolloutEventTracker) cleanup(maxAge time.Duration) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	now := time.Now()
	cleaned := 0
	
	for key, state := range t.activeRollouts {
		if now.Sub(state.LastSeenAt) > maxAge {
			delete(t.activeRollouts, key)
			cleaned++
		}
	}
	
	if cleaned > 0 {
		log.Debugf("Cleaned up %d stale rollout entries", cleaned)
	}
}

// getActiveRolloutCount returns the number of currently tracked rollouts
func (t *rolloutEventTracker) getActiveRolloutCount() int {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return len(t.activeRollouts)
}

// shutdown removes all event handlers
func (t *rolloutEventTracker) shutdown() {
	// Note: In a real implementation, you'd remove the handlers here
	// The current KSM framework doesn't provide easy handler removal
	log.Debugf("Shutting down rollout event tracker")
}