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
type Cluster struct {
	Signature   token.Signature
	TokenLists  []*token.TokenList
	Pattern     *token.TokenList
	WildcardMap map[int]bool
	PatternID   uint64

	// Timestamp tracking for stateful encoding
	CreatedAt  time.Time // When pattern was first created
	UpdatedAt  time.Time // When pattern was last modified
	LastSentAt time.Time // When we last sent this pattern to gRPC
}

// NewCluster creates a new cluster.
func NewCluster(signature token.Signature, tokenList *token.TokenList) *Cluster {
	now := time.Now()
	return &Cluster{
		Signature:   signature,
		TokenLists:  []*token.TokenList{tokenList},
		Pattern:     nil,
		WildcardMap: make(map[int]bool),
		PatternID:   0, // Will be assigned when pattern is generated
		CreatedAt:   now,
		UpdatedAt:   now,
		LastSentAt:  time.Time{}, // Zero time - never sent
	}
}

// Add adds a TokenList to this cluster if it has a matching signature.
func (c *Cluster) Add(tokenList *token.TokenList) bool {
	signature := token.NewSignature(tokenList)

	if !c.Signature.Equals(signature) {
		return false
	}

	c.TokenLists = append(c.TokenLists, tokenList)

	c.Pattern = nil
	c.WildcardMap = make(map[int]bool)
	c.UpdatedAt = time.Now() // Pattern will change when regenerated z

	return true
}

// Size returns the number of TokenLists in this cluster.
func (c *Cluster) Size() int {
	return len(c.TokenLists)
}

// GeneratePattern analyzes all TokenLists in the cluster to identify wildcard positions.
// Uses intelligent mergeability logic to determine which positions can be wildcarded.
// If the cluster contains heterogeneous TokenLists that can't merge, uses the largest
// mergeable group for pattern generation.
func (c *Cluster) GeneratePattern() *token.TokenList {
	if c.Pattern != nil {
		return c.Pattern
	}

	if len(c.TokenLists) == 0 {
		return nil
	}

	if len(c.TokenLists) == 1 {
		c.Pattern = c.TokenLists[0]
		return c.Pattern
	}

	// Check if cluster is heterogeneous - contains unmergeable sub-groups
	groups := merging.FindMergeableGroups(c.TokenLists)

	// If we have multiple groups, the cluster is heterogeneous
	// Use the largest group for pattern generation
	var primaryGroup []*token.TokenList
	if len(groups) > 1 {
		// Find the largest group
		maxSize := 0
		for _, group := range groups {
			if len(group) > maxSize {
				maxSize = len(group)
				primaryGroup = group
			}
		}
		// TODO: Need to handle semantic mergeability of different patterns in the group
	} else {
		primaryGroup = groups[0]
	}

	// Now generate pattern from the primary group using merging logic
	template := primaryGroup[0]
	if template.Length() == 0 {
		return nil
	}

	// Start with the template
	pattern := template

	// Progressively merge with each TokenList in the group
	for i := 1; i < len(primaryGroup); i++ {
		merged := merging.MergeTokenLists(pattern, primaryGroup[i])
		if merged != nil {
			pattern = merged
		}
		// If merge fails (shouldn't happen since FindMergeableGroups verified it), keep current pattern
	}

	// Build wildcard map and handle special path patterns
	c.WildcardMap = make(map[int]bool)
	patternTokens := make([]token.Token, pattern.Length())

	for i := 0; i < pattern.Length(); i++ {
		tok := pattern.Tokens[i]

		if tok.Wildcard == token.IsWildcard {
			c.WildcardMap[i] = true

			// Special handling for path wildcards
			if tok.Type == token.TokenAbsolutePath && len(primaryGroup) > 0 {
				firstPath := primaryGroup[0].Tokens[i].Value
				tok.Value = getPathPattern(firstPath)
			}
		}

		patternTokens[i] = tok
	}

	c.Pattern = token.NewTokenListWithTokens(patternTokens)
	return c.Pattern
}

// GetWildcardPositions returns wildcard token positions (indices in token array).
func (c *Cluster) GetWildcardPositions() []int {
	if c.Pattern == nil {
		c.GeneratePattern()
	}

	var positions []int
	for pos := range c.WildcardMap {
		positions = append(positions, pos)
	}

	return positions
}

// GetWildcardCharPositions returns character positions where wildcards appear in the pattern string.
// This is used for stateful encoding where the intake needs to know where to insert dynamic values.
func (c *Cluster) GetWildcardCharPositions() []int {
	if c.Pattern == nil {
		c.GeneratePattern()
	}

	var charPositions []int
	currentPos := 0

	for _, tok := range c.Pattern.Tokens {
		// Clean the token value for proper length calculation
		cleaned := sanitizeForTemplate(tok.Value)

		if tok.Wildcard == token.IsWildcard {
			// Record the current character position for this wildcard
			charPositions = append(charPositions, currentPos)
			// Wildcard is represented as "*" (1 character)
			currentPos += 1
		} else if cleaned != "" {
			// Add the length of the cleaned token value
			currentPos += len(cleaned)
		}
	}

	return charPositions
}

// HasWildcards returns true if this cluster contains wildcard positions.
func (c *Cluster) HasWildcards() bool {
	if c.Pattern == nil {
		c.GeneratePattern()
	}

	return len(c.WildcardMap) > 0
}

// GetWildcardValues extracts the actual values from the most recent token list that correspond to wildcard positions
func (c *Cluster) GetWildcardValues() []string {
	if c.Pattern == nil {
		c.GeneratePattern()
	}

	// Get the most recent token list
	if len(c.TokenLists) == 0 {
		return nil
	}
	lastTokenList := c.TokenLists[len(c.TokenLists)-1]

	// Extract values at wildcard positions
	var values []string
	for i, tok := range c.Pattern.Tokens {
		if tok.Wildcard == token.IsWildcard {
			if i < len(lastTokenList.Tokens) {
				values = append(values, lastTokenList.Tokens[i].Value)
			}
		}
	}

	return values
}

// ExtractWildcardValues extracts the wildcard values from a specific TokenList
func (c *Cluster) ExtractWildcardValues(tokenList *token.TokenList) []string {
	if c.Pattern == nil {
		c.GeneratePattern()
	}

	if len(c.WildcardMap) == 0 {
		return []string{}
	}

	var wildcardValues []string
	for i := 0; i < tokenList.Length(); i++ {
		if c.WildcardMap[i] {
			wildcardValues = append(wildcardValues, tokenList.Tokens[i].Value)
		}
	}

	return wildcardValues
}

// GetPatternString returns a string representation of the pattern
func (c *Cluster) GetPatternString() string {
	if c.Pattern == nil {
		c.GeneratePattern()
	}

	if c.Pattern == nil {
		return ""
	}

	var parts []string
	for _, tok := range c.Pattern.Tokens {
		// Use "*" for wildcard positions, actual value otherwise
		if tok.Wildcard == token.IsWildcard {
			parts = append(parts, "*")
		} else {
			// Only use printable ASCII/UTF-8 characters in the template
			cleaned := sanitizeForTemplate(tok.Value)
			if cleaned != "" {
				parts = append(parts, cleaned)
			}
		}
	}
	return strings.Join(parts, "")
}

// sanitizeForTemplate removes non-printable characters from template strings
func sanitizeForTemplate(s string) string {
	runes := []rune(s)
	result := make([]rune, 0, len(runes))
	for _, r := range runes {
		// Keep only printable characters (space and above, excluding DEL)
		if r >= ' ' && r != 0x7F && r < 0xFFFD {
			result = append(result, r)
		}
	}
	return string(result)
}

// GetPatternID returns the pattern ID for this cluster
func (c *Cluster) GetPatternID() uint64 {
	return c.PatternID
}

// SetPatternID sets the pattern ID for this cluster
func (c *Cluster) SetPatternID(id uint64) {
	c.PatternID = id
}

// MarkAsSent updates the LastSentAt timestamp to indicate this pattern was sent to gRPC
func (c *Cluster) MarkAsSent() {
	c.LastSentAt = time.Now()
}

// NeedsSending returns true if this pattern has never been sent or has been updated since last sent
func (c *Cluster) NeedsSending() bool {
	return c.LastSentAt.IsZero() || c.UpdatedAt.After(c.LastSentAt)
}

// IsNewPattern returns true if this pattern has never been sent
func (c *Cluster) IsNewPattern() bool {
	return c.LastSentAt.IsZero()
}

// WasUpdatedSinceLastSent returns true if pattern was updated since last sent
func (c *Cluster) WasUpdatedSinceLastSent() bool {
	return !c.LastSentAt.IsZero() && c.UpdatedAt.After(c.LastSentAt)
}

// MergeTokensIfFits attempts to merge this cluster with another cluster.
// This is used for batch consolidation where clusters with the same signature
// might be further consolidated based on semantic mergeability.
func (c *Cluster) MergeTokensIfFits(other *Cluster) bool {
	// Check if clusters have the same structure
	if c.Signature.Position != other.Signature.Position || c.Signature.Length != other.Signature.Length {
		return false
	}

	// Check if tokens can be merged at each position
	if len(c.TokenLists) == 0 || len(other.TokenLists) == 0 {
		return false
	}

	// Use the first TokenList from each cluster for comparison
	tokenList1 := c.TokenLists[0]
	tokenList2 := other.TokenLists[0]

	// Delegate to merging package for semantic mergeability check
	if !merging.CanMergeTokenLists(tokenList1, tokenList2) {
		return false
	}

	// Merge is possible - add other cluster's TokenLists to this cluster
	c.TokenLists = append(c.TokenLists, other.TokenLists...)

	// Invalidate pattern cache since cluster has changed
	c.Pattern = nil
	c.WildcardMap = make(map[int]bool)
	c.UpdatedAt = time.Now()

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
