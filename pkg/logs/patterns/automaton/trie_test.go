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
		{"GET", token.TokenHttpMethod},
		{"POST", token.TokenHttpMethod},
		{"ERROR", token.TokenSeverityLevel},
		{"INFO", token.TokenSeverityLevel},
		{"debug", token.TokenSeverityLevel}, // lowercase
		{" ", token.TokenWhitespace},
		{"\t", token.TokenWhitespace},
		{"unknown", token.TokenWord}, // fallback
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
		{"200", token.TokenHttpStatus},
		{"404", token.TokenHttpStatus},
		{"500", token.TokenHttpStatus},
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

	// Test that unknown patterns fall back to terminal rules
	result = testTrie.Match("unknown")
	if result == token.TokenUnknown {
		t.Error("Expected terminal rules to handle unknown patterns")
	}
}

func TestTrie_AddTerminalRule(t *testing.T) {
	// Test adding terminal rule to global rule manager instead
	err := AddTerminalRule(
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
	err := AddTerminalRule(
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
	testTrie.AddTerminalRule(`^TEST$`, token.TokenNumeric, PriorityHigh)

	// Exact match should take priority
	result := testTrie.Match("TEST")
	if result != token.TokenWord {
		t.Errorf("Exact match should take priority, expected TokenWord, got %v", result)
	}
}

func TestTrie_EmptyInput(t *testing.T) {
	result := globalTrie.Match("")
	if result != token.TokenUnknown {
		t.Errorf("Empty input should return TokenUnknown, got %v", result)
	}
}

func TestValidationFunctions(t *testing.T) {
	// Test IPv4 validation
	validIPv4 := []string{"192.168.1.1", "10.0.0.1", "255.255.255.255", "0.0.0.0"}
	invalidIPv4 := []string{"256.1.1.1", "192.168.1", "192.168.1.1.1", "abc.def.ghi.jkl"}

	for _, ip := range validIPv4 {
		if !validateIPv4(ip) {
			t.Errorf("validateIPv4('%s') should return true", ip)
		}
	}

	for _, ip := range invalidIPv4 {
		if validateIPv4(ip) {
			t.Errorf("validateIPv4('%s') should return false", ip)
		}
	}

	// Test email validation
	validEmails := []string{"test@example.com", "user@domain.org", "admin@company.co.uk"}
	invalidEmails := []string{"invalid", "test@", "@domain.com", "test@@domain.com"}

	for _, email := range validEmails {
		if !validateEmail(email) {
			t.Errorf("validateEmail('%s') should return true", email)
		}
	}

	for _, email := range invalidEmails {
		if validateEmail(email) {
			t.Errorf("validateEmail('%s') should return false", email)
		}
	}

	// Test date validation
	validDates := []string{"2023-12-25", "2023-12-25T14:30:00", "12/25/2023", "2023-12-25T14:30:00.123Z"}
	invalidDates := []string{"invalid", "123", "abc", ""}

	for _, date := range validDates {
		if !validateDate(date) {
			t.Errorf("validateDate('%s') should return true", date)
		}
	}

	for _, date := range invalidDates {
		if validateDate(date) {
			t.Errorf("validateDate('%s') should return false", date)
		}
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
