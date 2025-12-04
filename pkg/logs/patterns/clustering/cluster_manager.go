// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists
// and identifying wildcard positions for pattern extraction.
package clustering

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"

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
}

// NewClusterManager creates a new ClusterManager.
func NewClusterManager() *ClusterManager {
	return &ClusterManager{
		hashBuckets: make(map[uint64][]*Cluster),
	}
}

// Add processes a TokenList and adds it to the appropriate cluster.
// Returns the pattern that was created/updated and a PatternChangeType indicating what changed.
func (cm *ClusterManager) Add(tokenList *token.TokenList) (*Pattern, PatternChangeType) {
	if tokenList == nil || tokenList.IsEmpty() {
		log.Errorf("Cluster Manager failed to add log: %v for patterning. Token list is empty or nil.", tokenList.String())
		return nil, PatternNoChange
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
		for _, p := range cluster.Patterns {
			if p.Sample != nil && merging.CanMergeTokenLists(tokenList, p.Sample) {
				matchedPattern = p
				oldWildcardCount = p.GetWildcardCount()
				break
			}
		}

		// Add the tokenList to the cluster (merges or creates new pattern)
		pattern := cluster.AddTokenListToPatterns(tokenList)

		// Check if a new pattern was created (no match found or merge failed)
		if matchedPattern == nil || matchedPattern.PatternID != pattern.PatternID {
			return pattern, PatternNew
		}

		// Check if wildcard count changed (pattern evolved)
		if pattern.GetWildcardCount() != oldWildcardCount {
			return pattern, PatternUpdated
		}

		return pattern, PatternNoChange
	}

	// If no matching pattern was found, create a new cluster and pattern.
	newCluster := NewCluster(signature)
	// Add the token list to create the first pattern
	pattern := newCluster.AddTokenListToPatterns(tokenList)
	cm.hashBuckets[hash] = append(clusters, newCluster)

	return pattern, PatternNew
}

// Clear removes all clusters.
func (cm *ClusterManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.hashBuckets = make(map[uint64][]*Cluster)
}

// generatePatternID generates a unique pattern ID
func generatePatternID() uint64 {
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return uint64(time.Now().UnixNano())
	}
	return binary.BigEndian.Uint64(buf[:])
}
