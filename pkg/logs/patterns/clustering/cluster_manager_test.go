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

// Test-only helper functions

// getCluster retrieves the cluster with the given signature.
func getCluster(cm *ClusterManager, signature token.Signature) *Cluster {
	hash := signature.Hash

	cm.mu.RLock()
	defer cm.mu.RUnlock()

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

// getAllPatterns returns all patterns across all clusters.
func getAllPatterns(cm *ClusterManager) []*Pattern {
	var allPatterns []*Pattern

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Iterate through all clusters in all hash buckets
	for _, clusters := range cm.hashBuckets {
		for _, cluster := range clusters {
			// Collect all patterns from this cluster
			allPatterns = append(allPatterns, cluster.Patterns...)
		}
	}

	return allPatterns
}

func TestClusterManager_NewClusterManager(t *testing.T) {
	cm := NewClusterManager()

	assert.NotNil(t, cm, "ClusterManager should not be nil")
	assert.Equal(t, 0, cm.PatternCount(), "New ClusterManager should have 0 patterns")
	assert.Equal(t, int64(0), cm.EstimatedBytes(), "New ClusterManager should have 0 estimated bytes")
}

func TestClusterManager_Add_NewCluster(t *testing.T) {
	cm := NewClusterManager()

	// Create TokenList
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenListWithTokens(tokens)

	pattern, changeType, _, _ := cm.Add(tokenList)

	assert.NotNil(t, pattern, "Should return a pattern")
	assert.Equal(t, 1, pattern.LogCount, "Pattern should have log count 1")
	assert.Equal(t, PatternNew, changeType, "Expected PatternNew for first add")
	assert.Equal(t, 1, cm.PatternCount(), "Should track 1 pattern after first add")
	assert.Greater(t, cm.EstimatedBytes(), int64(0), "Estimated bytes should be > 0 after first add")
}

func TestClusterManager_Add_ExistingCluster(t *testing.T) {
	cm := NewClusterManager()

	// Create two TokenLists with same signature
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHTTPMethod}, // Different value, same type
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath}, // Different value, same type
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	pattern1, changeType1, _, _ := cm.Add(tokenList1)
	pattern2, changeType2, _, _ := cm.Add(tokenList2)

	// Should be the same pattern (same cluster, merged together)
	assert.Equal(t, pattern1.PatternID, pattern2.PatternID, "TokenLists with same signature should merge into same pattern")
	assert.Equal(t, 2, pattern2.LogCount, "Pattern should have log count 2")
	assert.Equal(t, PatternNew, changeType1, "Expected PatternNew for first add")

	// With eager pattern generation, adding the second token list creates wildcards (pattern update)
	assert.Equal(t, PatternUpdated, changeType2, "Expected PatternUpdated for second add (creates wildcards)")
	assert.Equal(t, 1, cm.PatternCount(), "Merging into existing pattern should not increase pattern count")
}

func TestClusterManager_Add_DifferentSignatures(t *testing.T) {
	cm := NewClusterManager()

	// Create TokenLists with different signatures
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel}, // Different type
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord}, // Different type
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	pattern1, _, _, _ := cm.Add(tokenList1)
	pattern2, _, _, _ := cm.Add(tokenList2)

	// Should be different patterns (different clusters)
	assert.NotEqual(t, pattern1.PatternID, pattern2.PatternID, "TokenLists with different signatures should create different patterns")
	assert.Equal(t, 2, cm.PatternCount(), "Different signatures should increase pattern count")
}

func TestClusterManager_GetCluster(t *testing.T) {
	cm := NewClusterManager()

	// Create and add TokenList
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenListWithTokens(tokens)
	signature := token.NewSignature(tokenList)

	addedPattern, _, _, _ := cm.Add(tokenList)

	// Retrieve cluster by signature
	retrievedCluster := getCluster(cm, signature)

	assert.NotNil(t, retrievedCluster, "Should retrieve cluster by signature")
	assert.Equal(t, 1, len(retrievedCluster.Patterns), "Cluster should have 1 pattern")
	assert.Equal(t, addedPattern.PatternID, retrievedCluster.Patterns[0].PatternID, "Pattern IDs should match")

	// Try to get non-existent cluster
	differentTokens := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	differentTokenList := token.NewTokenListWithTokens(differentTokens)
	differentSignature := token.NewSignature(differentTokenList)

	nonExistentCluster := getCluster(cm, differentSignature)
	assert.Nil(t, nonExistentCluster, "Should return nil for non-existent cluster")
}

func TestClusterManager_Clear(t *testing.T) {
	cm := NewClusterManager()

	// Add some data
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenListWithTokens(tokens)
	signature := token.NewSignature(tokenList)

	cm.Add(tokenList)
	assert.Equal(t, 1, cm.PatternCount(), "Should track 1 pattern before clear")

	// Verify data exists
	assert.NotNil(t, getCluster(cm, signature), "Should have cluster before clear")

	// Clear
	cm.Clear()

	// Verify data is gone
	assert.Nil(t, getCluster(cm, signature), "Should have no cluster after clear")
	assert.Equal(t, 0, cm.PatternCount(), "Should reset pattern count on clear")
	assert.Equal(t, int64(0), cm.EstimatedBytes(), "Should reset estimated bytes on clear")
}

func TestClusterManager_GetAllPatterns(t *testing.T) {
	cm := NewClusterManager()

	// Initially empty
	patterns := getAllPatterns(cm)
	assert.Equal(t, 0, len(patterns), "Should have no patterns initially")

	// Add pattern 1 (signature 1)
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	pattern1, _, _, _ := cm.Add(token.NewTokenListWithTokens(tokens1))

	// Add pattern 2 (same signature, should merge into pattern 1)
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath},
	}
	pattern2, _, _, _ := cm.Add(token.NewTokenListWithTokens(tokens2))

	// Add pattern 3 (different signature)
	tokens3 := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	pattern3, _, _, _ := cm.Add(token.NewTokenListWithTokens(tokens3))

	// Get all patterns
	allPatterns := getAllPatterns(cm)

	// Should have 2 patterns: pattern1 (merged with pattern2) and pattern3
	assert.Equal(t, 2, len(allPatterns), "Should have 2 patterns total")

	// Verify we have both pattern IDs
	patternIDs := make(map[uint64]bool)
	for _, p := range allPatterns {
		patternIDs[p.PatternID] = true
	}
	assert.True(t, patternIDs[pattern1.PatternID], "Should include pattern 1")
	assert.True(t, patternIDs[pattern3.PatternID], "Should include pattern 3")
	assert.Equal(t, pattern1.PatternID, pattern2.PatternID, "Pattern 1 and 2 should be the same (merged)")
}

func TestClusterManager_PatternChangeType(t *testing.T) {
	cm := NewClusterManager()

	// Create token lists with same signature (HTTP method, space, path)
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/users", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/orders", Type: token.TokenAbsolutePath},
	}
	tokens3 := []token.Token{
		{Value: "PUT", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/items", Type: token.TokenAbsolutePath},
	}
	tokens4 := []token.Token{
		{Value: "DELETE", Type: token.TokenHTTPMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/products", Type: token.TokenAbsolutePath},
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	tokenList3 := token.NewTokenListWithTokens(tokens3)
	tokenList4 := token.NewTokenListWithTokens(tokens4)

	// First add - should create a new pattern
	pattern1, changeType1, _, _ := cm.Add(tokenList1)
	assert.Equal(t, PatternNew, changeType1, "Expected PatternNew for first add")
	t.Logf("✅ Add #1: PatternNew (created pattern with PatternID=%d)", pattern1.PatternID)

	// Second add - same signature, adding to existing pattern creates wildcards (pattern update)
	pattern2, changeType2, _, _ := cm.Add(tokenList2)
	assert.Equal(t, PatternUpdated, changeType2, "Expected PatternUpdated for second add (creates wildcards)")
	assert.Equal(t, pattern1.PatternID, pattern2.PatternID, "Should return same pattern for same signature")
	t.Logf("✅ Add #2: PatternUpdated (wildcards created, logCount=%d)", pattern2.LogCount)
	t.Logf("   Pattern after 2 logs: '%s'", pattern2.GetPatternString())

	// Third add - pattern exists but wildcard count unchanged (still 2 wildcards)
	pattern3, changeType3, _, _ := cm.Add(tokenList3)
	assert.Equal(t, PatternNoChange, changeType3, "Expected PatternNoChange for third add (wildcard count unchanged)")
	assert.Equal(t, pattern1.PatternID, pattern3.PatternID, "Should return same pattern for same signature")
	t.Logf("✅ Add #3: PatternNoChange (wildcard count unchanged, logCount=%d)", pattern3.LogCount)
	t.Logf("   Pattern after 3 logs: '%s'", pattern3.GetPatternString())

	// Fourth add - pattern exists, wildcard count still unchanged
	pattern4, changeType4, _, _ := cm.Add(tokenList4)
	assert.Equal(t, PatternNoChange, changeType4, "Expected PatternNoChange for fourth add (wildcard count unchanged)")
	t.Logf("✅ Add #4: PatternNoChange (wildcard count unchanged, logCount=%d)", pattern4.LogCount)

	// Final pattern (eagerly generated by Add)
	t.Logf("   Final pattern after 4 logs: '%s'", pattern4.GetPatternString())

	// Verify all returned the same pattern
	assert.Equal(t, 4, pattern4.LogCount, "Expected pattern log count 4")
}
