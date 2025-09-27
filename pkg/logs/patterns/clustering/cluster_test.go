// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestCluster_NewCluster(t *testing.T) {
	// Create a simple TokenList
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList := token.NewTokenList(tokens)
	signature := tokenList.Signature()

	cluster := NewCluster(signature, tokenList)

	if cluster.Size() != 1 {
		t.Errorf("Expected cluster size 1, got %d", cluster.Size())
	}

	if !cluster.Signature.Equals(signature) {
		t.Error("Cluster signature doesn't match expected signature")
	}

	if cluster.Pattern != nil {
		t.Error("Pattern should be nil initially (computed lazily)")
	}
}

func TestCluster_Add(t *testing.T) {
	// Create first TokenList
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList1 := token.NewTokenList(tokens1)
	signature1 := tokenList1.Signature()

	cluster := NewCluster(signature1, tokenList1)

	// Create second TokenList with same signature but different values
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/users", Type: token.TokenAbsolutePath},
	}
	tokenList2 := token.NewTokenList(tokens2)

	// Should add successfully (same signature)
	if !cluster.Add(tokenList2) {
		t.Error("Failed to add TokenList with matching signature")
	}

	if cluster.Size() != 2 {
		t.Errorf("Expected cluster size 2, got %d", cluster.Size())
	}

	// Create third TokenList with different signature
	tokens3 := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	tokenList3 := token.NewTokenList(tokens3)

	// Should fail to add (different signature)
	if cluster.Add(tokenList3) {
		t.Error("Should not add TokenList with different signature")
	}

	if cluster.Size() != 2 {
		t.Errorf("Expected cluster size to remain 2, got %d", cluster.Size())
	}
}

func TestCluster_GeneratePattern_NoWildcards(t *testing.T) {
	// Create cluster with identical TokenLists
	tokens := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokenList1 := token.NewTokenList(tokens)
	tokenList2 := token.NewTokenList(tokens) // Identical

	cluster := NewCluster(tokenList1.Signature(), tokenList1)
	cluster.Add(tokenList2)

	pattern := cluster.GeneratePattern()

	if pattern == nil {
		t.Fatal("Pattern should not be nil")
	}

	// Should have no wildcards since all values are identical
	if cluster.HasWildcards() {
		t.Error("Should not have wildcards for identical TokenLists")
	}

	// Pattern should match original tokens
	if pattern.Length() != 3 {
		t.Errorf("Expected pattern length 3, got %d", pattern.Length())
	}

	if pattern.Tokens[0].Value != "GET" {
		t.Errorf("Expected first token 'GET', got '%s'", pattern.Tokens[0].Value)
	}
}

func TestCluster_GeneratePattern_WithWildcards(t *testing.T) {
	// Create cluster with different values at some positions
	tokens1 := []token.Token{
		{Value: "GET", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/api", Type: token.TokenAbsolutePath},
	}
	tokens2 := []token.Token{
		{Value: "POST", Type: token.TokenHttpMethod},     // Different value
		{Value: " ", Type: token.TokenWhitespace},        // Same value
		{Value: "/users", Type: token.TokenAbsolutePath}, // Different value
	}

	tokenList1 := token.NewTokenList(tokens1)
	tokenList2 := token.NewTokenList(tokens2)

	cluster := NewCluster(tokenList1.Signature(), tokenList1)
	cluster.Add(tokenList2)

	pattern := cluster.GeneratePattern()

	if pattern == nil {
		t.Fatal("Pattern should not be nil")
	}

	// Should have wildcards at positions 0 and 2
	if !cluster.HasWildcards() {
		t.Error("Should have wildcards for different values")
	}

	wildcardPositions := cluster.GetWildcardPositions()
	expectedPositions := map[int]bool{0: true, 2: true}

	if len(wildcardPositions) != 2 {
		t.Errorf("Expected 2 wildcard positions, got %d", len(wildcardPositions))
	}

	for _, pos := range wildcardPositions {
		if !expectedPositions[pos] {
			t.Errorf("Unexpected wildcard position: %d", pos)
		}
	}

	// Check pattern tokens
	if pattern.Tokens[0].Value != "*" || !pattern.Tokens[0].IsWildcard {
		t.Error("Position 0 should be a wildcard")
	}

	if pattern.Tokens[1].Value != " " || pattern.Tokens[1].IsWildcard {
		t.Error("Position 1 should not be a wildcard")
	}

	if pattern.Tokens[2].Value != "/*" || !pattern.Tokens[2].IsWildcard {
		t.Error("Position 2 should be a wildcard with path pattern")
	}
}

func TestCluster_GeneratePattern_SingleTokenList(t *testing.T) {
	// Create cluster with single TokenList
	tokens := []token.Token{
		{Value: "ERROR", Type: token.TokenSeverityLevel},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "failed", Type: token.TokenWord},
	}
	tokenList := token.NewTokenList(tokens)

	cluster := NewCluster(tokenList.Signature(), tokenList)
	pattern := cluster.GeneratePattern()

	if pattern == nil {
		t.Fatal("Pattern should not be nil")
	}

	// Single TokenList should have no wildcards
	if cluster.HasWildcards() {
		t.Error("Single TokenList should not have wildcards")
	}

	// Pattern should be identical to original
	if pattern.Length() != tokenList.Length() {
		t.Error("Pattern length should match original TokenList")
	}

	for i, tok := range pattern.Tokens {
		if tok.Value != tokenList.Tokens[i].Value {
			t.Errorf("Pattern token %d value mismatch: expected '%s', got '%s'",
				i, tokenList.Tokens[i].Value, tok.Value)
		}
	}
}

func TestCluster_GeneratePattern_Caching(t *testing.T) {
	// Create cluster
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

	tokenList1 := token.NewTokenList(tokens1)
	tokenList2 := token.NewTokenList(tokens2)

	cluster := NewCluster(tokenList1.Signature(), tokenList1)
	cluster.Add(tokenList2)

	// Generate pattern twice
	pattern1 := cluster.GeneratePattern()
	pattern2 := cluster.GeneratePattern()

	// Should return the same cached instance
	if pattern1 != pattern2 {
		t.Error("Pattern should be cached and return same instance")
	}

	// Add another TokenList - should invalidate cache
	tokens3 := []token.Token{
		{Value: "PUT", Type: token.TokenHttpMethod},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "/items", Type: token.TokenAbsolutePath},
	}
	tokenList3 := token.NewTokenList(tokens3)
	cluster.Add(tokenList3)

	pattern3 := cluster.GeneratePattern()

	// Should be a new instance (cache was invalidated)
	if pattern1 == pattern3 {
		t.Error("Pattern cache should be invalidated after adding new TokenList")
	}
}
