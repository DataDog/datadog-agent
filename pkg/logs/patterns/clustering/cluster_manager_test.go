// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestClusterManager_NewClusterManager(t *testing.T) {
	cm := NewClusterManager()

	if cm == nil {
		t.Fatal("ClusterManager should not be nil")
	}

	stats := cm.GetStats()
	if stats.TotalTokenLists != 0 || stats.TotalClusters != 0 || stats.HashBuckets != 0 {
		t.Error("New ClusterManager should have zero stats")
	}
}

func TestClusterManager_Add_NewCluster(t *testing.T) {
	cm := NewClusterManager()

	// Create TokenList
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenListWithTokens(tokens)

	cluster, changeType := cm.Add(tokenList)

	if cluster == nil {
		t.Fatal("Should return a cluster")
	}

	if cluster.Size() != 1 {
		t.Errorf("Cluster should have size 1, got %d", cluster.Size())
	}

	if changeType != PatternNew {
		t.Errorf("Expected PatternNew for first add, got %v", changeType)
	}

	stats := cm.GetStats()
	if stats.TotalTokenLists != 1 || stats.TotalClusters != 1 {
		t.Errorf("Expected 1 TokenList and 1 cluster, got %d TokenLists and %d clusters",
			stats.TotalTokenLists, stats.TotalClusters)
	}
}

func TestClusterManager_Add_ExistingCluster(t *testing.T) {
	cm := NewClusterManager()

	// Create two TokenLists with same signature
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHttpMethod}, // Different value, same type
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath}, // Different value, same type
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	cluster1, changeType1 := cm.Add(tokenList1)
	cluster2, changeType2 := cm.Add(tokenList2)

	// Should be the same cluster
	if cluster1 != cluster2 {
		t.Error("TokenLists with same signature should go to same cluster")
	}

	if cluster1.Size() != 2 {
		t.Errorf("Cluster should have size 2, got %d", cluster1.Size())
	}

	if changeType1 != PatternNew {
		t.Errorf("Expected PatternNew for first add, got %v", changeType1)
	}

	if changeType2 != PatternNoChange {
		t.Errorf("Expected PatternNoChange for second add to same cluster, got %v", changeType2)
	}

	stats := cm.GetStats()
	if stats.TotalTokenLists != 2 || stats.TotalClusters != 1 {
		t.Errorf("Expected 2 TokenLists and 1 cluster, got %d TokenLists and %d clusters",
			stats.TotalTokenLists, stats.TotalClusters)
	}
}

func TestClusterManager_Add_DifferentSignatures(t *testing.T) {
	cm := NewClusterManager()

	// Create TokenLists with different signatures
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
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

	cluster1, _ := cm.Add(tokenList1)
	cluster2, _ := cm.Add(tokenList2)

	// Should be different clusters
	if cluster1 == cluster2 {
		t.Error("TokenLists with different signatures should go to different clusters")
	}

	stats := cm.GetStats()
	if stats.TotalTokenLists != 2 || stats.TotalClusters != 2 {
		t.Errorf("Expected 2 TokenLists and 2 clusters, got %d TokenLists and %d clusters",
			stats.TotalTokenLists, stats.TotalClusters)
	}
}

func TestClusterManager_GetCluster(t *testing.T) {
	cm := NewClusterManager()

	// Create and add TokenList
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenListWithTokens(tokens)
	signature := token.NewSignature(tokenList)

	addedCluster, _ := cm.Add(tokenList)

	// Retrieve cluster by signature
	retrievedCluster := cm.GetCluster(signature)

	if retrievedCluster != addedCluster {
		t.Error("Retrieved cluster should be the same as added cluster")
	}

	// Try to get non-existent cluster
	differentTokens := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	differentTokenList := token.NewTokenListWithTokens(differentTokens)
	differentSignature := token.NewSignature(differentTokenList)

	nonExistentCluster := cm.GetCluster(differentSignature)
	if nonExistentCluster != nil {
		t.Error("Should return nil for non-existent cluster")
	}
}

func TestClusterManager_GetAllClusters(t *testing.T) {
	cm := NewClusterManager()

	// Add multiple clusters
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	tokens3 := []token.Token{
		{Value: "192.168.1.1", Type: token.TokenIPv4},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "connected", Type: token.TokenWord},
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	tokenList3 := token.NewTokenListWithTokens(tokens3)

	cm.Add(tokenList1)
	cm.Add(tokenList2)
	cm.Add(tokenList3)

	allClusters := cm.GetAllClusters()

	if len(allClusters) != 3 {
		t.Errorf("Expected 3 clusters, got %d", len(allClusters))
	}
}

func TestClusterManager_GetClustersByLength(t *testing.T) {
	cm := NewClusterManager()

	// Add TokenLists of different lengths with different signatures
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
	} // Length 2

	tokens2 := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	} // Length 3

	tokens3 := []token.Token{
		{Value: "192.168.1.1", Type: token.TokenIPv4},
		{Value: " ", Type: token.TokenWhitespace},
	} // Length 2 (different signature than tokens1)

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	tokenList3 := token.NewTokenListWithTokens(tokens3)

	cm.Add(tokenList1)
	cm.Add(tokenList2)
	cm.Add(tokenList3)

	// Get clusters of length 2 - should have 2 different clusters
	length2Clusters := cm.GetClustersByLength(2)
	if len(length2Clusters) != 2 {
		t.Errorf("Expected 2 clusters of length 2, got %d", len(length2Clusters))
	}

	// Get clusters of length 3
	length3Clusters := cm.GetClustersByLength(3)
	if len(length3Clusters) != 1 {
		t.Errorf("Expected 1 cluster of length 3, got %d", len(length3Clusters))
	}

	// Get clusters of non-existent length
	length5Clusters := cm.GetClustersByLength(5)
	if len(length5Clusters) != 0 {
		t.Errorf("Expected 0 clusters of length 5, got %d", len(length5Clusters))
	}
}

func TestClusterManager_GetLargestClusters(t *testing.T) {
	cm := NewClusterManager()

	// Create clusters of different sizes
	// Cluster 1: size 3
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList1a := token.NewTokenListWithTokens(tokens1)
	tokenList1b := token.NewTokenListWithTokens([]token.Token{
		{Value: "POST", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath},
	})
	tokenList1c := token.NewTokenListWithTokens([]token.Token{
		{Value: "PUT", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/items", Type: token.TokenAbsolutePath},
	})

	// Cluster 2: size 1
	tokens2 := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	tokenList2 := token.NewTokenListWithTokens(tokens2)

	// Cluster 3: size 2
	tokens3 := []token.Token{
		{Value: "192.168.1.1", Type: token.TokenIPv4},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "connected", Type: token.TokenWord},
	}
	tokenList3a := token.NewTokenListWithTokens(tokens3)
	tokenList3b := token.NewTokenListWithTokens([]token.Token{
		{Value: "10.0.0.1", Type: token.TokenIPv4},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "disconnected", Type: token.TokenWord},
	})

	cm.Add(tokenList1a)
	cm.Add(tokenList1b)
	cm.Add(tokenList1c)
	cm.Add(tokenList2)
	cm.Add(tokenList3a)
	cm.Add(tokenList3b)

	// Get top 2 largest clusters
	largest := cm.GetLargestClusters(2)

	if len(largest) != 2 {
		t.Errorf("Expected 2 largest clusters, got %d", len(largest))
	}

	// Should be ordered by size (largest first)
	if largest[0].Size() != 3 {
		t.Errorf("Largest cluster should have size 3, got %d", largest[0].Size())
	}

	if largest[1].Size() != 2 {
		t.Errorf("Second largest cluster should have size 2, got %d", largest[1].Size())
	}
}

func TestClusterManager_Clear(t *testing.T) {
	cm := NewClusterManager()

	// Add some data
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenListWithTokens(tokens)
	cm.Add(tokenList)

	// Verify data exists
	stats := cm.GetStats()
	if stats.TotalTokenLists == 0 || stats.TotalClusters == 0 {
		t.Error("Should have data before clear")
	}

	// Clear
	cm.Clear()

	// Verify data is gone
	stats = cm.GetStats()
	if stats.TotalTokenLists != 0 || stats.TotalClusters != 0 || stats.HashBuckets != 0 {
		t.Error("Should have no data after clear")
	}

	allClusters := cm.GetAllClusters()
	if len(allClusters) != 0 {
		t.Error("Should have no clusters after clear")
	}
}

func TestClusterManager_Stats(t *testing.T) {
	cm := NewClusterManager()

	// Add TokenLists to create clusters of different sizes
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath},
	}
	tokens3 := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	tokenList3 := token.NewTokenListWithTokens(tokens3)

	cm.Add(tokenList1)
	cm.Add(tokenList2) // Same cluster as tokenList1
	cm.Add(tokenList3) // Different cluster

	stats := cm.GetStats()

	if stats.TotalTokenLists != 3 {
		t.Errorf("Expected 3 total TokenLists, got %d", stats.TotalTokenLists)
	}

	if stats.TotalClusters != 2 {
		t.Errorf("Expected 2 total clusters, got %d", stats.TotalClusters)
	}

	expectedAvg := 3.0 / 2.0 // 3 TokenLists / 2 clusters
	if stats.AverageClusterSize != expectedAvg {
		t.Errorf("Expected average cluster size %.2f, got %.2f", expectedAvg, stats.AverageClusterSize)
	}

	if stats.HashBuckets == 0 {
		t.Error("Should have at least one hash bucket")
	}
}

func TestClusterManager_PatternChangeType(t *testing.T) {
	cm := NewClusterManager()

	// Create token lists with same signature (HTTP method, space, path)
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/users", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/orders", Type: token.TokenAbsolutePath},
	}
	tokens3 := []token.Token{
		{Value: "PUT", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/items", Type: token.TokenAbsolutePath},
	}
	tokens4 := []token.Token{
		{Value: "DELETE", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api/products", Type: token.TokenAbsolutePath},
	}

	tokenList1 := token.NewTokenListWithTokens(tokens1)
	tokenList2 := token.NewTokenListWithTokens(tokens2)
	tokenList3 := token.NewTokenListWithTokens(tokens3)
	tokenList4 := token.NewTokenListWithTokens(tokens4)

	// First add - should create a new pattern
	cluster1, changeType1 := cm.Add(tokenList1)
	if changeType1 != PatternNew {
		t.Errorf("Expected PatternNew for first add, got %v", changeType1)
	}
	t.Logf("✅ Add #1: PatternNew (created cluster with PatternID=%d)", cluster1.GetPatternID())

	// Second add - same signature, but pattern not yet generated, so no change
	cluster2, changeType2 := cm.Add(tokenList2)
	if changeType2 != PatternNoChange {
		t.Errorf("Expected PatternNoChange for second add, got %v", changeType2)
	}
	if cluster1 != cluster2 {
		t.Error("Should return same cluster for same signature")
	}
	t.Logf("✅ Add #2: PatternNoChange (added to existing cluster, size=%d)", cluster2.Size())

	// Generate pattern to set up for PatternUpdated
	pattern := cluster2.GeneratePattern()
	if pattern == nil {
		t.Fatal("Pattern should be generated")
	}
	t.Logf("   Pattern after 2 logs: '%s'", cluster2.GetPatternString())

	// Third add - pattern exists, so it will be updated
	cluster3, changeType3 := cm.Add(tokenList3)
	if changeType3 != PatternUpdated {
		t.Errorf("Expected PatternUpdated for third add (pattern exists), got %v", changeType3)
	}
	if cluster1 != cluster3 {
		t.Error("Should return same cluster for same signature")
	}
	t.Logf("✅ Add #3: PatternUpdated (pattern will change, size=%d)", cluster3.Size())

	// Regenerate pattern to see the change
	newPattern := cluster3.GeneratePattern()
	if newPattern == nil {
		t.Fatal("Pattern should be regenerated")
	}
	t.Logf("   Pattern after 3 logs: '%s'", cluster3.GetPatternString())

	// Fourth add - pattern exists, so updated again
	cluster4, changeType4 := cm.Add(tokenList4)
	if changeType4 != PatternUpdated {
		t.Errorf("Expected PatternUpdated for fourth add (pattern exists), got %v", changeType4)
	}
	t.Logf("✅ Add #4: PatternUpdated (pattern will change, size=%d)", cluster4.Size())

	// Final pattern
	cluster4.GeneratePattern()
	t.Logf("   Final pattern after 4 logs: '%s'", cluster4.GetPatternString())

	// Verify all returned the same cluster
	if cluster1.Size() != 4 {
		t.Errorf("Expected cluster size 4, got %d", cluster1.Size())
	}
}
