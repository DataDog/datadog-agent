// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists
// and identifying wildcard positions for pattern extraction.
package clustering

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Cluster represents a group of TokenLists with identical signatures.
type Cluster struct {
	Signature   token.Signature
	TokenLists  []*token.TokenList
	Pattern     *token.TokenList
	WildcardMap map[int]bool
	PatternID   uint64
}

// NewCluster creates a new cluster.
func NewCluster(signature token.Signature, tokenList *token.TokenList) *Cluster {
	return &Cluster{
		Signature:   signature,
		TokenLists:  []*token.TokenList{tokenList},
		Pattern:     nil,
		WildcardMap: make(map[int]bool),
		PatternID:   0, // Will be assigned when pattern is generated
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

	return true
}

// Size returns the number of TokenLists in this cluster.
func (c *Cluster) Size() int {
	return len(c.TokenLists)
}

// GeneratePattern analyzes all TokenLists in the cluster to identify wildcard positions.
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

	template := c.TokenLists[0]
	if template.Length() == 0 {
		return nil
	}

	patternTokens := make([]token.Token, template.Length())

	for i := 0; i < template.Length(); i++ {
		firstValue := template.Tokens[i].Value
		firstType := template.Tokens[i].Type
		allSame := true

		for j := 1; j < len(c.TokenLists); j++ {
			if i >= c.TokenLists[j].Length() {
				allSame = false
				break
			}

			if c.TokenLists[j].Tokens[i].Value != firstValue {
				allSame = false
				break
			}
		}

		if allSame {
			patternTokens[i] = template.Tokens[i]
		} else {
			wildcardValue := "*"

			if firstType == token.TokenAbsolutePath {
				wildcardValue = getPathPattern(firstValue)
			}

			patternTokens[i] = token.Token{
				Value:      wildcardValue,
				Type:       firstType,
				IsWildcard: true,
			}
			c.WildcardMap[i] = true
		}
	}

	c.Pattern = token.NewTokenListWithTokens(patternTokens)
	return c.Pattern
}

// GetWildcardPositions returns wildcard positions.
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

// HasWildcards returns true if this cluster contains wildcard positions.
func (c *Cluster) HasWildcards() bool {
	if c.Pattern == nil {
		c.GeneratePattern()
	}

	return len(c.WildcardMap) > 0
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
		parts = append(parts, tok.Value)
	}
	return strings.Join(parts, "")
}

// GetPatternID returns the pattern ID for this cluster
func (c *Cluster) GetPatternID() uint64 {
	return c.PatternID
}

// SetPatternID sets the pattern ID for this cluster
func (c *Cluster) SetPatternID(id uint64) {
	c.PatternID = id
}

// MergeTokensIfFits attempts to merge this cluster with another cluster
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

	if tokenList1.Length() != tokenList2.Length() {
		return false
	}

	// Check mergeability at each token position
	for i := 0; i < tokenList1.Length(); i++ {
		level := tokenList1.Tokens[i].GetMergeabilityLevel(&tokenList2.Tokens[i])
		if !level.IsMergeable() {
			return false
		}
	}

	// Merge is possible - add other cluster's TokenLists to this cluster
	c.TokenLists = append(c.TokenLists, other.TokenLists...)

	// Invalidate pattern cache since cluster has changed
	c.Pattern = nil
	c.WildcardMap = make(map[int]bool)

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
