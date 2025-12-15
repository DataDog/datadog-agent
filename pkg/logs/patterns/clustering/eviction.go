// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
package clustering

import (
	"container/heap"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// patternHeap implements heap.Interface for efficient eviction based on scores.
// It's a min-heap: patterns with the lowest eviction scores bubble to the top.
type patternHeap struct {
	items []patternHeapItem
}

type patternHeapItem struct {
	pattern *Pattern
	score   float64 // Cached eviction score
}

// EvictionPolicy is the policy for evicting patterns from the cluster manager.
type EvictionPolicy int

const (
	// EvictionPolicyLFUDecay uses LFU with exponential age decay
	EvictionPolicyLFUDecay EvictionPolicy = iota
)

// calculateEvictionScore calculates the eviction score for a pattern.
// Lower scores indicate higher priority for eviction.
//
// The score combines:
// - Frequency (LogCount/HitCount): More frequent patterns get higher scores
// - Age decay: Older patterns gradually lose priority
// - Recency boost: Recently accessed patterns get bonus points
// Formula: score = (LogCount / (1 + age)^decayFactor) * (1 + recencyBoost). This uses a power-law decay algorithm to prioritize older patterns that are still used.
func (p *Pattern) calculateEvictionScore(now time.Time, decayFactor float64) float64 {
	// Base frequency score
	frequency := float64(p.LogCount)

	// Age-based decay (from CreatedAt)
	ageDays := now.Sub(p.CreatedAt).Hours() / 24.0

	// Clamp to reasonable range [0, 365] to handle clock skew
	if ageDays < 0 {
		ageDays = 0 // Clock moved backward
	} else if ageDays > 365 {
		ageDays = 365 // Clock moved forward or genuinely old pattern
	}

	// Apply power-law decay: score = frequency / (1 + age)^decayFactor
	ageDecay := 1.0 / math.Pow(1.0+ageDays, decayFactor)
	baseScore := frequency * ageDecay

	// Recency boost (from LastAccessAt)
	hoursSinceAccess := now.Sub(p.LastAccessAt).Hours()
	if hoursSinceAccess < 0 {
		hoursSinceAccess = 0 // Handle clock skew
	}

	// Patterns accessed recently get a bonus (hyperbolic decay)
	// recencyBoost ranges from ~1.0 (just accessed) to ~0.0 (very old access)
	recencyBoost := 1.0 / (1.0 + hoursSinceAccess/24.0)

	// Combine base score with recency boost
	// Frequency is primary signal, recency is secondary
	finalScore := baseScore * (1.0 + recencyBoost)

	return finalScore
}

// evictPattern evicts a specific pattern from the cluster manager.
func (cm *ClusterManager) evictPattern(pattern *Pattern) {
	var (
		targetHash uint64
		targetSig  *token.Signature
	)

	// Fast path: narrow search to the signature bucket when possible
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
}

// EvictLowestScoringPatterns evicts up to numToEvict patterns with the lowest eviction scores.
// Returns the list of evicted patterns.
func (cm *ClusterManager) EvictLowestScoringPatterns(numToEvict int, decayFactor float64) []*Pattern {
	if numToEvict <= 0 {
		return nil
	}

	now := time.Now()

	// Build heap of all patterns with their scores
	h := &patternHeap{
		items: make([]patternHeapItem, 0),
	}

	// Collect all patterns and calculate scores
	cm.mu.RLock()
	for _, clusters := range cm.hashBuckets {
		for _, cluster := range clusters {
			for _, pattern := range cluster.Patterns {
				score := pattern.calculateEvictionScore(now, decayFactor)
				h.items = append(h.items, patternHeapItem{
					pattern: pattern,
					score:   score,
				})
			}
		}
	}
	cm.mu.RUnlock()

	// Build the min-heap: O(N) operation
	heap.Init(h)

	// Extract and evict the N patterns with lowest scores
	evicted := make([]*Pattern, 0, numToEvict)
	for i := 0; i < numToEvict && h.Len() > 0; i++ {
		item := heap.Pop(h).(patternHeapItem)

		// Evict the pattern using existing eviction logic
		cm.evictPattern(item.pattern)
		evicted = append(evicted, item.pattern)
	}

	return evicted
}

// EvictToMemoryTarget evicts patterns until the target memory is freed.
// It uses actual pattern sizes rather than averages for precision.
func (cm *ClusterManager) EvictToMemoryTarget(targetBytesToFree int64, decayFactor float64) []*Pattern {
	if targetBytesToFree <= 0 {
		return nil
	}

	now := time.Now()

	// Build heap of all patterns sorted by eviction score
	h := &patternHeap{
		items: make([]patternHeapItem, 0),
	}

	cm.mu.RLock()
	for _, clusters := range cm.hashBuckets {
		for _, cluster := range clusters {
			for _, pattern := range cluster.Patterns {
				score := pattern.calculateEvictionScore(now, decayFactor)
				h.items = append(h.items, patternHeapItem{
					pattern: pattern,
					score:   score,
				})
			}
		}
	}
	cm.mu.RUnlock()

	heap.Init(h)

	// Evict patterns until we've freed enough memory
	evicted := make([]*Pattern, 0)
	bytesFreed := int64(0)

	for h.Len() > 0 && bytesFreed < targetBytesToFree {
		item := heap.Pop(h).(patternHeapItem)
		cm.evictPattern(item.pattern)
		bytesFreed += item.pattern.EstimatedBytes()
		evicted = append(evicted, item.pattern)
	}

	return evicted
}

// Len returns the number of items in the heap
func (h patternHeap) Len() int { return len(h.items) }

// Less reports whether item i should sort before item j (min-heap: lower scores first)
func (h patternHeap) Less(i, j int) bool {
	return h.items[i].score < h.items[j].score
}

// Swap exchanges items i and j
func (h patternHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

// Push adds an item to the heap (required by heap.Interface)
func (h *patternHeap) Push(x interface{}) {
	h.items = append(h.items, x.(patternHeapItem))
}

// Pop removes and returns the minimum item (required by heap.Interface)
func (h *patternHeap) Pop() interface{} {
	old := h.items
	n := len(old)
	item := old[n-1]
	h.items = old[0 : n-1]
	return item
}
