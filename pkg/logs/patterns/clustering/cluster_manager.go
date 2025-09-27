// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists
// and identifying wildcard positions for pattern extraction.
package clustering

import (
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
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
func (cm *ClusterManager) Add(tokenList *token.TokenList) *Cluster {
	if tokenList == nil || tokenList.IsEmpty() {
		return nil
	}

	signature := tokenList.Signature()
	hash := signature.Hash

	clusters := cm.hashBuckets[hash]

	for _, cluster := range clusters {
		if cluster.Signature.Equals(signature) {
			cluster.Add(tokenList)
			cm.totalTokenLists++
			return cluster
		}
	}

	newCluster := NewCluster(signature, tokenList)
	cm.hashBuckets[hash] = append(clusters, newCluster)

	cm.totalTokenLists++
	cm.totalClusters++

	return newCluster
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
