// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"strings"
)

// CompressedGroup is a compact structural description of a correlated group of anomalies.
type CompressedGroup struct {
	CorrelatorName string            `json:"correlator"`
	GroupID        string            `json:"groupId"`
	Title          string            `json:"title"`
	CommonTags     map[string]string `json:"commonTags"`
	Patterns       []MetricPattern   `json:"patterns"`
	MemberSources  []string          `json:"memberSources"`
	SeriesCount    int               `json:"seriesCount"`
	Precision      float64           `json:"precision"`
	FirstSeen      int64             `json:"firstSeen,omitempty"`
	LastUpdated    int64             `json:"lastUpdated,omitempty"`
}

// MetricPattern describes a wildcard or exact metric name pattern within a compressed group.
type MetricPattern struct {
	Pattern   string  `json:"pattern"`
	Matched   int     `json:"matched"`
	Universe  int     `json:"universe"`
	Precision float64 `json:"precision"`
}

// seriesCompact is a lightweight representation of a series for compression.
type seriesCompact struct {
	Namespace string
	Name      string
	Tags      []string
}

// extractCommonTags finds tags shared by all members, returning common tags as a map
// and the residual per-member tags.
func extractCommonTags(members []seriesCompact) (common map[string]string, residuals [][]string) {
	common = make(map[string]string)
	if len(members) == 0 {
		return common, nil
	}

	// Parse tags from first member as candidates
	candidates := make(map[string]string)
	for _, tag := range members[0].Tags {
		k, v := splitTag(tag)
		candidates[k] = v
	}

	// Intersect with remaining members
	for _, m := range members[1:] {
		memberTags := make(map[string]string)
		for _, tag := range m.Tags {
			k, v := splitTag(tag)
			memberTags[k] = v
		}
		for k, v := range candidates {
			if mv, ok := memberTags[k]; !ok || mv != v {
				delete(candidates, k)
			}
		}
	}

	common = candidates

	// Compute residuals
	residuals = make([][]string, len(members))
	for i, m := range members {
		for _, tag := range m.Tags {
			k, _ := splitTag(tag)
			if _, isCommon := common[k]; !isCommon {
				residuals[i] = append(residuals[i], tag)
			}
		}
	}

	return common, residuals
}

// splitTag splits a "key:value" tag into its parts.
func splitTag(tag string) (key, value string) {
	if idx := strings.Index(tag, ":"); idx >= 0 {
		return tag[:idx], tag[idx+1:]
	}
	return tag, ""
}

// trieNode is a node in a metric name trie.
type trieNode struct {
	segment       string
	children      map[string]*trieNode
	memberCount   int // group members passing through this node
	universeCount int // all known series passing through this node
	isLeaf        bool
}

func newTrieNode(segment string) *trieNode {
	return &trieNode{
		segment:  segment,
		children: make(map[string]*trieNode),
	}
}

// buildTrie constructs a trie from metric names split on '.'.
// memberNames are the group members; universeNames are all known series names.
func buildTrie(memberNames, universeNames []string) *trieNode {
	root := newTrieNode("")

	// Insert universe names
	for _, name := range universeNames {
		segments := strings.Split(name, ".")
		node := root
		node.universeCount++
		for _, seg := range segments {
			child, ok := node.children[seg]
			if !ok {
				child = newTrieNode(seg)
				node.children[seg] = child
			}
			child.universeCount++
			node = child
		}
		node.isLeaf = true
	}

	// Mark member names
	memberSet := make(map[string]struct{}, len(memberNames))
	for _, name := range memberNames {
		memberSet[name] = struct{}{}
	}
	for _, name := range memberNames {
		segments := strings.Split(name, ".")
		node := root
		node.memberCount++
		for _, seg := range segments {
			child, ok := node.children[seg]
			if !ok {
				// Member name not in universe - add it
				child = newTrieNode(seg)
				node.children[seg] = child
				child.universeCount++
			}
			child.memberCount++
			node = child
		}
		node.isLeaf = true
	}

	return root
}

// compressFromTrie does a DFS over the trie, emitting wildcard patterns where
// memberCount/universeCount >= threshold.
func compressFromTrie(root *trieNode, threshold float64) []MetricPattern {
	var patterns []MetricPattern
	for _, child := range root.children {
		patterns = append(patterns, compressNode(child, "", threshold)...)
	}
	// Sort for deterministic output
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Pattern < patterns[j].Pattern
	})
	return patterns
}

func compressNode(node *trieNode, prefix string, threshold float64) []MetricPattern {
	fullPath := node.segment
	if prefix != "" {
		fullPath = prefix + "." + node.segment
	}

	// If this node has no member traffic, skip entirely
	if node.memberCount == 0 {
		return nil
	}

	// Check if we can wildcard at this level
	if node.universeCount > 0 && len(node.children) > 0 {
		precision := float64(node.memberCount) / float64(node.universeCount)
		if precision >= threshold {
			// Emit wildcard pattern covering this subtree
			return []MetricPattern{{
				Pattern:   fullPath + ".*",
				Matched:   node.memberCount,
				Universe:  node.universeCount,
				Precision: precision,
			}}
		}
	}

	// Leaf node with members: emit exact match
	if node.isLeaf && len(node.children) == 0 {
		precision := 1.0
		if node.universeCount > 0 {
			precision = float64(node.memberCount) / float64(node.universeCount)
		}
		return []MetricPattern{{
			Pattern:   fullPath,
			Matched:   node.memberCount,
			Universe:  node.universeCount,
			Precision: precision,
		}}
	}

	// Recurse into children
	var patterns []MetricPattern
	for _, child := range node.children {
		patterns = append(patterns, compressNode(child, fullPath, threshold)...)
	}

	// If this is also a leaf (has both children and is a leaf endpoint), include it
	if node.isLeaf && node.memberCount > 0 {
		precision := 1.0
		if node.universeCount > 0 {
			precision = float64(node.memberCount) / float64(node.universeCount)
		}
		patterns = append(patterns, MetricPattern{
			Pattern:   fullPath,
			Matched:   node.memberCount,
			Universe:  node.universeCount,
			Precision: precision,
		})
	}

	return patterns
}

// stripAggSuffix removes :avg, :count, etc. from metric names for grouping purposes.
func stripAggSuffix(name string) string {
	if idx := strings.LastIndex(name, ":"); idx != -1 {
		suffix := name[idx+1:]
		switch suffix {
		case "avg", "count", "sum", "min", "max":
			return name[:idx]
		}
	}
	return name
}

// CompressGroup produces a CompressedGroup from a set of member series and a universe of all series.
func CompressGroup(correlatorName, groupID, title string, members []seriesCompact, universe []seriesCompact, threshold float64) CompressedGroup {
	if len(members) == 0 {
		return CompressedGroup{
			CorrelatorName: correlatorName,
			GroupID:        groupID,
			Title:          title,
			CommonTags:     map[string]string{},
			Patterns:       []MetricPattern{},
			MemberSources:  []string{},
		}
	}

	// Extract common tags
	commonTags, _ := extractCommonTags(members)

	// Collect member and universe metric names (stripped of agg suffix)
	memberNameSet := make(map[string]struct{})
	var memberSources []string
	for _, m := range members {
		stripped := stripAggSuffix(m.Name)
		memberNameSet[stripped] = struct{}{}
		memberSources = append(memberSources, seriesKey(m.Namespace, m.Name, m.Tags))
	}
	memberNames := make([]string, 0, len(memberNameSet))
	for name := range memberNameSet {
		memberNames = append(memberNames, name)
	}
	sort.Strings(memberNames)

	universeNameSet := make(map[string]struct{})
	for _, u := range universe {
		stripped := stripAggSuffix(u.Name)
		universeNameSet[stripped] = struct{}{}
	}
	universeNames := make([]string, 0, len(universeNameSet))
	for name := range universeNameSet {
		universeNames = append(universeNames, name)
	}
	sort.Strings(universeNames)

	// Build trie and compress
	root := buildTrie(memberNames, universeNames)
	patterns := compressFromTrie(root, threshold)

	// Calculate overall precision
	totalMatched := 0
	totalUniverse := 0
	for _, p := range patterns {
		totalMatched += p.Matched
		totalUniverse += p.Universe
	}
	overallPrecision := 0.0
	if totalUniverse > 0 {
		overallPrecision = float64(totalMatched) / float64(totalUniverse)
	}

	sort.Strings(memberSources)

	// Generate a title if none provided
	if title == "" {
		title = fmt.Sprintf("%s group %s", correlatorName, groupID)
	}

	return CompressedGroup{
		CorrelatorName: correlatorName,
		GroupID:        groupID,
		Title:          title,
		CommonTags:     commonTags,
		Patterns:       patterns,
		MemberSources:  memberSources,
		SeriesCount:    len(members),
		Precision:      overallPrecision,
	}
}
