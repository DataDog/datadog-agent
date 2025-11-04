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
	hashBuckets     map[uint64][]*Cluster
	totalTokenLists int
	totalClusters   int
}

// NewClusterManager creates a new ClusterManager.
func NewClusterManager() *ClusterManager {
	return &ClusterManager{
		hashBuckets:     make(map[uint64][]*Cluster),
		totalTokenLists: 0,
		totalClusters:   0,
	}
}

// Add processes a TokenList and adds it to the appropriate cluster.
// Returns the cluster and a PatternChangeType indicating what changed.
func (cm *ClusterManager) Add(tokenList *token.TokenList) (*Cluster, PatternChangeType) {
	if tokenList == nil || tokenList.IsEmpty() {
		return nil, PatternNoChange
	}

	signature := token.NewSignature(tokenList)
	hash := signature.Hash

	clusters := cm.hashBuckets[hash]

	for _, cluster := range clusters {
		if cluster.Signature.Equals(signature) {
			// Check if pattern will be updated
			// If cluster already has a pattern and we're adding more token lists,
			// the pattern might gain new wildcards
			willUpdate := cluster.Size() > 1 && cluster.Pattern != nil

			cluster.Add(tokenList)
			cm.totalTokenLists++

			if willUpdate {
				return cluster, PatternUpdated
			}
			return cluster, PatternNoChange
		}
	}

	// Creating a new cluster means a new pattern
	newCluster := NewCluster(signature, tokenList)
	newCluster.SetPatternID(generatePatternID())
	cm.hashBuckets[hash] = append(clusters, newCluster)

	cm.totalTokenLists++
	cm.totalClusters++

	return newCluster, PatternNew
}

// GetCluster retrieves the cluster with the given signature.
func (cm *ClusterManager) GetCluster(signature token.Signature) *Cluster {
	hash := signature.Hash

	clusters, exists := cm.hashBuckets[hash]
	if !exists {
		return nil
	}

	for _, cluster := range clusters {
		if cluster.Signature.Equals(signature) {
			return cluster
		}
	}

	return nil
}

// GetClustersWithPatterns returns all clusters that have patterns defined.
// This is useful for re-sending pattern state after stream rotation.
func (cm *ClusterManager) GetClustersWithPatterns() []*Cluster {
	var clustersWithPatterns []*Cluster

	for _, clusters := range cm.hashBuckets {
		for _, cluster := range clusters {
			// Only include clusters with actual patterns
			if cluster.Pattern != nil {
				clustersWithPatterns = append(clustersWithPatterns, cluster)
			}
		}
	}

	return clustersWithPatterns
}

// Clear removes all clusters and resets statistics.
func (cm *ClusterManager) Clear() {
	cm.hashBuckets = make(map[uint64][]*Cluster)
	cm.totalTokenLists = 0
	cm.totalClusters = 0
}

// GetAllClusters returns all clusters in the manager.
func (cm *ClusterManager) GetAllClusters() []*Cluster {
	var allClusters []*Cluster

	for _, clusters := range cm.hashBuckets {
		allClusters = append(allClusters, clusters...)
	}

	return allClusters
}

// GetClustersByLength returns clusters by length.
func (cm *ClusterManager) GetClustersByLength(length int) []*Cluster {
	var result []*Cluster

	for _, clusters := range cm.hashBuckets {
		for _, cluster := range clusters {
			if cluster.Signature.Length == length {
				result = append(result, cluster)
			}
		}
	}

	return result
}

// GetClustersByHash returns clusters by hash.
func (cm *ClusterManager) GetClustersByHash(hash uint64) []*Cluster {
	if clusters, exists := cm.hashBuckets[hash]; exists {
		result := make([]*Cluster, len(clusters))
		copy(result, clusters)
		return result
	}

	return []*Cluster{}
}

// Stats returns statistics about the clustering.
type ClusterStats struct {
	TotalTokenLists    int
	TotalClusters      int
	HashBuckets        int
	AverageClusterSize float64
}

// GetStats returns current clustering statistics.
func (cm *ClusterManager) GetStats() ClusterStats {
	avgSize := 0.0
	if cm.totalClusters > 0 {
		avgSize = float64(cm.totalTokenLists) / float64(cm.totalClusters)
	}

	return ClusterStats{
		TotalTokenLists:    cm.totalTokenLists,
		TotalClusters:      cm.totalClusters,
		HashBuckets:        len(cm.hashBuckets),
		AverageClusterSize: avgSize,
	}
}

// GetLargestClusters returns the N largest clusters.
func (cm *ClusterManager) GetLargestClusters(n int) []*Cluster {
	allClusters := cm.GetAllClusters()

	// Simple bubble sort for small N
	for i := 0; i < len(allClusters)-1; i++ {
		for j := 0; j < len(allClusters)-i-1; j++ {
			if allClusters[j].Size() < allClusters[j+1].Size() {
				allClusters[j], allClusters[j+1] = allClusters[j+1], allClusters[j]
			}
		}
	}

	if n > len(allClusters) {
		n = len(allClusters)
	}

	return allClusters[:n]
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
