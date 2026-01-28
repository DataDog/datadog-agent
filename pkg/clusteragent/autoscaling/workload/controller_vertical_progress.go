// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"sync"
	"time"
)

const (
	// rolloutStallTimeout is the duration after which a rollout is considered stalled if no pods have moved
	// TODO: We should probably read the Rollout strategy to know what's the acceptable limit.
	rolloutStallTimeout = 60 * time.Minute
)

// rolloutProgressEntry tracks the progress of a rollout for stall detection
type rolloutProgressEntry struct {
	recommendationID string
	podCount         int32
	lastMovementTime time.Time
}

// rolloutProgressTracker tracks rollout progress per autoscaler for stall detection.
// It monitors pod counts to detect when a rollout has stalled (no pod movement for too long).
type rolloutProgressTracker struct {
	mu      sync.Mutex
	entries map[string]rolloutProgressEntry
}

// newRolloutProgressTracker creates a new rolloutProgressTracker
func newRolloutProgressTracker() *rolloutProgressTracker {
	return &rolloutProgressTracker{
		entries: make(map[string]rolloutProgressEntry),
	}
}

// Update updates the progress for an autoscaler and returns whether the rollout is stalled.
// A rollout is considered stalled if the pod count hasn't increased for rolloutStallTimeout.
// This should be called during sync when pod counts are fetched.
func (r *rolloutProgressTracker) Update(autoscalerID, recommendationID string, podCount int32, currentTime time.Time) (stalled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[autoscalerID]
	if !exists || entry.recommendationID != recommendationID {
		// First time tracking this recommendation or new recommendation
		r.entries[autoscalerID] = rolloutProgressEntry{
			recommendationID: recommendationID,
			podCount:         podCount,
			lastMovementTime: currentTime,
		}
		return false
	}

	// Check if there's been progress (pod count increased)
	if podCount > entry.podCount {
		entry.podCount = podCount
		entry.lastMovementTime = currentTime
		r.entries[autoscalerID] = entry
		return false
	}

	// Check if we've been waiting too long without progress
	return currentTime.Sub(entry.lastMovementTime) >= rolloutStallTimeout
}

// Clear removes the progress entry for an autoscaler when rollout is complete
func (r *rolloutProgressTracker) Clear(autoscalerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, autoscalerID)
}
