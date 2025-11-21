// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automaton

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestNewRuleManager tests the creation of a new rule manager
func TestNewRuleManager(t *testing.T) {
	rm := NewRuleManager()

	assert.NotNil(t, rm.rules, "Expected rules slice to be initialized")
	assert.NotNil(t, rm.categories, "Expected categories map to be initialized")
	assert.Equal(t, 0, len(rm.rules), "Expected empty rules slice")
}

// TestRuleManager_AddRule tests the addition of a new rule
func TestRuleManager_AddRule(t *testing.T) {
	rm := NewRuleManager()

	err := rm.AddRule(
		"TestIPv4",
		`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`,
		"network",
		"Test IPv4 pattern",
		token.TokenIPv4,
		100,
		[]string{"192.168.1.1", "10.0.0.1"},
	)

	assert.NoError(t, err, "Failed to add rule")
	assert.Equal(t, 1, len(rm.rules), "Expected 1 rule")

	rule := rm.rules[0]
	assert.Equal(t, "TestIPv4", rule.Name, "Expected rule name 'TestIPv4'")
	assert.Equal(t, token.TokenIPv4, rule.TokenType, "Expected token type TokenIPv4")
	assert.Equal(t, 100, rule.Priority, "Expected priority 100")
	assert.Equal(t, "network", rule.Category, "Expected category 'network'")
}

// TestRuleManager_AddRule_InvalidPattern tests the addition of a new rule with an invalid regex pattern
func TestRuleManager_AddRule_InvalidPattern(t *testing.T) {
	rm := NewRuleManager()

	err := rm.AddRule(
		"BadRule",
		`[invalid(regex`,
		"test",
		"Invalid regex",
		token.TokenWord,
		50,
		[]string{},
	)

	assert.Error(t, err, "Expected error for invalid regex pattern")
}

// TestRuleManager_AddRule_InvalidExample tests the addition of a new rule with an invalid example
func TestRuleManager_AddRule_InvalidExample(t *testing.T) {
	rm := NewRuleManager()

	err := rm.AddRule(
		"TestRule",
		`^\d+$`,
		"test",
		"Numeric pattern",
		token.TokenNumeric,
		50,
		[]string{"123", "abc"}, // "abc" doesn't match ^\d+$
	)

	assert.Error(t, err, "Expected error for example that doesn't match pattern")
}

// TestRuleManager_AddRule_Duplicate tests the addition of a duplicate rule
func TestRuleManager_AddRule_Duplicate(t *testing.T) {
	rm := NewRuleManager()

	// Add first rule
	err := rm.AddRule("TestRule", `^\d+$`, "test", "Numeric", token.TokenNumeric, 50, []string{"123"})
	assert.NoError(t, err, "Failed to add first rule")

	// Try to add duplicate rule
	err = rm.AddRule("TestRule", `^[a-z]+$`, "test", "Alpha", token.TokenWord, 50, []string{"abc"})
	assert.Error(t, err, "Expected error when adding duplicate rule name")
	assert.Contains(t, err.Error(), "already exists", "Expected 'already exists' error")
}

// TestRuleManager_RemoveRule tests the removal of a rule
func TestRuleManager_RemoveRule(t *testing.T) {
	rm := NewRuleManager()

	// Add a rule first
	rm.AddRule("TestRule", `^\d+$`, "test", "Test", token.TokenNumeric, 50, []string{"123"})

	assert.Equal(t, 1, len(rm.rules), "Expected 1 rule before removal")

	// Remove the rule
	removed := rm.RemoveRule("TestRule")
	assert.True(t, removed, "Expected RemoveRule to return true")
	assert.Equal(t, 0, len(rm.rules), "Expected 0 rules after removal")

	// Try to remove non-existent rule
	removed = rm.RemoveRule("NonExistent")
	assert.False(t, removed, "Expected RemoveRule to return false for non-existent rule")
}

// TestRuleManager_GetRule tests the retrieval of a rule by name
func TestRuleManager_GetRule(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("TestRule", `^\d+$`, "test", "Test", token.TokenNumeric, 50, []string{"123"})

	rule := rm.GetRule("TestRule")
	assert.NotNil(t, rule, "Expected to find rule 'TestRule'")
	if rule != nil {
		assert.Equal(t, "TestRule", rule.Name, "Expected rule name 'TestRule'")
	}

	notFound := rm.GetRule("NonExistent")
	assert.Nil(t, notFound, "Expected nil for non-existent rule")
}

// TestRuleManager_PriorityOrdering tests the ordering of rules by priority
func TestRuleManager_PriorityOrdering(t *testing.T) {
	rm := NewRuleManager()

	// Add rules in different priority order
	rm.AddRule("Low", `low`, "test", "Low priority", token.TokenWord, 10, []string{"low"})
	rm.AddRule("High", `high`, "test", "High priority", token.TokenWord, 100, []string{"high"})
	rm.AddRule("Medium", `medium`, "test", "Medium priority", token.TokenWord, 50, []string{"medium"})

	rules := rm.ListRules()
	assert.Equal(t, 3, len(rules), "Expected 3 rules")

	// Should be ordered by priority (highest first)
	expectedOrder := []string{"High", "Medium", "Low"}
	expectedPriorities := []int{100, 50, 10}

	for i, rule := range rules {
		assert.Equal(t, expectedOrder[i], rule.Name, "Rule %d name mismatch", i)
		assert.Equal(t, expectedPriorities[i], rule.Priority, "Rule %d priority mismatch", i)
	}
}

// TestRuleManager_ApplyRules tests the application of rules to a value
func TestRuleManager_ApplyRules(t *testing.T) {
	rm := NewRuleManager()

	// Add rules with different priorities
	rm.AddRule("IPv4", `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`,
		"network", "IPv4", token.TokenIPv4, 100, []string{"192.168.1.1"})
	rm.AddRule("Numeric", `^\d+$`, "numeric", "Numbers", token.TokenNumeric, 30, []string{"123"})

	tests := []struct {
		input    string
		expected token.TokenType
	}{
		{"192.168.1.1", token.TokenIPv4}, // Higher priority rule should match
		{"123", token.TokenNumeric},
		{"999.999.999.999", token.TokenWord}, // Invalid IPv4, no rule matches - generic word
		{"abc", token.TokenWord},             // No rule matches - generic word
	}

	for _, test := range tests {
		result := rm.ApplyRules(test.input)
		assert.Equal(t, test.expected, result, "ApplyRules('%s') mismatch", test.input)
	}
}

// TestRuleManager_GetRulesByCategory tests the retrieval of rules by category
func TestRuleManager_GetRulesByCategory(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("IPv4", `ipv4`, "network", "IPv4", token.TokenIPv4, 100, []string{"ipv4"})
	rm.AddRule("Email", `email`, "network", "Email", token.TokenEmail, 90, []string{"email"})
	rm.AddRule("Numeric", `num`, "numeric", "Number", token.TokenNumeric, 50, []string{"num"})

	networkRules := rm.GetRulesByCategory("network")
	assert.Equal(t, 2, len(networkRules), "Expected 2 network rules")

	numericRules := rm.GetRulesByCategory("numeric")
	assert.Equal(t, 1, len(numericRules), "Expected 1 numeric rule")

	emptyRules := rm.GetRulesByCategory("nonexistent")
	assert.Equal(t, 0, len(emptyRules), "Expected 0 rules for nonexistent category")
}

// TestRuleManager_GetCategories tests the retrieval of categories
func TestRuleManager_GetCategories(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("Rule1", `r1`, "network", "Rule 1", token.TokenWord, 50, []string{"r1"})
	rm.AddRule("Rule2", `r2`, "time", "Rule 2", token.TokenWord, 50, []string{"r2"})
	rm.AddRule("Rule3", `r3`, "network", "Rule 3", token.TokenWord, 50, []string{"r3"})

	categories := rm.GetCategories()
	assert.Equal(t, 2, len(categories), "Expected 2 categories")

	// Categories should be sorted
	expectedCategories := []string{"network", "time"}
	for i, expected := range expectedCategories {
		if assert.Less(t, i, len(categories), "Category %d should exist", i) {
			assert.Equal(t, expected, categories[i], "Expected category %d to be '%s'", i, expected)
		}
	}
}

// TestRuleManager_GetRuleStats tests the retrieval of rule statistics
func TestRuleManager_GetRuleStats(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("IPv4", `ipv4`, "network", "IPv4", token.TokenIPv4, 100, []string{"ipv4"})
	rm.AddRule("Email", `email`, "network", "Email", token.TokenEmail, 90, []string{"email"})
	rm.AddRule("Numeric", `num`, "numeric", "Number", token.TokenNumeric, 50, []string{"num"})

	stats := rm.GetRuleStats()

	assert.Equal(t, 3, stats.TotalRules, "Expected TotalRules=3")
	assert.Equal(t, 2, stats.Categories, "Expected Categories=2")
	assert.Equal(t, 2, stats.ByCategory["network"], "Expected 2 network rules")
	assert.Equal(t, 1, stats.ByCategory["numeric"], "Expected 1 numeric rule")
	assert.Equal(t, 1, stats.ByTokenType[token.TokenIPv4], "Expected 1 IPv4 token rule")
}

// TestGetPredefinedRules tests the retrieval of predefined rules
func TestGetPredefinedRules(t *testing.T) {
	rules := GetPredefinedRules()

	assert.NotEqual(t, 0, len(rules), "Expected predefined rules to be non-empty")

	// Check that we have the expected rule types
	foundRules := make(map[string]bool)
	for _, rule := range rules {
		foundRules[rule.Name] = true

		// Validate rule structure
		assert.NotNil(t, rule.Pattern, "Rule '%s' has nil pattern", rule.Name)
		assert.NotEqual(t, "", rule.Name, "Found rule with empty name")
		assert.NotEqual(t, "", rule.Category, "Rule '%s' has empty category", rule.Name)
		assert.NotEqual(t, 0, len(rule.Examples), "Rule '%s' has no examples", rule.Name)

		// Test examples against pattern
		for _, example := range rule.Examples {
			assert.True(t, rule.Pattern.MatchString(example),
				"Rule '%s': example '%s' doesn't match pattern", rule.Name, example)
		}
	}

	expectedRules := []string{"IPv4Address", "EmailAddress", "URI", "HTTPStatus", "Numeric"}
	for _, expected := range expectedRules {
		assert.True(t, foundRules[expected], "Expected predefined rule '%s' not found", expected)
	}
}

// TestRuleManager_LoadPredefinedRules tests the loading of predefined rules
func TestRuleManager_LoadPredefinedRules(t *testing.T) {
	rm := NewRuleManager()

	err := rm.LoadPredefinedRules()
	assert.NoError(t, err, "Failed to load predefined rules")

	rules := rm.ListRules()
	assert.NotEqual(t, 0, len(rules), "Expected predefined rules to be loaded")

	// Verify some key rules exist
	ipv4Rule := rm.GetRule("IPv4Address")
	assert.NotNil(t, ipv4Rule, "Expected IPv4Address rule to be loaded")

	emailRule := rm.GetRule("EmailAddress")
	assert.NotNil(t, emailRule, "Expected EmailAddress rule to be loaded")

	// Test that rules are working
	result := rm.ApplyRules("192.168.1.1")
	assert.Equal(t, token.TokenIPv4, result, "Expected IPv4 token for '192.168.1.1'")

	result = rm.ApplyRules("test@example.com")
	assert.Equal(t, token.TokenEmail, result, "Expected Email token for 'test@example.com'")
}
