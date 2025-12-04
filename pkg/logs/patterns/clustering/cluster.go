// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists
// and identifying wildcard positions for pattern extraction.
package clustering

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering/merging"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Cluster represents a cluster with a group of TokenLists that have identical signatures.
// A cluster may contain multiple patterns if token lists with the same signature cannot be merged since structural Fidelity is Valuable.
// Examples:
// "Status: OK"     → HTTP response format
// "Status; OK"     → CSV-like format
// "Status OK"      → Plain text format
// These are different log formats, even if semantically similar → we need to keep them separate.
type Cluster struct {
	Signature token.Signature
	Patterns  []*Pattern // Multiple patterns per cluster

	// Timestamp tracking for the cluster itself
	CreatedAt time.Time // When cluster was first created
	UpdatedAt time.Time // When cluster was last modified (any pattern changed)
}

// NewCluster creates a new cluster.
func NewCluster(signature token.Signature) *Cluster {
	now := time.Now()
	return &Cluster{
		Signature: signature,
		Patterns:  nil, // Will be generated when needed
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// =============================================================================
// Core Clustering Logic
// =============================================================================

// AddTokenListToPatterns adds a TokenList to the appropriate pattern in the cluster.
// If no matching pattern exists, creates a new one.
func (c *Cluster) AddTokenListToPatterns(tokenList *token.TokenList) *Pattern {
	// Ensure patterns are generated
	if len(c.Patterns) == 0 {
		// No patterns yet, create first one
		patternID := generatePatternID()
		pattern := newPattern(tokenList, patternID)

		c.Patterns = []*Pattern{pattern}
		// Update the cluster's new pattern at timestamp
		c.UpdatedAt = time.Now()
		return pattern
	}

	// Try to find a matching pattern
	for _, p := range c.Patterns {
		// Check if this TokenList can merge with this pattern's sample
		if p.Sample != nil && merging.CanMergeTokenLists(tokenList, p.Sample) {
			// CRITICAL: Also verify it can merge with the template
			// If template has evolved differently, regeneratePattern will fail
			// and we should create a new pattern instead
			// Note: CanMergeTokenLists is not symmetric, so check both directions
			if p.Template != nil {
				templateCompatible1 := merging.CanMergeTokenLists(p.Template, tokenList)
				templateCompatible2 := merging.CanMergeTokenLists(tokenList, p.Template)
				templateCompatible := templateCompatible1 || templateCompatible2
				if !templateCompatible {
					// Log matches sample but not template - template has evolved incompatibly
					// Skip this pattern and continue searching or create new one
					// This will create a new pattern instead
					continue
				}
			}

			// Merge into existing pattern (same PatternID is preserved)
			p.LogCount++
			p.UpdatedAt = time.Now()
			c.UpdatedAt = time.Now()

			// Incrementally merge the new token list into the pattern template
			// regeneratePattern will update template if merge succeeds
			if c.regeneratePattern(p, tokenList) {
				return p // Return existing pattern with updated template
			}
			// regeneratePattern failed - template couldn't merge with tokenList
			// This shouldn't happen if we checked above, but handle it gracefully
			// Create a new pattern instead
			break
		}
	}

	// No matching pattern found, create a new one
	patternID := generatePatternID()
	pattern := newPattern(tokenList, patternID)
	c.Patterns = append(c.Patterns, pattern)
	c.UpdatedAt = time.Now()
	return pattern
}

// regeneratePattern incrementally merges a new token list into the pattern.
// Returns true if merge succeeded, false if merge failed.
func (c *Cluster) regeneratePattern(p *Pattern, newTokenList *token.TokenList) bool {
	if p.Template == nil {
		return false
	}

	// Incremental merge: merge new log with existing template
	merged := merging.MergeTokenLists(p.Template, newTokenList)
	if merged == nil {
		// Merge failed - template and newTokenList are incompatible
		return false
	}

	p.Template = merged
	p.Positions = make([]int, 0, merged.Length())

	// Build wildcard positions list when 2 tokenlists are mergable.
	for i := 0; i < merged.Length(); i++ {
		tok := merged.Tokens[i]
		if tok.Wildcard == token.IsWildcard {
			p.Positions = append(p.Positions, i)

			// Special handling for path wildcards
			if tok.Type == token.TokenAbsolutePath && p.Sample != nil && i < p.Sample.Length() {
				firstPath := p.Sample.Tokens[i].Value
				merged.Tokens[i].Value = getPathPattern(firstPath)
			}
		}
	}

	p.UpdatedAt = time.Now()
	return true
}

// getPathPattern converts a path to hierarchical wildcard pattern
func getPathPattern(path string) string {
	if path == "/" {
		return "/"
	}

	// Remove leading/trailing slashes and split
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "/"
	}

	parts := strings.Split(trimmed, "/")
	result := ""
	for i := 0; i < len(parts); i++ {
		result += "/*"
	}

	return result
}
