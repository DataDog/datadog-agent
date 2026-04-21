// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
package clustering

import (
	"fmt"
	"sort"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// PatternChangeType indicates what changed when adding a TokenList to the cluster manager
type PatternChangeType int

const (
	// PatternNoChange means the TokenList was added to an existing cluster without structural changes
	PatternNoChange PatternChangeType = iota
	// PatternNew means a brand new pattern was created (either in a new cluster or within an existing one)
	PatternNew
	// PatternUpdated means an existing pattern's structure changed (more wildcards added)
	PatternUpdated
	// PatternTooLarge means the log exceeds the max template size and should be sent as a RawLog.
	// No pattern is created; the caller must not dereference the returned (nil) pattern.
	PatternTooLarge
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

	// firstWordProtection controls whether the first word token position is protected
	// from becoming a wildcard during merge. When true (default), logs that differ only
	// by first word (e.g., "ERROR" vs "WARN") create separate patterns.
	firstWordProtection bool

	// firstWordMaxCardinality is the per-cluster adaptive threshold. Once a cluster sees
	// more than this many unique concrete values at the first-word position, protection is
	// automatically disabled for that cluster. 0 means no adaptive override (uses firstWordProtection as-is).
	firstWordMaxCardinality int

	// saturatedThreshold is the number of consecutive identical TryMerge returns (tl1 pointer
	// unchanged) before a pattern is marked saturated and eligible for the single-pass fast path.
	// 0 disables saturation scoring.
	saturatedThreshold int

	// maxPatternsPerCluster caps len(Patterns) per cluster. 0 = unlimited.
	maxPatternsPerCluster int
	// scanBudget limits CanMerge iterations per message in the full-scan loop. 0 = unlimited.
	scanBudget int
	// maxTemplateSizeBytes rejects logs whose raw token content exceeds this byte threshold,
	// sending them as RawLog instead. 0 = unlimited. Prevents single huge logs (e.g. AWS
	// instance metadata dumps) from bloating snapshot state with useless ~1MB templates.
	maxTemplateSizeBytes int
}

// NewClusterManager creates a new ClusterManager.
func NewClusterManager() *ClusterManager {
	return &ClusterManager{
		hashBuckets:         make(map[uint64][]*Cluster),
		nextID:              1,
		firstWordProtection: true,
	}
}

// NewClusterManagerWithConfig creates a new ClusterManager with configurable options.
// firstWordProtection: global first-word merge protection toggle.
// firstWordMaxCardinality: per-cluster adaptive threshold (0 = no adaptive override).
// saturatedThreshold: consecutive identical merges before pattern is marked saturated (0 = disabled).
// maxPatternsPerCluster: per-cluster pattern cap; 0 = unlimited.
// scanBudget: CanMerge iterations per message in the full-scan loop; 0 = unlimited.
// maxTemplateSizeBytes: reject logs whose raw content exceeds this size; 0 = unlimited.
func NewClusterManagerWithConfig(firstWordProtection bool, firstWordMaxCardinality int, saturatedThreshold int, maxPatternsPerCluster int, scanBudget int, maxTemplateSizeBytes int) *ClusterManager {
	return &ClusterManager{
		hashBuckets:             make(map[uint64][]*Cluster),
		nextID:                  1,
		firstWordProtection:     firstWordProtection,
		firstWordMaxCardinality: firstWordMaxCardinality,
		saturatedThreshold:      saturatedThreshold,
		maxPatternsPerCluster:   maxPatternsPerCluster,
		scanBudget:              scanBudget,
		maxTemplateSizeBytes:    maxTemplateSizeBytes,
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

	// Reject logs whose raw content exceeds the template size limit. A 1KB+ template is
	// almost never reused (e.g. AWS metadata dumps), so storing it wastes snapshot bytes.
	// The caller should send these as RawLog datums instead.
	if cm.maxTemplateSizeBytes > 0 {
		rawLen := 0
		for i := range tokenList.Tokens {
			rawLen += len(tokenList.Tokens[i].Value)
		}
		if rawLen > cm.maxTemplateSizeBytes {
			return nil, PatternTooLarge, cm.patternCount, cm.estimatedBytes
		}
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

		oldPatternCount := len(cluster.Patterns)

		// Delegate to AddTokenListToPatterns which checks CanMerge against Template
		// (not Sample). The invariant Template >= Sample guarantees that
		// CanMerge(Template, X) accepts whenever CanMerge(Sample, X) would.
		// See TestClusterManager_TemplateAtLeastAsPermissiveAsSample.
		pattern, oldWildcardCount, oldMatchedBytes := cluster.AddTokenListToPatterns(tokenList, cm)

		newPatternCreated := len(cluster.Patterns) > oldPatternCount
		if newPatternCreated {
			cm.patternCount++
			cm.estimatedBytes += pattern.EstimatedBytes()
			return pattern, PatternNew, cm.patternCount, cm.estimatedBytes
		}

		cm.estimatedBytes += pattern.EstimatedBytes() - oldMatchedBytes

		if pattern.GetWildcardCount() != oldWildcardCount {
			return pattern, PatternUpdated, cm.patternCount, cm.estimatedBytes
		}

		return pattern, PatternNoChange, cm.patternCount, cm.estimatedBytes
	}

	// If no matching pattern was found, create a new cluster and pattern.
	newCluster := NewCluster(signature, cm.firstWordProtection, cm.firstWordMaxCardinality, cm.saturatedThreshold, cm.maxPatternsPerCluster, cm.scanBudget)
	pattern, _, _ := newCluster.AddTokenListToPatterns(tokenList, cm)
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

// PositionStats describes the token distribution at one position across all patterns in a cluster.
type PositionStats struct {
	Pos           int
	TokenType     string
	WildcardCount int
	ConcreteCount int
	UniqueValues  int
	ConflictRatio float64 // fraction of cross-pattern comparisons that are Conflict
}

// ClusterReport describes a cluster's pattern distribution for instrumentation.
type ClusterReport struct {
	Signature    string
	PatternCount int
	Positions    []PositionStats
}

// TopClusters returns the top N clusters by pattern count with per-position analysis.
// Used for instrumentation to understand mega-cluster structure before implementing an index.
func (cm *ClusterManager) TopClusters(n int) []ClusterReport {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Collect all clusters
	type entry struct {
		sig     string
		cluster *Cluster
	}
	var all []entry
	for _, clusters := range cm.hashBuckets {
		for _, c := range clusters {
			all = append(all, entry{c.Signature.String(), c})
		}
	}

	// Sort by pattern count descending
	sort.Slice(all, func(i, j int) bool {
		return len(all[i].cluster.Patterns) > len(all[j].cluster.Patterns)
	})

	if n > len(all) {
		n = len(all)
	}

	reports := make([]ClusterReport, 0, n)
	for _, e := range all[:n] {
		reports = append(reports, analyzeCluster(e.sig, e.cluster))
	}
	return reports
}

// analyzeCluster computes per-position stats for a cluster.
func analyzeCluster(sig string, c *Cluster) ClusterReport {
	patterns := c.Patterns
	if len(patterns) == 0 {
		return ClusterReport{Signature: sig}
	}

	nPos := 0
	for _, p := range patterns {
		if p.Template != nil && len(p.Template.Tokens) > nPos {
			nPos = len(p.Template.Tokens)
		}
	}

	positions := make([]PositionStats, nPos)
	for pos := 0; pos < nPos; pos++ {
		var wildcardCount, concreteCount int
		uniqueValues := make(map[string]struct{})
		tokenType := ""

		for _, p := range patterns {
			if p.Template == nil || pos >= len(p.Template.Tokens) {
				continue
			}
			tok := p.Template.Tokens[pos]
			tokenType = tok.Type.String()
			if tok.Wildcard == token.IsWildcard {
				wildcardCount++
			} else {
				concreteCount++
				uniqueValues[tok.Value] = struct{}{}
			}
		}

		// Sample conflict ratio: compare adjacent pattern pairs at this position
		conflictCount, total := 0, 0
		for i := 0; i+1 < len(patterns); i++ {
			p1, p2 := patterns[i], patterns[i+1]
			if p1.Template == nil || p2.Template == nil {
				continue
			}
			if pos >= len(p1.Template.Tokens) || pos >= len(p2.Template.Tokens) {
				continue
			}
			tok1, tok2 := &p1.Template.Tokens[pos], &p2.Template.Tokens[pos]
			if tok1.Wildcard == token.IsWildcard || tok2.Wildcard == token.IsWildcard {
				continue
			}
			result := tok1.Compare(tok2)
			total++
			if result == token.Conflict {
				conflictCount++
			}
		}

		conflictRatio := 0.0
		if total > 0 {
			conflictRatio = float64(conflictCount) / float64(total)
		}

		positions[pos] = PositionStats{
			Pos:           pos,
			TokenType:     tokenType,
			WildcardCount: wildcardCount,
			ConcreteCount: concreteCount,
			UniqueValues:  len(uniqueValues),
			ConflictRatio: conflictRatio,
		}
	}

	return ClusterReport{
		Signature:    sig,
		PatternCount: len(patterns),
		Positions:    positions,
	}
}

// FormatTopClusters returns a human-readable summary of top cluster stats.
func (cm *ClusterManager) FormatTopClusters(n int) string {
	reports := cm.TopClusters(n)
	if len(reports) == 0 {
		return "no clusters"
	}
	var out string
	for i, r := range reports {
		out += fmt.Sprintf("\nCluster #%d sig=%s patterns=%d\n", i+1, r.Signature, r.PatternCount)
		out += fmt.Sprintf("  %-5s %-20s %-8s %-8s %-8s %-12s\n",
			"pos", "type", "wildcard", "concrete", "unique", "conflictRatio")
		for _, p := range r.Positions {
			if p.ConcreteCount == 0 && p.WildcardCount == 0 {
				continue
			}
			out += fmt.Sprintf("  %-5d %-20s %-8d %-8d %-8d %.3f\n",
				p.Pos, p.TokenType, p.WildcardCount, p.ConcreteCount, p.UniqueValues, p.ConflictRatio)
		}
	}
	return out
}

// generatePatternID generates a unique pattern ID using a monotonic counter.
// Must be called while holding the ClusterManager lock.
func (cm *ClusterManager) generatePatternID() uint64 {
	id := cm.nextID
	cm.nextID++
	return id
}
