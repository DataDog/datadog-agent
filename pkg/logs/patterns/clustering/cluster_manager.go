// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
package clustering

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering/merging"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// PatternChangeType indicates what changed when adding a TokenList to the cluster manager
type PatternChangeType int

const (
	// PatternNoChange means the TokenList was added to an existing cluster without structural changes
	PatternNoChange PatternChangeType = iota
	// PatternNew means a brand new pattern was created (first time seeing this signature)
	PatternNew
	// PatternUpdated means an existing pattern's structure changed (more wildcards added)
	PatternUpdated
)

// ClusterManager manages the clustering of TokenLists using hash-based bucketing.
type ClusterManager struct {
	mu          sync.RWMutex
	hashBuckets map[uint64][]*Cluster
	nextID      uint64

	// patternCount tracks the total number of patterns across all clusters.
	// This is maintained incrementally to avoid O(N) scans on every Add().
	patternCount int

	// estimatedBytes tracks an approximate memory footprint of patterns stored in this manager.
	// This is an estimate (not exact Go heap usage) and is intended for threshold-based eviction triggers.
	estimatedBytes int64
}

// NewClusterManager creates a new ClusterManager.
func NewClusterManager() *ClusterManager {
	return &ClusterManager{
		hashBuckets: make(map[uint64][]*Cluster),
		nextID:      1,
	}
}

// PatternCount returns the total number of patterns currently stored.
func (cm *ClusterManager) PatternCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.patternCount
}

// EstimatedBytes returns the approximate memory footprint (in bytes) of patterns currently stored.
func (cm *ClusterManager) EstimatedBytes() int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.estimatedBytes
}

// Add processes a TokenList and adds it to the appropriate cluster.
// Returns:
// - pattern: the pattern that was created/updated
// - changeType: what changed (new/updated/no change)
// - patternCount: total patterns after this addition
// - estimatedBytes: total estimated memory after this addition
func (cm *ClusterManager) Add(tokenList *token.TokenList) (*Pattern, PatternChangeType, int, int64) {
	if tokenList == nil || tokenList.IsEmpty() {
		log.Errorf("Cluster Manager failed to add log: %v for patterning. Token list is empty or nil.", tokenList.String())
		return nil, PatternNoChange, 0, 0
	}

	// Lock the cluster manager to prevent concurrent access to the hash buckets. Current implementation is single-threaded on local pipeline, but we will eventually build a shared cluster manager across multiple pipelines.
	// todo: implement a shared cluster manager across multiple pipelines
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Create new signature and hash it
	signature := token.NewSignature(tokenList)
	hash := signature.Hash

	// Get hash bucket
	clusters := cm.hashBuckets[hash]

	// Look for existing cluster with matching signature
	for _, cluster := range clusters {
		if !cluster.Signature.Equals(signature) {
			continue
		}

		// Find which pattern within the cluster the tokenList will match
		var matchedPattern *Pattern
		var oldWildcardCount int
		oldPatternCount := len(cluster.Patterns)
		var oldMatchedBytes int64
		for _, p := range cluster.Patterns {
			if p.Sample != nil && merging.CanMergeTokenLists(tokenList, p.Sample) {
				matchedPattern = p
				oldWildcardCount = p.GetWildcardCount()
				oldMatchedBytes = p.EstimatedBytes()
				break
			}
		}

		// Add the tokenList to the cluster (merges or creates new pattern)
		pattern := cluster.AddTokenListToPatterns(tokenList, cm)

		// Update counters (pattern count + estimated bytes)
		// If a new pattern was appended to this cluster, increment counts.
		if len(cluster.Patterns) > oldPatternCount {
			cm.patternCount++
			cm.estimatedBytes += pattern.EstimatedBytes()
		} else if matchedPattern != nil && matchedPattern.PatternID == pattern.PatternID {
			// Existing pattern updated; adjust bytes based on template evolution.
			cm.estimatedBytes += pattern.EstimatedBytes() - oldMatchedBytes
		}

		// Check if a new pattern was created (no match found or merge failed)
		if matchedPattern == nil || matchedPattern.PatternID != pattern.PatternID {
			return pattern, PatternNew, cm.patternCount, cm.estimatedBytes
		}

		// Check if wildcard count changed (pattern evolved)
		if pattern.GetWildcardCount() != oldWildcardCount {
			return pattern, PatternUpdated, cm.patternCount, cm.estimatedBytes
		}

		return pattern, PatternNoChange, cm.patternCount, cm.estimatedBytes
	}

	// If no matching pattern was found, create a new cluster and pattern.
	newCluster := NewCluster(signature)
	// Add the token list to create the first pattern
	pattern := newCluster.AddTokenListToPatterns(tokenList, cm)
	cm.hashBuckets[hash] = append(clusters, newCluster)

	// New cluster always creates exactly one new pattern
	cm.patternCount++
	cm.estimatedBytes += pattern.EstimatedBytes()

	return pattern, PatternNew, cm.patternCount, cm.estimatedBytes
}

// Clear removes all clusters.
func (cm *ClusterManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.hashBuckets = make(map[uint64][]*Cluster)
	cm.patternCount = 0
	cm.estimatedBytes = 0
}

// GetStats returns the current pattern count and estimated memory usage.
// This is a read-only operation that acquires a read lock for thread safety.
func (cm *ClusterManager) GetStats() (patternCount int, estimatedBytes int64) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.patternCount, cm.estimatedBytes
}

// generatePatternID generates a unique pattern ID using a monotonic counter.
// Must be called while holding the ClusterManager lock.
func (cm *ClusterManager) generatePatternID() uint64 {
	id := cm.nextID
	cm.nextID++
	return id
}
