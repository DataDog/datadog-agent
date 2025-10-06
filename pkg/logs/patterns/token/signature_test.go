// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package token

import (
	"testing"
)

func TestNewSignature(t *testing.T) {
	// Empty TokenList
	emptyTL := NewTokenList()
	emptySig := NewSignature(emptyTL)
	if emptySig.Position != "" || emptySig.Count != "" || emptySig.Length != 0 || emptySig.Hash != 0 {
		t.Error("Empty TokenList should have empty signature")
	}

	// Non-empty TokenList
	tokens := []Token{
		{Type: TokenHttpMethod, Value: "GET"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenAbsolutePath, Value: "/api"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenHttpStatus, Value: "200"},
	}
	tl := NewTokenListWithTokens(tokens)
	sig := NewSignature(tl)

	expectedPosition := "HttpMethod|Whitespace|AbsolutePath|Whitespace|HttpStatus"
	if sig.Position != expectedPosition {
		t.Errorf("Expected position signature '%s', got '%s'", expectedPosition, sig.Position)
	}

	if sig.Length != 5 {
		t.Errorf("Expected length 5, got %d", sig.Length)
	}

	if sig.Hash == 0 {
		t.Error("Hash should not be 0 for non-empty TokenList")
	}
}

func TestSignature_Equals(t *testing.T) {
	tokens1 := []Token{
		{Type: TokenWord, Value: "hello"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenWord, Value: "world"},
	}
	tokens2 := []Token{
		{Type: TokenWord, Value: "goodbye"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenWord, Value: "world"},
	}
	tokens3 := []Token{
		{Type: TokenWord, Value: "hello"},
		{Type: TokenNumeric, Value: "123"}, // Different type
	}

	tl1 := NewTokenListWithTokens(tokens1)
	tl2 := NewTokenListWithTokens(tokens2)
	tl3 := NewTokenListWithTokens(tokens3)

	sig1 := NewSignature(tl1)
	sig2 := NewSignature(tl2)
	sig3 := NewSignature(tl3)

	// Same structure, different values - should be equal
	if !sig1.Equals(sig2) {
		t.Error("TokenLists with same structure should have equal signatures")
	}

	// Different structure - should not be equal
	if sig1.Equals(sig3) {
		t.Error("TokenLists with different structure should not have equal signatures")
	}

	// Test signature equality with itself
	if !sig1.Equals(sig1) {
		t.Error("Signature should equal itself")
	}
}

func TestSignature_String(t *testing.T) {
	tokens := []Token{
		{Type: TokenWord, Value: "test"},
	}
	tl := NewTokenListWithTokens(tokens)
	sig := NewSignature(tl)

	str := sig.String()
	if str == "" {
		t.Error("Signature string should not be empty")
	}

	// Should contain key components
	if !containsAll(str, []string{"pos:", "count:", "len:", "hash:"}) {
		t.Errorf("Signature string should contain all components, got: %s", str)
	}
}

func TestSignature_IsEmpty(t *testing.T) {
	// Empty signature
	emptyTL := NewTokenList()
	emptySig := NewSignature(emptyTL)
	if !emptySig.IsEmpty() {
		t.Error("Empty signature should return true for IsEmpty()")
	}

	// Non-empty signature
	tokens := []Token{{Type: TokenWord, Value: "test"}}
	tl := NewTokenListWithTokens(tokens)
	sig := NewSignature(tl)
	if sig.IsEmpty() {
		t.Error("Non-empty signature should return false for IsEmpty()")
	}
}

func TestSignature_HasSameStructure(t *testing.T) {
	// Same structure, different values
	tokens1 := []Token{
		{Type: TokenHttpMethod, Value: "GET"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenAbsolutePath, Value: "/api"},
	}
	tokens2 := []Token{
		{Type: TokenHttpMethod, Value: "POST"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenAbsolutePath, Value: "/users"},
	}

	tl1 := NewTokenListWithTokens(tokens1)
	tl2 := NewTokenListWithTokens(tokens2)
	sig1 := NewSignature(tl1)
	sig2 := NewSignature(tl2)

	if !sig1.HasSameStructure(sig2) {
		t.Error("Signatures with same structure should return true")
	}

	// Different structure
	tokens3 := []Token{
		{Type: TokenWord, Value: "different"},
		{Type: TokenNumeric, Value: "123"},
	}
	tl3 := NewTokenListWithTokens(tokens3)
	sig3 := NewSignature(tl3)

	if sig1.HasSameStructure(sig3) {
		t.Error("Signatures with different structure should return false")
	}
}

func TestSignature_GetHashBucket(t *testing.T) {
	tokens := []Token{
		{Type: TokenWord, Value: "test"},
	}
	tl := NewTokenListWithTokens(tokens)
	sig := NewSignature(tl)

	hashBucket := sig.GetHashBucket()
	if hashBucket != sig.Hash {
		t.Error("GetHashBucket should return the signature hash")
	}
	if hashBucket == 0 {
		t.Error("Hash bucket should not be 0 for non-empty signature")
	}
}

func TestComputeHash(t *testing.T) {
	// Test that same input produces same hash
	input1 := "test input"
	input2 := "test input"
	input3 := "different input"

	hash1 := computeHash(input1)
	hash2 := computeHash(input2)
	hash3 := computeHash(input3)

	if hash1 != hash2 {
		t.Error("Same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("Different input should produce different hash (very likely)")
	}
	if hash1 == 0 {
		t.Error("Hash should not be 0")
	}
}

func TestSignature_ConsistentHashing(t *testing.T) {
	// Test that identical TokenLists produce identical signatures with same hash
	tokens := []Token{
		{Type: TokenHttpMethod, Value: "GET"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenAbsolutePath, Value: "/api"},
	}

	tl1 := NewTokenListWithTokens(tokens)
	tl2 := NewTokenListWithTokens(tokens)

	sig1 := NewSignature(tl1)
	sig2 := NewSignature(tl2)

	if sig1.Hash != sig2.Hash {
		t.Error("Identical TokenLists should produce identical signature hashes")
	}
	if !sig1.Equals(sig2) {
		t.Error("Identical TokenLists should produce equal signatures")
	}
}

// Helper function to check if string contains all substrings
func containsAll(str string, substrings []string) bool {
	for _, substr := range substrings {
		found := false
		for i := 0; i <= len(str)-len(substr); i++ {
			if str[i:i+len(substr)] == substr {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
