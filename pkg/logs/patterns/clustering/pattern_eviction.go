// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
package clustering

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/eviction"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EvictionPolicy is the policy for evicting patterns from the cluster manager.
type EvictionPolicy int

const (
	// EvictionPolicyLFUDecay uses LFU with exponential age decay
	EvictionPolicyLFUDecay EvictionPolicy = iota
)

// CollectEvictables collects all patterns as evictables (implements eviction.EvictableCollection).
func (cm *ClusterManager) CollectEvictables() []eviction.Evictable {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	evictables := make([]eviction.Evictable, 0, cm.patternCount)
	for _, clusters := range cm.hashBuckets {
		for _, cluster := range clusters {
			for _, pattern := range cluster.Patterns {
				evictables = append(evictables, pattern)
			}
		}
	}
	return evictables
}

// RemoveEvictable removes a specific pattern from the cluster manager (implements eviction.EvictableCollection).
func (cm *ClusterManager) RemoveEvictable(item eviction.Evictable) {
	pattern := item.(*Pattern)
	cm.removePattern(pattern)
}

// removePattern removes a specific pattern from the cluster manager's hash buckets.
func (cm *ClusterManager) removePattern(pattern *Pattern) {
	var (
		targetHash uint64
		targetSig  *token.Signature
	)

	if pattern.Sample != nil && !pattern.Sample.IsEmpty() {
		sig := token.NewSignature(pattern.Sample)
		targetSig = &sig
		targetHash = sig.GetHashBucket()
	} else if pattern.Template != nil && !pattern.Template.IsEmpty() {
		sig := token.NewSignature(pattern.Template)
		targetSig = &sig
		targetHash = sig.GetHashBucket()
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	removeFromBucket := func(hash uint64, clusters []*Cluster, verifySignature bool) bool {
		for ci := 0; ci < len(clusters); ci++ {
			cluster := clusters[ci]
			if verifySignature && targetSig != nil && !targetSig.Equals(cluster.Signature) {
				continue
			}

			for pi := 0; pi < len(cluster.Patterns); pi++ {
				if cluster.Patterns[pi] != pattern {
					continue
				}

				// Update counters before removing pattern
				cm.patternCount--
				cm.estimatedBytes -= pattern.EstimatedBytes()

				// Remove pattern with swap delete for speed
				lastPattern := len(cluster.Patterns) - 1
				cluster.Patterns[pi] = cluster.Patterns[lastPattern]
				cluster.Patterns = cluster.Patterns[:lastPattern]
				cluster.UpdatedAt = time.Now()

				if len(cluster.Patterns) == 0 {
					// Remove cluster from bucket
					lastCluster := len(clusters) - 1
					clusters[ci] = clusters[lastCluster]
					clusters = clusters[:lastCluster]

					if len(clusters) == 0 {
						delete(cm.hashBuckets, hash)
					} else {
						cm.hashBuckets[hash] = clusters
					}
				} else {
					cm.hashBuckets[hash] = clusters
				}
				return true
			}
		}
		return false
	}

	// Try targeted bucket first
	if targetSig != nil {
		if clusters, ok := cm.hashBuckets[targetHash]; ok {
			if removeFromBucket(targetHash, clusters, true) {
				return
			}
		}
	}

	// Fallback: full scan if signature was missing or not found in target bucket
	for hash, clusters := range cm.hashBuckets {
		if removeFromBucket(hash, clusters, false) {
			return
		}
	}

	log.Warnf("Pattern %d not found during eviction, may have been already removed", pattern.PatternID)
}

// EvictLowestScoringPatterns evicts up to numToEvict patterns with the lowest eviction scores.
// Returns the list of evicted patterns.
func (cm *ClusterManager) EvictLowestScoringPatterns(numToEvict int, decayFactor float64) []*Pattern {
	evictables := eviction.EvictLowestScoring(cm, numToEvict, decayFactor)
	if len(evictables) == 0 {
		return nil
	}
	patterns := make([]*Pattern, len(evictables))
	for i, ev := range evictables {
		patterns[i] = ev.(*Pattern)
	}
	return patterns
}

// EvictToMemoryTarget evicts patterns until the target memory is freed.
// It uses actual pattern sizes rather than averages for precision.
func (cm *ClusterManager) EvictToMemoryTarget(targetBytesToFree int64, decayFactor float64) []*Pattern {
	evictables := eviction.EvictToMemoryTarget(cm, targetBytesToFree, decayFactor)
	if len(evictables) == 0 {
		return nil
	}
	patterns := make([]*Pattern, len(evictables))
	for i, ev := range evictables {
		patterns[i] = ev.(*Pattern)
	}
	return patterns
}
