// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automaton

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestGlobalTrie_ExactMatch(t *testing.T) {
	tests := []struct {
		input    string
		expected token.TokenType
	}{
		{"GET", token.TokenHTTPMethod},
		{"POST", token.TokenHTTPMethod},
		{"ERROR", token.TokenSeverityLevel},
		{"INFO", token.TokenSeverityLevel},
		{"debug", token.TokenSeverityLevel}, // lowercase
		{" ", token.TokenWhitespace},
		{"\t", token.TokenWhitespace},
		{"unknown", token.TokenWord}, // no rule matches - generic word
	}

	for _, test := range tests {
		result := globalTrie.Match(test.input)
		if result != test.expected {
			t.Errorf("globalTrie.Match('%s'): expected %v, got %v",
				test.input, test.expected, result)
		}
	}
}

func TestGlobalTrie_TerminalRules(t *testing.T) {
	tests := []struct {
		input    string
		expected token.TokenType
	}{
		{"200", token.TokenHTTPStatus},
		{"404", token.TokenHTTPStatus},
		{"500", token.TokenHTTPStatus},
		{"192.168.1.1", token.TokenIPv4},
		{"10.0.0.1", token.TokenIPv4},
		{"test@example.com", token.TokenEmail},
		{"user@domain.org", token.TokenEmail},
		{"/api/users", token.TokenAbsolutePath},
		{"/var/log/app.log", token.TokenAbsolutePath},
		{"2023-12-25", token.TokenDate},
		{"2023-12-25T14:30:00", token.TokenDate},
		{"1234", token.TokenNumeric}, // 4 digits won't match HTTP status
		{"0", token.TokenNumeric},
		{"https://example.com", token.TokenURI},
		{"http://api.domain.com/path", token.TokenURI},
	}

	for _, test := range tests {
		result := globalTrie.Match(test.input)
		if result != test.expected {
			t.Errorf("globalTrie.Match('%s'): expected %v, got %v",
				test.input, test.expected, result)
		}
	}
}

func TestTrieStats(t *testing.T) {
	stats := globalTrie.GetStats()

	if stats.ExactPatterns == 0 {
		t.Error("Expected some exact patterns in trie")
	}
	if stats.TerminalRules == 0 {
		t.Error("Expected some terminal rules")
	}
	if stats.TrieNodes == 0 {
		t.Error("Expected some trie nodes")
	}

	t.Logf("Trie Stats: %d exact patterns, %d terminal rules, %d nodes, max depth %d",
		stats.ExactPatterns, stats.TerminalRules, stats.TrieNodes, stats.MaxDepth)
}

func TestTrie_AddExactPattern(t *testing.T) {
	// Create a new trie for testing
	testTrie := NewTrie()

	// Add a custom pattern
	testTrie.AddExactPattern("CUSTOM", token.TokenWord)

	// Test that it matches
	result := testTrie.Match("CUSTOM")
	if result != token.TokenWord {
		t.Errorf("Expected TokenWord for 'CUSTOM', got %v", result)
	}

	// Test that unknown patterns fall back to TokenWord (generic word)
	result = testTrie.Match("unknown")
	if result != token.TokenWord {
		t.Errorf("Expected TokenWord for 'unknown', got %v", result)
	}
}

func TestTrie_AddTerminalRule(t *testing.T) {
	// Test adding terminal rule to global rule manager
	err := globalRuleManager.AddRule(
		"TestRule",
		`^TEST\d+$`,
		"test",
		"Test rule for testing",
		token.TokenNumeric,
		PriorityHigh, // Higher priority than existing rules
		[]string{"TEST123"},
	)
	if err != nil {
		t.Fatalf("Failed to add terminal rule: %v", err)
	}

	// Test that it matches using global trie
	result := globalTrie.Match("TEST123")
	if result != token.TokenNumeric {
		t.Errorf("Expected TokenNumeric for 'TEST123', got %v", result)
	}

	// Test that non-matching patterns don't match
	result = globalTrie.Match("TESTXYZ")
	if result == token.TokenNumeric {
		t.Error("Should not match non-numeric pattern")
	}

	// Clean up - remove the test rule
	globalRuleManager.RemoveRule("TestRule")
}

func TestTrie_InvalidTerminalRule(t *testing.T) {
	// Try to add invalid regex to global rule manager
	err := globalRuleManager.AddRule(
		"InvalidRule",
		`[invalid(regex`,
		"test",
		"Invalid rule",
		token.TokenWord,
		PriorityMedium,
		[]string{},
	)
	if err == nil {
		t.Error("Expected error for invalid regex pattern")
	}
}

func TestTrie_ExactMatchPriority(t *testing.T) {
	testTrie := NewTrie()

	// Add exact pattern
	testTrie.AddExactPattern("TEST", token.TokenWord)

	// Add terminal rule that would also match
	globalRuleManager.AddRule(
		"ExactMatchTestRule",
		`^TEST$`,
		"test",
		"Test rule for exact match priority",
		token.TokenNumeric,
		PriorityHigh,
		[]string{"TEST"},
	)

	// Exact match should take priority
	result := testTrie.Match("TEST")
	if result != token.TokenWord {
		t.Errorf("Exact match should take priority, expected TokenWord, got %v", result)
	}

	// Clean up
	globalRuleManager.RemoveRule("ExactMatchTestRule")
}

func TestTrie_EmptyInput(t *testing.T) {
	result := globalTrie.Match("")
	if result != token.TokenUnknown {
		t.Errorf("Empty input should return TokenUnknown, got %v", result)
	}
}

func TestTrieNodeStructure(t *testing.T) {
	testTrie := NewTrie()
	testTrie.AddExactPattern("ABC", token.TokenWord)

	// Verify trie structure
	stats := testTrie.GetStats()
	if stats.TrieNodes < 4 { // root + A + B + C
		t.Errorf("Expected at least 4 trie nodes, got %d", stats.TrieNodes)
	}
	if stats.ExactPatterns < 1 {
		t.Errorf("Expected at least 1 exact pattern, got %d", stats.ExactPatterns)
	}
}

func TestTrieDepthCalculation(t *testing.T) {
	testTrie := NewTrie()
	testTrie.AddExactPattern("A", token.TokenWord)
	testTrie.AddExactPattern("ABCDEFGHIJ", token.TokenWord) // 10 chars deep

	stats := testTrie.GetStats()
	if stats.MaxDepth < 10 {
		t.Errorf("Expected max depth >= 10, got %d", stats.MaxDepth)
	}
}
