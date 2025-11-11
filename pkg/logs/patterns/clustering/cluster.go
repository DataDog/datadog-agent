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

// Cluster represents a group of TokenLists with identical signatures.
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
func NewCluster(signature token.Signature, tokenList *token.TokenList) *Cluster {
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
			// Merge into existing pattern (same PatternID is preserved)
			p.LogCount++
			p.UpdatedAt = time.Now()
			c.UpdatedAt = time.Now()

			// Incrementally merge the new token list into the pattern template
			c.regeneratePattern(p, tokenList)
			return p // Return existing pattern with updated template
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
func (c *Cluster) regeneratePattern(p *Pattern, newTokenList *token.TokenList) {
	if p.Template == nil {
		return
	}

	// Incremental merge: merge new log with existing template
	merged := merging.MergeTokenLists(p.Template, newTokenList)
	if merged == nil {
		// Merge failed (shouldn't happen since CanMergeTokenLists passed), keep current template
		return
	}

	p.Template = merged
	p.Positions = make([]int, 0, merged.Length())

	// Build wildcard positions list
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
}

// =============================================================================
// Pattern Access Methods
// =============================================================================

// FindMatchingPattern finds the Pattern that matches the given TokenList.
// Returns the matching Pattern, or nil if no match found.
func (c *Cluster) FindMatchingPattern(tokenList *token.TokenList) *Pattern {
	// Ensure patterns are generated
	if len(c.Patterns) == 0 {
		return nil
	}

	// Try to find a Pattern where the TokenList can merge
	for _, p := range c.Patterns {
		// Check if this TokenList can merge with the pattern's sample
		if p.Sample != nil && merging.CanMergeTokenLists(tokenList, p.Sample) {
			return p
		}
	}

	// Fallback: return most common pattern (largest group)
	return c.GetMostCommonPattern()
}

// GetPatternString returns a string representation of the most common pattern.
// For backward compatibility.
func (c *Cluster) GetPatternString() string {
	primary := c.GetMostCommonPattern()
	if primary == nil {
		return ""
	}
	return primary.getPatternString()
}

// GetMostCommonPattern returns the pattern with the highest log count in this cluster.
// When a cluster contains multiple patterns (due to structural differences like special characters),
// this returns the most frequently occurring pattern, which is typically the most representative.
func (c *Cluster) GetMostCommonPattern() *Pattern {
	if len(c.Patterns) == 0 {
		return nil
	}

	mostCommonIdx := 0
	maxLogCount := c.Patterns[0].LogCount
	for idx, p := range c.Patterns {
		if p.LogCount > maxLogCount {
			maxLogCount = p.LogCount
			mostCommonIdx = idx
		}
	}
	return c.Patterns[mostCommonIdx]
}

// GetAllPatterns returns all Patterns in this cluster.
func (c *Cluster) GetAllPatterns() []*Pattern {
	return c.Patterns
}

// GetPatternID returns the pattern ID for the most common pattern.
// For backward compatibility.
func (c *Cluster) GetPatternID() uint64 {
	primary := c.GetMostCommonPattern()
	if primary == nil {
		return 0
	}
	return primary.PatternID
}

// =============================================================================
// Wildcard Methods
// =============================================================================

// HasWildcards returns true if any pattern in this cluster contains wildcard positions.
func (c *Cluster) HasWildcards() bool {
	for _, p := range c.Patterns {
		if p.hasWildcards() {
			return true
		}
	}
	return false
}

// GetWildcardPositions returns wildcard token positions for the most common pattern.
// For backward compatibility.
func (c *Cluster) GetWildcardPositions() []int {
	primary := c.GetMostCommonPattern()
	if primary == nil {
		return nil
	}
	return primary.getWildcardPositions()
}

// GetWildcardCharPositions returns character positions where wildcards appear in the most common pattern string.
// For backward compatibility.
func (c *Cluster) GetWildcardCharPositions() []int {
	primary := c.GetMostCommonPattern()
	if primary == nil {
		return nil
	}
	return primary.getWildcardCharPositions()
}

// GetWildcardValues extracts the actual values from the most recent token list in the most common pattern.
// For backward compatibility.
func (c *Cluster) GetWildcardValues() []string {
	primary := c.GetMostCommonPattern()
	if primary == nil {
		return nil
	}
	return primary.getWildcardValues()
}

// ExtractWildcardValues extracts the wildcard values from a specific TokenList.
// Uses the matching Pattern to determine wildcard positions.
func (c *Cluster) ExtractWildcardValues(tokenList *token.TokenList) []string {
	// Find the matching pattern for this TokenList
	p := c.FindMatchingPattern(tokenList)
	if p == nil {
		return []string{}
	}
	return p.extractWildcardValues(tokenList)
}

// =============================================================================
// State Management & Metadata
// =============================================================================

// Size returns the total number of TokenLists across all patterns in this cluster.
func (c *Cluster) Size() int {
	total := 0
	for _, p := range c.Patterns {
		total += p.size()
	}
	return total
}

// MarkAsSent updates the LastSentAt timestamp for all patterns.
func (c *Cluster) MarkAsSent() {
	for _, p := range c.Patterns {
		p.markAsSent()
	}
}

// NeedsSending returns true if any pattern has never been sent or has been updated since last sent.
func (c *Cluster) NeedsSending() bool {
	for _, p := range c.Patterns {
		if p.needsSending() {
			return true
		}
	}
	return false
}

// =============================================================================
// Helper Functions
// =============================================================================

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
