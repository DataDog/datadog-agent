// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

	cluster := NewCluster(signature, tokenList)

	assert.Equal(t, 0, cluster.Size(), "Expected cluster size 0 initially")
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

	cluster := NewCluster(signature1, tokenList1)
	cluster.AddTokenListToPatterns(tokenList1)

	assert.Equal(t, 1, cluster.Size(), "Expected initial cluster size 1")

	// Create second TokenList with same signature but different values
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath},
	}
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	// Add tokenList with matching signature
	cluster.AddTokenListToPatterns(tokenList2)

	assert.Equal(t, 2, cluster.Size(), "Expected cluster size 2 after adding")
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

	cluster := NewCluster(signature, tokenList)
	cluster.AddTokenListToPatterns(tokenList)

	// Should have exactly one pattern (which is also the primary)
	assert.Equal(t, 1, len(cluster.Patterns), "Should have exactly one pattern")

	mostCommon := cluster.GetMostCommonPattern()
	assert.NotNil(t, mostCommon, "Most common pattern should not be nil")

	pattern := mostCommon.Template
	assert.NotNil(t, pattern, "Pattern template should not be nil")
	assert.False(t, cluster.HasWildcards(), "Single log should not have wildcards")
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

	cluster := NewCluster(signature, nil)

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

	cluster.AddTokenListToPatterns(tokenList1)
	cluster.AddTokenListToPatterns(tokenList2) // Different special char - new pattern
	cluster.AddTokenListToPatterns(tokenList3) // Same special char as tokens1 - same pattern

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

	cluster := NewCluster(signature, nil)

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

	pattern1 := cluster.AddTokenListToPatterns(tokenList1)
	pattern2 := cluster.AddTokenListToPatterns(tokenList2)

	// Should return different patterns
	assert.NotEqual(t, pattern1, pattern2, "Should create different patterns for different whitespace")

	// FindMatchingPattern should return the correct pattern for each token list
	found1 := cluster.FindMatchingPattern(tokenList1)
	found2 := cluster.FindMatchingPattern(tokenList2)

	assert.Equal(t, pattern1, found1, "Should find the first pattern for tokenList1")
	assert.Equal(t, pattern2, found2, "Should find the second pattern for tokenList2")
}

func TestCluster_GetMostCommonPattern(t *testing.T) {
	signature := token.Signature{
		Position: "Word|Whitespace|Word",
		Length:   3,
		Hash:     9999,
	}

	cluster := NewCluster(signature, nil)

	// Add multiple token lists that split into different patterns
	// Pattern 1: 3 logs (should be most common)
	for i := 0; i < 3; i++ {
		tokens := []token.Token{
			{Value: "Service", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "started", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}
		tokenList := token.NewTokenListWithTokens(tokens)
		cluster.AddTokenListToPatterns(tokenList)
	}

	// Pattern 2: 1 log (less common)
	tokens2 := []token.Token{
		{Value: "Service", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: "  ", Type: token.TokenWhitespace}, // Different whitespace
		{Value: "stopped", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	cluster.AddTokenListToPatterns(tokenList2)

	mostCommon := cluster.GetMostCommonPattern()
	assert.NotNil(t, mostCommon, "Most common pattern should not be nil")
	assert.Equal(t, 3, mostCommon.LogCount, "Most common pattern should have 3 logs")
}

func TestCluster_GetAllPatterns(t *testing.T) {
	signature := token.Signature{
		Position: "Word|Whitespace|Numeric",
		Length:   3,
		Hash:     1111,
	}

	cluster := NewCluster(signature, nil)

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

	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens1))
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens2))
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens3))

	allPatterns := cluster.GetAllPatterns()
	assert.Len(t, allPatterns, 3, "Expected 3 patterns")
}

func TestCluster_ExtractWildcardValues_MultiPattern(t *testing.T) {
	signature := token.Signature{
		Position: "Error|Word|Whitespace|Word",
		Length:   4,
		Hash:     2222,
	}

	cluster := NewCluster(signature, nil)

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

	cluster.AddTokenListToPatterns(tokenList1)
	cluster.AddTokenListToPatterns(tokenList2)

	// Both should merge into same pattern
	// Extract wildcard values from tokenList2
	values := cluster.ExtractWildcardValues(tokenList2)

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

	cluster := NewCluster(signature, nil)

	// Add 2 token lists to pattern 1
	for i := 0; i < 2; i++ {
		tokens := []token.Token{
			{Value: "Test", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "passed", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens))
	}

	// Add 3 token lists to pattern 2 (different whitespace)
	for i := 0; i < 3; i++ {
		tokens := []token.Token{
			{Value: "Test", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: "  ", Type: token.TokenWhitespace}, // Different
			{Value: "failed", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens))
	}

	// Total size should be 5 (2 + 3)
	assert.Equal(t, 5, cluster.Size(), "Expected cluster size 5 (2 + 3)")
}

func TestCluster_BackwardCompatibility(t *testing.T) {
	// Test that old API methods still work (GetPatternString, GetWildcardPositions, etc.)
	signature := token.Signature{
		Position: "Word|Whitespace|Word",
		Length:   3,
		Hash:     4444,
	}

	cluster := NewCluster(signature, nil)

	tokens1 := []token.Token{
		{Value: "Service", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "started", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}
	tokens2 := []token.Token{
		{Value: "Service", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "stopped", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}

	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens1))
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens(tokens2))

	patternString := cluster.GetPatternString()
	assert.NotEmpty(t, patternString, "GetPatternString should return a valid pattern string")

	hasWildcards := cluster.HasWildcards()
	assert.True(t, hasWildcards, "Should have wildcards")

	wildcardPositions := cluster.GetWildcardPositions()
	assert.NotEmpty(t, wildcardPositions, "Should have wildcard positions")

	t.Logf("✅ Backward compatibility: pattern='%s', wildcards=%v", patternString, wildcardPositions)
}
