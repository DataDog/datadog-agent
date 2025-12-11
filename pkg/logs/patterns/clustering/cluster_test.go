// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering/merging"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestCluster_NewCluster(t *testing.T) {
	// Create a simple TokenList
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenListWithTokens(tokens)
	signature := token.NewSignature(tokenList)

	cluster := NewCluster(signature)

	assert.Equal(t, 0, clusterSize(cluster), "Expected cluster size 0 initially")
	assert.True(t, cluster.Signature.Equals(signature), "Cluster signature doesn't match expected signature")
	assert.Empty(t, cluster.Patterns, "Patterns should be empty initially (computed lazily)")
}

func TestCluster_AddTokenListToPatterns(t *testing.T) {
	// Create first TokenList
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList1 := token.NewTokenListWithTokens(tokens1)
	signature1 := token.NewSignature(tokenList1)

	cm := NewClusterManager()
	cluster := NewCluster(signature1)
	cluster.AddTokenListToPatterns(tokenList1, cm)

	assert.Equal(t, 1, clusterSize(cluster), "Expected initial cluster size 1")

	// Create second TokenList with same signature but different values
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath},
	}
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	// Add tokenList with matching signature
	cluster.AddTokenListToPatterns(tokenList2, cm)

	assert.Equal(t, 2, clusterSize(cluster), "Expected cluster size 2 after adding")
	assert.NotEmpty(t, cluster.Patterns, "Expected patterns to exist after adding TokenLists")
}

func TestCluster_SinglePattern_SingleLog(t *testing.T) {
	// When a cluster has only one log, it creates one pattern with no wildcards
	tokens := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	tokenList := token.NewTokenListWithTokens(tokens)
	signature := token.NewSignature(tokenList)

	cm := NewClusterManager()
	cluster := NewCluster(signature)
	cluster.AddTokenListToPatterns(tokenList, cm)

	// Should have exactly one pattern (which is also the primary)
	assert.Equal(t, 1, len(cluster.Patterns), "Should have exactly one pattern")

	mostCommon := getMostCommonPattern(cluster)
	assert.NotNil(t, mostCommon, "Most common pattern should not be nil")

	pattern := mostCommon.Template
	assert.NotNil(t, pattern, "Pattern template should not be nil")
	assert.False(t, hasWildcards(cluster), "Single log should not have wildcards")
	assert.Equal(t, tokenList.Length(), pattern.Length(), "Pattern length should match original TokenList")

	for i, tok := range pattern.Tokens {
		assert.Equal(t, tokenList.Tokens[i].Value, tok.Value,
			"Pattern token %d value mismatch", i)
	}
}

func TestCluster_MultiplePatterns_SpecialCharVariation(t *testing.T) {
	// This is the key test for multi-pattern clusters!
	// TokenLists with same signature but different special characters should create multiple patterns
	// Note: Whitespace variations now merge (normalized to single space)

	signature := token.Signature{
		Position: "Error|Word|Whitespace|Word|Word|Word",
		Length:   6,
		Hash:     1234,
	}

	cluster := NewCluster(signature)

	// Create TokenLists with different special characters (cannot merge - structural difference)
	tokens1 := []token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard}, // Protected first word
		{Value: ":", Type: token.TokenWord, Wildcard: token.NotWildcard},     // Colon
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "connection", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}
	tokens2 := []token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: ";", Type: token.TokenWord, Wildcard: token.NotWildcard}, // Semicolon - DIFFERENT!
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "connection", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "timeout", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}
	tokens3 := []token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: ":", Type: token.TokenWord, Wildcard: token.NotWildcard}, // Colon - matches tokens1
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "database", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "error", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	tokenList3 := token.NewTokenListWithTokens(tokens3)

	cm := NewClusterManager()
	cluster.AddTokenListToPatterns(tokenList1, cm)
	cluster.AddTokenListToPatterns(tokenList2, cm) // Different special char - new pattern
	cluster.AddTokenListToPatterns(tokenList3, cm) // Same special char as tokens1 - same pattern

	// Should have 2 patterns (one for colon, one for semicolon)
	assert.Len(t, cluster.Patterns, 2, "Expected 2 patterns due to special character variation")

	// Verify pattern sizes
	pattern1Size := cluster.Patterns[0].size()
	pattern2Size := cluster.Patterns[1].size()

	// One pattern should have 2 token lists, the other should have 1
	validSizes := (pattern1Size == 2 && pattern2Size == 1) || (pattern1Size == 1 && pattern2Size == 2)
	assert.True(t, validSizes, "Expected pattern sizes [2, 1], got [%d, %d]", pattern1Size, pattern2Size)

	t.Logf("✅ Multi-pattern cluster created: %d patterns", len(cluster.Patterns))
	t.Logf("   Pattern 1: %d token lists", cluster.Patterns[0].size())
	t.Logf("   Pattern 2: %d token lists", cluster.Patterns[1].size())
}

func TestCluster_FindMatchingPattern(t *testing.T) {
	signature := token.Signature{
		Position: "Error|Word|Whitespace|Word",
		Length:   4,
		Hash:     5678,
	}

	cluster := NewCluster(signature)

	tokens1 := []token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: ":", Type: token.TokenWord},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}
	tokens2 := []token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: ":", Type: token.TokenWord},
		{Value: "  ", Type: token.TokenWhitespace}, // Different whitespace
		{Value: "timeout", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	cm := NewClusterManager()
	pattern1 := cluster.AddTokenListToPatterns(tokenList1, cm)
	pattern2 := cluster.AddTokenListToPatterns(tokenList2, cm)

	// Should return different patterns
	assert.NotEqual(t, pattern1, pattern2, "Should create different patterns for different whitespace")

	// findMatchingPattern should return the correct pattern for each token list
	found1 := findMatchingPattern(cluster, tokenList1)
	found2 := findMatchingPattern(cluster, tokenList2)

	assert.Equal(t, pattern1, found1, "Should find the first pattern for tokenList1")
	assert.Equal(t, pattern2, found2, "Should find the second pattern for tokenList2")
}

func TestCluster_GetMostCommonPattern(t *testing.T) {
	signature := token.Signature{
		Position: "Word|Whitespace|Word",
		Length:   3,
		Hash:     9999,
	}

	cluster := NewCluster(signature)
	cm := NewClusterManager()

	// Add multiple token lists that split into different patterns
	// Pattern 1: 3 logs (should be most common)
	for i := 0; i < 3; i++ {
		tokens := []token.Token{
			{Value: "Service", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "started", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}
		tokenList := token.NewTokenListWithTokens(tokens)
		cluster.AddTokenListToPatterns(tokenList, cm)
	}

	// Pattern 2: 1 log (less common)
	tokens2 := []token.Token{
		{Value: "Service", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: "  ", Type: token.TokenWhitespace}, // Different whitespace
		{Value: "stopped", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	cluster.AddTokenListToPatterns(tokenList2, cm)

	mostCommon := getMostCommonPattern(cluster)
	assert.NotNil(t, mostCommon, "Most common pattern should not be nil")
	assert.Equal(t, 3, mostCommon.LogCount, "Most common pattern should have 3 logs")
}

func TestCluster_GetAllPatterns(t *testing.T) {
	signature := token.Signature{
		Position: "Word|Whitespace|Numeric",
		Length:   3,
		Hash:     1111,
	}

	cluster := NewCluster(signature)
	cm := NewClusterManager()

	// Create 3 different patterns via whitespace variation
	tokens1 := []token.Token{
		{Value: "Count", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "42", Type: token.TokenNumeric},
	}
	tokens2 := []token.Token{
		{Value: "Count", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: "  ", Type: token.TokenWhitespace}, // Different
		{Value: "100", Type: token.TokenNumeric},
	}
	tokens3 := []token.Token{
		{Value: "Count", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: "   ", Type: token.TokenWhitespace}, // Different
		{Value: "200", Type: token.TokenNumeric},
	}

	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens1), cm)
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens2), cm)
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens3), cm)

	allPatterns := cluster.Patterns
	assert.Len(t, allPatterns, 3, "Expected 3 patterns")
}

func TestCluster_ExtractWildcardValues_MultiPattern(t *testing.T) {
	signature := token.Signature{
		Position: "Error|Word|Whitespace|Word",
		Length:   4,
		Hash:     2222,
	}

	cluster := NewCluster(signature)

	tokens1 := []token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: ":", Type: token.TokenWord},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "connection", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}
	tokens2 := []token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: ":", Type: token.TokenWord},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "timeout", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	cm := NewClusterManager()
	cluster.AddTokenListToPatterns(tokenList1, cm)
	cluster.AddTokenListToPatterns(tokenList2, cm)

	// Both should merge into same pattern
	// Extract wildcard values from tokenList2
	values := extractWildcardValues(cluster, tokenList2)

	// Should have one wildcard value for the last word token
	assert.Len(t, values, 1, "Expected 1 wildcard value")
	if len(values) > 0 {
		assert.Equal(t, "timeout", values[0], "Expected wildcard value 'timeout'")
	}
}

func TestCluster_Size_MultiPattern(t *testing.T) {
	signature := token.Signature{
		Position: "Word|Whitespace|Word",
		Length:   3,
		Hash:     3333,
	}

	cluster := NewCluster(signature)
	cm := NewClusterManager()

	// Add 2 token lists to pattern 1
	for i := 0; i < 2; i++ {
		tokens := []token.Token{
			{Value: "Test", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "passed", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens), cm)
	}

	// Add 3 token lists to pattern 2 (different whitespace)
	for i := 0; i < 3; i++ {
		tokens := []token.Token{
			{Value: "Test", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: "  ", Type: token.TokenWhitespace}, // Different
			{Value: "failed", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens), cm)
	}

	// Total size should be 5 (2 + 3)
	assert.Equal(t, 5, clusterSize(cluster), "Expected cluster size 5 (2 + 3)")
}

// =============================================================================
// Test Helper Functions
// =============================================================================

// getMostCommonPattern returns the pattern with the highest log count in the cluster.
func getMostCommonPattern(c *Cluster) *Pattern {
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

// getPatternString returns a string representation of the most common pattern.
func getPatternString(c *Cluster) string {
	mostCommon := getMostCommonPattern(c)
	if mostCommon == nil {
		return ""
	}
	return mostCommon.GetPatternString()
}

// getWildcardPositions returns wildcard token positions for the most common pattern.
func getWildcardPositions(c *Cluster) []int {
	mostCommon := getMostCommonPattern(c)
	if mostCommon == nil {
		return nil
	}
	return mostCommon.Positions
}

// hasWildcards returns true if any pattern in this cluster contains wildcard positions.
func hasWildcards(c *Cluster) bool {
	for _, p := range c.Patterns {
		if len(p.Positions) > 0 {
			return true
		}
	}
	return false
}

// extractWildcardValues extracts wildcard values from a TokenList using the matching pattern.
func extractWildcardValues(c *Cluster, tokenList *token.TokenList) []string {
	p := findMatchingPattern(c, tokenList)
	if p == nil {
		return []string{}
	}
	return p.GetWildcardValues(tokenList)
}

// findMatchingPattern finds the Pattern that matches the given TokenList.
func findMatchingPattern(c *Cluster, tokenList *token.TokenList) *Pattern {
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

	// No matching pattern found
	return nil
}

// clusterSize returns the total number of logs across all patterns in the cluster.
func clusterSize(c *Cluster) int {
	total := 0
	for _, p := range c.Patterns {
		total += p.LogCount
	}
	return total
}
