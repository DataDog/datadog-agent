// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package token

import (
	"testing"
)

func TestTokenList_NewTokenList(t *testing.T) {
	// Empty token list
	tl := NewTokenList()
	if tl == nil {
		t.Fatal("NewTokenList should not return nil")
	}
	if !tl.IsEmpty() {
		t.Error("New TokenList should be empty")
	}
	if tl.Length() != 0 {
		t.Errorf("New TokenList should have length 0, got %d", tl.Length())
	}

	// Token list with initial tokens
	tokens := []Token{
		{Type: TokenWord, Value: "hello"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenWord, Value: "world"},
	}
	tl2 := NewTokenList(tokens)
	if tl2.Length() != 3 {
		t.Errorf("Expected length 3, got %d", tl2.Length())
	}
	if tl2.IsEmpty() {
		t.Error("TokenList with tokens should not be empty")
	}
}

func TestTokenList_Add(t *testing.T) {
	tl := NewTokenList()

	token1 := Token{Type: TokenWord, Value: "hello"}
	tl.Add(token1)

	if tl.Length() != 1 {
		t.Errorf("Expected length 1, got %d", tl.Length())
	}
	if tl.IsEmpty() {
		t.Error("TokenList should not be empty after adding token")
	}
	if tl.Tokens[0].Value != "hello" {
		t.Errorf("Expected token value 'hello', got '%s'", tl.Tokens[0].Value)
	}
}

func TestTokenList_String(t *testing.T) {
	// Empty list
	tl := NewTokenList()
	if tl.String() != "[]" {
		t.Errorf("Empty TokenList string should be '[]', got '%s'", tl.String())
	}

	// Non-empty list
	tl.Add(Token{Type: TokenWord, Value: "hello"})
	tl.Add(Token{Type: TokenWhitespace, Value: " "})
	tl.Add(Token{Type: TokenWord, Value: "world"})

	expected := "[Word(hello), Whitespace( ), Word(world)]"
	if tl.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, tl.String())
	}
}

func TestTokenList_PositionSignature(t *testing.T) {
	// Empty token list
	emptyTL := NewTokenList()
	if emptyTL.PositionSignature() != "" {
		t.Error("Empty TokenList should have empty position signature")
	}

	// Non-empty token list
	tokens := []Token{
		{Type: TokenHttpMethod, Value: "GET"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenAbsolutePath, Value: "/api"},
	}
	tl := NewTokenList(tokens)

	expectedPosition := "HttpMethod|Whitespace|AbsolutePath"
	if tl.PositionSignature() != expectedPosition {
		t.Errorf("Expected position signature '%s', got '%s'", expectedPosition, tl.PositionSignature())
	}
}

func TestTokenList_CountSignature(t *testing.T) {
	// Empty token list
	emptyTL := NewTokenList()
	if emptyTL.CountSignature() != "" {
		t.Error("Empty TokenList should have empty count signature")
	}

	// Non-empty token list with duplicates
	tokens := []Token{
		{Type: TokenWord, Value: "hello"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenWord, Value: "world"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenNumeric, Value: "123"},
	}
	tl := NewTokenList(tokens)

	countSig := tl.CountSignature()
	// Should contain counts for each type
	if !containsSubstring(countSig, "Word:2") {
		t.Errorf("Count signature should contain 'Word:2', got '%s'", countSig)
	}
	if !containsSubstring(countSig, "Whitespace:2") {
		t.Errorf("Count signature should contain 'Whitespace:2', got '%s'", countSig)
	}
	if !containsSubstring(countSig, "Numeric:1") {
		t.Errorf("Count signature should contain 'Numeric:1', got '%s'", countSig)
	}
}

func TestTokenList_Signature(t *testing.T) {
	// Test that TokenList.Signature() creates a proper signature
	tokens := []Token{
		{Type: TokenHttpMethod, Value: "GET"},
		{Type: TokenWhitespace, Value: " "},
		{Type: TokenAbsolutePath, Value: "/api"},
	}
	tl := NewTokenList(tokens)
	sig := tl.Signature()

	if sig.Length != 3 {
		t.Errorf("Expected signature length 3, got %d", sig.Length)
	}
	if sig.Hash == 0 {
		t.Error("Signature hash should not be 0")
	}
	if sig.Position == "" {
		t.Error("Signature position should not be empty")
	}
}

// Helper function to check if string contains substring
func containsSubstring(str, substr string) bool {
	return len(str) >= len(substr) && findSubstring(str, substr) >= 0
}

func findSubstring(str, substr string) int {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
