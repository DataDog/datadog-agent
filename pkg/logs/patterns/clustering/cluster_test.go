// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	cluster := NewCluster(signature, true, 0, 0, 0, 0)

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
	cluster := NewCluster(signature1, true, 0, 0, 0, 0)
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
	cluster := NewCluster(signature, true, 0, 0, 0, 0)
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

	cluster := NewCluster(signature, true, 0, 0, 0, 0)

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
	pattern1Size := int(cluster.Patterns[0].GetFrequency())
	pattern2Size := int(cluster.Patterns[1].GetFrequency())

	// One pattern should have 2 token lists, the other should have 1
	validSizes := (pattern1Size == 2 && pattern2Size == 1) || (pattern1Size == 1 && pattern2Size == 2)
	assert.True(t, validSizes, "Expected pattern sizes [2, 1], got [%d, %d]", pattern1Size, pattern2Size)

	t.Logf("✅ Multi-pattern cluster created: %d patterns", len(cluster.Patterns))
	t.Logf("   Pattern 1: %d token lists", int(cluster.Patterns[0].GetFrequency()))
	t.Logf("   Pattern 2: %d token lists", int(cluster.Patterns[1].GetFrequency()))
}

func TestCluster_FindMatchingPattern(t *testing.T) {
	signature := token.Signature{
		Position: "Error|Word|Whitespace|Word",
		Length:   4,
		Hash:     5678,
	}

	cluster := NewCluster(signature, true, 0, 0, 0, 0)

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
	pattern1, _, _ := cluster.AddTokenListToPatterns(tokenList1, cm)
	pattern2, _, _ := cluster.AddTokenListToPatterns(tokenList2, cm)

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

	cluster := NewCluster(signature, true, 0, 0, 0, 0)
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

	cluster := NewCluster(signature, true, 0, 0, 0, 0)
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

	cluster := NewCluster(signature, true, 0, 0, 0, 0)

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

	cluster := NewCluster(signature, true, 0, 0, 0, 0)
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
// Saturation Scoring Tests
// =============================================================================

func TestCluster_Saturation_BecomeSaturatedAfterThreshold(t *testing.T) {
	threshold := 5
	cm := NewClusterManagerWithConfig(true, 0, threshold, 0, 0)

	// Seed: first log creates the pattern
	seed := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn1", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	pattern, changeType, _, _ := cm.Add(seed)
	assert.Equal(t, PatternNew, changeType)
	assert.False(t, pattern.saturated)
	assert.Equal(t, 0, pattern.consecutiveIdenticalMerges)

	// Second log: creates wildcards (template structurally changes)
	log2 := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn2", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	pattern, changeType, _, _ = cm.Add(log2)
	assert.Equal(t, PatternUpdated, changeType)
	assert.False(t, pattern.saturated)
	assert.Equal(t, 0, pattern.consecutiveIdenticalMerges, "structural change resets counter")

	// Logs 3 through 3+threshold-1: template is now converged (last token is wildcard),
	// so TryMerge returns tl1 unchanged. Counter should increment each time.
	for i := 0; i < threshold-1; i++ {
		log := token.NewTokenListWithTokens([]token.Token{
			{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "connX", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		})
		pattern, _, _, _ = cm.Add(log)
		assert.Equal(t, i+1, pattern.consecutiveIdenticalMerges, "counter should be %d after %d identical merges", i+1, i+1)
		assert.False(t, pattern.saturated, "should not be saturated yet at count %d (threshold=%d)", i+1, threshold)
	}

	// One more identical merge should trigger saturation
	log := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "connFinal", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	pattern, _, _, _ = cm.Add(log)
	assert.Equal(t, threshold, pattern.consecutiveIdenticalMerges)
	assert.True(t, pattern.saturated, "should be saturated after %d consecutive identical merges", threshold)
}

func TestCluster_Saturation_ResetOnStructuralChange(t *testing.T) {
	threshold := 3
	cm := NewClusterManagerWithConfig(true, 0, threshold, 0, 0)

	// Seed + second log to create wildcards at position 2
	seed := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn1", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord, Wildcard: token.NotWildcard},
	})
	cm.Add(seed)

	log2 := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn2", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord, Wildcard: token.NotWildcard},
	})
	cm.Add(log2)

	// Saturate the pattern (threshold=3 identical merges)
	for i := 0; i < threshold; i++ {
		log := token.NewTokenListWithTokens([]token.Token{
			{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "connX", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "failed", Type: token.TokenWord, Wildcard: token.NotWildcard},
		})
		cm.Add(log)
	}

	patterns := getAllPatterns(cm)
	assert.Len(t, patterns, 1)
	assert.True(t, patterns[0].saturated, "pattern should be saturated")

	// Now send a log that causes a structural change (new wildcard at position 4)
	structuralChange := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn99", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "timeout", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	pattern, _, _, _ := cm.Add(structuralChange)
	assert.False(t, pattern.saturated, "structural change should reset saturation")
	assert.Equal(t, 0, pattern.consecutiveIdenticalMerges, "structural change should reset counter")
}

func TestCluster_Saturation_DesaturateOnUnexpectedMiss(t *testing.T) {
	threshold := 3

	// Create cluster directly to control saturation state
	sig := token.Signature{Position: "Word|Whitespace|Word", Length: 3, Hash: 7777}
	cluster := NewCluster(sig, true, 0, threshold, 0, 0)
	cm := NewClusterManager()

	// Seed pattern
	seed := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn1", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	cluster.AddTokenListToPatterns(seed, cm)

	// Create wildcard at position 2
	log2 := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn2", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	cluster.AddTokenListToPatterns(log2, cm)

	// Saturate
	for i := 0; i < threshold; i++ {
		log := token.NewTokenListWithTokens([]token.Token{
			{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "connX", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		})
		cluster.AddTokenListToPatterns(log, cm)
	}
	assert.True(t, cluster.Patterns[0].saturated)
	assert.Equal(t, cluster.lastMatchedPattern, cluster.Patterns[0])

	// Send a structurally incompatible log (different first word — protected).
	// TryMerge will return nil → desaturate → fall through → create new pattern.
	incompatible := token.NewTokenListWithTokens([]token.Token{
		{Value: "Warn", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "slow", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	cluster.AddTokenListToPatterns(incompatible, cm)

	// Original pattern should be desaturated
	assert.False(t, cluster.Patterns[0].saturated, "should desaturate on unexpected miss")
	assert.Equal(t, 0, cluster.Patterns[0].consecutiveIdenticalMerges, "counter should reset on miss")
	// A new pattern should have been created
	assert.Len(t, cluster.Patterns, 2, "incompatible log should create new pattern")
}

func TestCluster_Saturation_DisabledWhenThresholdZero(t *testing.T) {
	cm := NewClusterManagerWithConfig(true, 0, 0, 0, 0) // threshold=0 disables saturation

	seed := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn1", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	cm.Add(seed)

	// Create wildcard
	log2 := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn2", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	cm.Add(log2)

	// Send 100 identical-structure logs — should never saturate
	for i := 0; i < 100; i++ {
		log := token.NewTokenListWithTokens([]token.Token{
			{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "connX", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		})
		cm.Add(log)
	}

	patterns := getAllPatterns(cm)
	assert.Len(t, patterns, 1)
	assert.False(t, patterns[0].saturated, "should never saturate when threshold=0")
	assert.Equal(t, 0, patterns[0].consecutiveIdenticalMerges, "counter should stay 0 when disabled")
}

func TestCluster_Saturation_SkipsPositionsRebuildOnIdenticalMerge(t *testing.T) {
	threshold := 2
	cm := NewClusterManagerWithConfig(true, 0, threshold, 0, 0)

	// Seed + second log to create wildcard at position 2
	seed := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn1", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	pattern, _, _, _ := cm.Add(seed)
	assert.Empty(t, pattern.Positions, "no wildcards after first log")

	log2 := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn2", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	pattern, _, _, _ = cm.Add(log2)
	assert.Equal(t, []int{2}, pattern.Positions, "wildcard at position 2 after second log")

	// Capture the Positions slice header (pointer) before identical merges
	positionsBefore := pattern.Positions

	// Send identical-structure logs — Positions should not be rebuilt
	for i := 0; i < threshold+5; i++ {
		log := token.NewTokenListWithTokens([]token.Token{
			{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "connX", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		})
		pattern, _, _, _ = cm.Add(log)
	}

	// Positions content should be unchanged
	assert.Equal(t, []int{2}, pattern.Positions)
	// The slice should be the exact same backing array (not rebuilt)
	assert.True(t, &positionsBefore[0] == &pattern.Positions[0],
		"Positions slice should not be rebuilt on identical merges")
}

func TestCluster_Saturation_SaturatedPatternStillMergesCorrectly(t *testing.T) {
	threshold := 3
	cm := NewClusterManagerWithConfig(true, 0, threshold, 0, 0)

	// Build and saturate a pattern
	seed := token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn1", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	})
	cm.Add(seed)
	cm.Add(token.NewTokenListWithTokens([]token.Token{
		{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "conn2", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
	}))
	for i := 0; i < threshold; i++ {
		cm.Add(token.NewTokenListWithTokens([]token.Token{
			{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "connX", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}))
	}

	patterns := getAllPatterns(cm)
	assert.Len(t, patterns, 1)
	assert.True(t, patterns[0].saturated)
	logCountBefore := patterns[0].LogCount

	// Send more logs after saturation — should still merge correctly
	for i := 0; i < 10; i++ {
		pattern, changeType, _, _ := cm.Add(token.NewTokenListWithTokens([]token.Token{
			{Value: "Error", Type: token.TokenWord, Wildcard: token.NotWildcard},
			{Value: " ", Type: token.TokenWhitespace},
			{Value: "connPost", Type: token.TokenWord, Wildcard: token.PotentialWildcard},
		}))
		assert.Equal(t, PatternNoChange, changeType, "saturated pattern should still return PatternNoChange")
		assert.Equal(t, patterns[0].PatternID, pattern.PatternID, "should merge into same pattern")
	}

	assert.Equal(t, logCountBefore+10, patterns[0].LogCount, "log count should increase by 10")
	assert.True(t, patterns[0].saturated, "should remain saturated")
	assert.Equal(t, 1, cm.PatternCount(), "no new patterns should be created")
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
		if p.Sample != nil && merging.CanMergeTokenLists(tokenList, p.Sample, true) {
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

// =============================================================================
// Scan Budget Tests
// =============================================================================

func makeWord(v string) token.Token {
	return token.Token{Value: v, Type: token.TokenWord, Wildcard: token.PotentialWildcard}
}

func makeSpace() token.Token {
	return token.Token{Value: " ", Type: token.TokenWhitespace}
}

// TestScanBudgetEnforced verifies that the scan loop stops after scanBudget patterns.
// With budget=2 and 4 patterns, a value that only matches pattern #3 is NOT found,
// causing a new pattern to be created instead.
func TestScanBudgetEnforced(t *testing.T) {
	sig := token.NewSignature(token.NewTokenListWithTokens([]token.Token{
		makeWord("A"), makeSpace(), makeWord("B"),
	}))
	cm := NewClusterManager()
	// first_word_protection=true keeps distinct first-word seeds as separate patterns
	cluster := NewCluster(sig, true, 0, 0, 0, 2) // scanBudget=2

	seeds := []string{"alpha", "beta", "gamma", "delta"}
	for _, w := range seeds {
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
			makeWord(w), makeSpace(), makeWord("B"),
		}), cm)
	}
	require.Equal(t, 4, len(cluster.Patterns), "need 4 distinct seed patterns")

	// Reset cache and send "delta B" again — "delta" is at index 3, beyond budget=2.
	// Budget skips it, so a new pattern is created (or it falls through to force-widen
	// if maxPatterns is set; here maxPatterns=0 so a 5th pattern is created).
	cluster.lastMatchedPattern = nil
	patternsBefore := len(cluster.Patterns)
	deltaVal := cluster.Patterns[3].Sample.Tokens[0].Value

	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
		makeWord(deltaVal), makeSpace(), makeWord("B"),
	}), cm)

	// Budget=2 means indices 0 and 1 are scanned. Index 3 is not reached.
	// "delta" doesn't match patterns at 0/1, so a new pattern is created.
	assert.Equal(t, patternsBefore+1, len(cluster.Patterns), "budget should cause miss → new pattern")
}

// TestScanBudgetMoveToFront verifies that a scan hit within budget moves the matched
// pattern to index 1, ensuring it's found on the next call without scanning further.
func TestScanBudgetMoveToFront(t *testing.T) {
	sig := token.NewSignature(token.NewTokenListWithTokens([]token.Token{
		makeWord("X"), makeSpace(), makeWord("Y"),
	}))
	cm := NewClusterManager()
	// first_word_protection=true keeps distinct first-word seeds as separate patterns
	cluster := NewCluster(sig, true, 0, 0, 0, 5) // scanBudget=5 — large enough to scan all 3

	// Create 3 patterns; protection=true keeps them separate
	for _, w := range []string{"p0", "p1", "p2"} {
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
			makeWord(w), makeSpace(), makeWord("Y"),
		}), cm)
	}
	require.Equal(t, 3, len(cluster.Patterns))

	// "p2" is at index 2. Reset cache to force the full-scan loop.
	cluster.lastMatchedPattern = nil
	p2 := cluster.Patterns[2]
	p2Val := p2.Sample.Tokens[0].Value

	// Send "p2 Y" — should scan to index 2, find it, move it to index 1.
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
		makeWord(p2Val), makeSpace(), makeWord("Y"),
	}), cm)

	// p2 should now be at index 1 (moved from 2)
	require.Equal(t, 3, len(cluster.Patterns), "no new pattern should be created")
	assert.Same(t, p2, cluster.Patterns[1], "matched pattern should be at index 1 after move-to-front")
}

// =============================================================================
// Max Patterns Per Cluster Tests
// =============================================================================

// TestMaxPatternsCapEnforced verifies that once a cluster reaches maxPatternsPerCluster,
// new unmatched messages are force-widened into an existing pattern rather than creating
// a new one.
func TestMaxPatternsCapEnforced(t *testing.T) {
	sig := token.NewSignature(token.NewTokenListWithTokens([]token.Token{
		makeWord("svc"), makeSpace(), makeWord("val"),
	}))
	cm := NewClusterManager()
	// first_word_protection=true prevents distinct first-words from merging,
	// ensuring we actually reach the cap.
	cluster := NewCluster(sig, true, 0, 0, 3, 0) // maxPatternsPerCluster=3

	// Add 3 patterns; protection=true keeps them separate
	for _, w := range []string{"aaa", "bbb", "ccc"} {
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
			makeWord(w), makeSpace(), makeWord(w),
		}), cm)
	}
	assert.Equal(t, 3, len(cluster.Patterns), "should have exactly 3 patterns at cap")

	// This 4th input would normally create a new pattern, but we're at cap.
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
		makeWord("ddd"), makeSpace(), makeWord("ddd"),
	}), cm)

	assert.Equal(t, 3, len(cluster.Patterns), "should still have 3 patterns — cap enforced")

	// At least one pattern should now have a wildcard (force-widened)
	widened := false
	for _, p := range cluster.Patterns {
		if len(p.Positions) > 0 {
			widened = true
			break
		}
	}
	assert.True(t, widened, "at least one pattern should be widened (has wildcards)")
}

// TestMaxPatternsCapCorrectness verifies that force-widened patterns still match
// subsequent messages of the same shape.
func TestMaxPatternsCapCorrectness(t *testing.T) {
	sig := token.NewSignature(token.NewTokenListWithTokens([]token.Token{
		makeWord("svc"), makeSpace(), makeWord("val"),
	}))
	cm := NewClusterManager()
	// first_word_protection=true keeps seeds separate so we reach the cap.
	cluster := NewCluster(sig, true, 0, 0, 2, 0) // maxPatternsPerCluster=2

	// Fill to cap — protection=true ensures these stay as two distinct patterns
	for _, w := range []string{"aaa", "bbb"} {
		cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
			makeWord(w), makeSpace(), makeWord(w),
		}), cm)
	}
	require.Equal(t, 2, len(cluster.Patterns), "need exactly 2 patterns at cap for this test")

	// Force-widen with a new value
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
		makeWord("ccc"), makeSpace(), makeWord("ccc"),
	}), cm)
	assert.Equal(t, 2, len(cluster.Patterns), "cap still enforced after force-widen")

	// A subsequent message should match the widened pattern (not create another)
	initialCount := len(cluster.Patterns)
	cluster.AddTokenListToPatterns(token.NewTokenListWithTokens([]token.Token{
		makeWord("eee"), makeSpace(), makeWord("eee"),
	}), cm)
	assert.Equal(t, initialCount, len(cluster.Patterns), "subsequent message should not create new pattern")
}
