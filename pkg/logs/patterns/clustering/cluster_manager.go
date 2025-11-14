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

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
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
		return nil, PatternNoChange
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Create new signature and hash it
	signature := token.NewSignature(tokenList)
	hash := signature.Hash

	// Get hash bucket
	clusters := cm.hashBuckets[hash]

	// Look for existing cluster with matching signature
	for _, cluster := range clusters {
		if cluster.Signature.Equals(signature) {
			// Track the state before adding
			hadPatterns := len(cluster.Patterns) > 0
			oldPatternCount := len(cluster.Patterns)

			// Track if patterns had wildcards before
			hadWildcards := false
			if hadPatterns {
				for _, p := range cluster.Patterns {
					if p.hasWildcards() {
						hadWildcards = true
						break
					}
				}
			}

			// Add to appropriate pattern within the cluster
			pattern := cluster.AddTokenListToPatterns(tokenList)

			// Determine if this created a new pattern or updated an existing one
			if pattern != nil {
				newPatternCount := len(cluster.Patterns)
				if newPatternCount > oldPatternCount {
					// New pattern was created within the cluster (multi-pattern scenario)
					return pattern, PatternNew
				}

				// Check if wildcards were added to an existing pattern
				if hadPatterns && pattern.hasWildcards() && !hadWildcards {
					// Pattern gained wildcards
					return pattern, PatternUpdated
				}

				// If pattern already had wildcards and got more, it's also an update
				if hadPatterns && hadWildcards && pattern.size() > 2 {
					// Pattern structure may have changed (more wildcards)
					return pattern, PatternUpdated
				}
			}
			return pattern, PatternNoChange
		}
	}

	// Creating a new cluster means a new pattern
	newCluster := NewCluster(signature, tokenList)
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
