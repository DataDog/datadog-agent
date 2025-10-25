// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automaton

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestNewRuleManager(t *testing.T) {
	rm := NewRuleManager()

	if rm.rules == nil {
		t.Error("Expected rules slice to be initialized")
	}
	if rm.categories == nil {
		t.Error("Expected categories map to be initialized")
	}
	if len(rm.rules) != 0 {
		t.Errorf("Expected empty rules slice, got %d rules", len(rm.rules))
	}
}

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

	if err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	if len(rm.rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rm.rules))
	}

	rule := rm.rules[0]
	if rule.Name != "TestIPv4" {
		t.Errorf("Expected rule name 'TestIPv4', got '%s'", rule.Name)
	}
	if rule.TokenType != token.TokenIPv4 {
		t.Errorf("Expected token type TokenIPv4, got %v", rule.TokenType)
	}
	if rule.Priority != 100 {
		t.Errorf("Expected priority 100, got %d", rule.Priority)
	}
	if rule.Category != "network" {
		t.Errorf("Expected category 'network', got '%s'", rule.Category)
	}
}

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

	if err == nil {
		t.Error("Expected error for invalid regex pattern")
	}
}

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

	if err == nil {
		t.Error("Expected error for example that doesn't match pattern")
	}
}

func TestRuleManager_RemoveRule(t *testing.T) {
	rm := NewRuleManager()

	// Add a rule first
	rm.AddRule("TestRule", `^\d+$`, "test", "Test", token.TokenNumeric, 50, []string{"123"})

	if len(rm.rules) != 1 {
		t.Fatalf("Expected 1 rule before removal")
	}

	// Remove the rule
	removed := rm.RemoveRule("TestRule")
	if !removed {
		t.Error("Expected RemoveRule to return true")
	}

	if len(rm.rules) != 0 {
		t.Errorf("Expected 0 rules after removal, got %d", len(rm.rules))
	}

	// Try to remove non-existent rule
	removed = rm.RemoveRule("NonExistent")
	if removed {
		t.Error("Expected RemoveRule to return false for non-existent rule")
	}
}

func TestRuleManager_GetRule(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("TestRule", `^\d+$`, "test", "Test", token.TokenNumeric, 50, []string{"123"})

	rule := rm.GetRule("TestRule")
	if rule == nil {
		t.Fatal("Expected to find rule 'TestRule'")
	}
	if rule.Name != "TestRule" {
		t.Errorf("Expected rule name 'TestRule', got '%s'", rule.Name)
	}

	notFound := rm.GetRule("NonExistent")
	if notFound != nil {
		t.Error("Expected nil for non-existent rule")
	}
}

func TestRuleManager_PriorityOrdering(t *testing.T) {
	rm := NewRuleManager()

	// Add rules in different priority order
	rm.AddRule("Low", `low`, "test", "Low priority", token.TokenWord, 10, []string{"low"})
	rm.AddRule("High", `high`, "test", "High priority", token.TokenWord, 100, []string{"high"})
	rm.AddRule("Medium", `medium`, "test", "Medium priority", token.TokenWord, 50, []string{"medium"})

	rules := rm.ListRules()
	if len(rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(rules))
	}

	// Should be ordered by priority (highest first)
	expectedOrder := []string{"High", "Medium", "Low"}
	expectedPriorities := []int{100, 50, 10}

	for i, rule := range rules {
		if rule.Name != expectedOrder[i] {
			t.Errorf("Rule %d: expected name '%s', got '%s'", i, expectedOrder[i], rule.Name)
		}
		if rule.Priority != expectedPriorities[i] {
			t.Errorf("Rule %d: expected priority %d, got %d", i, expectedPriorities[i], rule.Priority)
		}
	}
}

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
		{"999.999.999.999", token.TokenWord}, // Invalid IPv4, no rule matches
		{"abc", token.TokenWord},             // No rule matches
	}

	for _, test := range tests {
		result := rm.ApplyRules(test.input)
		if result != test.expected {
			t.Errorf("ApplyRules('%s'): expected %v, got %v", test.input, test.expected, result)
		}
	}
}

func TestRuleManager_GetRulesByCategory(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("IPv4", `ipv4`, "network", "IPv4", token.TokenIPv4, 100, []string{"ipv4"})
	rm.AddRule("Email", `email`, "network", "Email", token.TokenEmail, 90, []string{"email"})
	rm.AddRule("Numeric", `num`, "numeric", "Number", token.TokenNumeric, 50, []string{"num"})

	networkRules := rm.GetRulesByCategory("network")
	if len(networkRules) != 2 {
		t.Errorf("Expected 2 network rules, got %d", len(networkRules))
	}

	numericRules := rm.GetRulesByCategory("numeric")
	if len(numericRules) != 1 {
		t.Errorf("Expected 1 numeric rule, got %d", len(numericRules))
	}

	emptyRules := rm.GetRulesByCategory("nonexistent")
	if len(emptyRules) != 0 {
		t.Errorf("Expected 0 rules for nonexistent category, got %d", len(emptyRules))
	}
}

func TestRuleManager_GetCategories(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("Rule1", `r1`, "network", "Rule 1", token.TokenWord, 50, []string{"r1"})
	rm.AddRule("Rule2", `r2`, "time", "Rule 2", token.TokenWord, 50, []string{"r2"})
	rm.AddRule("Rule3", `r3`, "network", "Rule 3", token.TokenWord, 50, []string{"r3"})

	categories := rm.GetCategories()
	if len(categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(categories))
	}

	// Categories should be sorted
	expectedCategories := []string{"network", "time"}
	for i, expected := range expectedCategories {
		if i >= len(categories) || categories[i] != expected {
			t.Errorf("Expected category %d to be '%s', got '%s'", i, expected, categories[i])
		}
	}
}

func TestRuleManager_GetRuleStats(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("IPv4", `ipv4`, "network", "IPv4", token.TokenIPv4, 100, []string{"ipv4"})
	rm.AddRule("Email", `email`, "network", "Email", token.TokenEmail, 90, []string{"email"})
	rm.AddRule("Numeric", `num`, "numeric", "Number", token.TokenNumeric, 50, []string{"num"})

	stats := rm.GetRuleStats()

	if stats.TotalRules != 3 {
		t.Errorf("Expected TotalRules=3, got %d", stats.TotalRules)
	}
	if stats.Categories != 2 {
		t.Errorf("Expected Categories=2, got %d", stats.Categories)
	}
	if stats.ByCategory["network"] != 2 {
		t.Errorf("Expected 2 network rules, got %d", stats.ByCategory["network"])
	}
	if stats.ByCategory["numeric"] != 1 {
		t.Errorf("Expected 1 numeric rule, got %d", stats.ByCategory["numeric"])
	}
	if stats.ByTokenType[token.TokenIPv4] != 1 {
		t.Errorf("Expected 1 IPv4 token rule, got %d", stats.ByTokenType[token.TokenIPv4])
	}
}

func TestGetPredefinedRules(t *testing.T) {
	rules := GetPredefinedRules()

	if len(rules) == 0 {
		t.Error("Expected predefined rules to be non-empty")
	}

	// Check that we have the expected rule types
	foundRules := make(map[string]bool)
	for _, rule := range rules {
		foundRules[rule.Name] = true

		// Validate rule structure
		if rule.Pattern == nil {
			t.Errorf("Rule '%s' has nil pattern", rule.Name)
		}
		if rule.Name == "" {
			t.Error("Found rule with empty name")
		}
		if rule.Category == "" {
			t.Errorf("Rule '%s' has empty category", rule.Name)
		}
		if len(rule.Examples) == 0 {
			t.Errorf("Rule '%s' has no examples", rule.Name)
		}

		// Test examples against pattern
		for _, example := range rule.Examples {
			if !rule.Pattern.MatchString(example) {
				t.Errorf("Rule '%s': example '%s' doesn't match pattern", rule.Name, example)
			}
		}
	}

	expectedRules := []string{"IPv4Address", "EmailAddress", "URI", "HTTPStatus", "Numeric"}
	for _, expected := range expectedRules {
		if !foundRules[expected] {
			t.Errorf("Expected predefined rule '%s' not found", expected)
		}
	}
}

func TestRuleManager_LoadPredefinedRules(t *testing.T) {
	rm := NewRuleManager()

	err := rm.LoadPredefinedRules()
	if err != nil {
		t.Fatalf("Failed to load predefined rules: %v", err)
	}

	rules := rm.ListRules()
	if len(rules) == 0 {
		t.Error("Expected predefined rules to be loaded")
	}

	// Verify some key rules exist
	ipv4Rule := rm.GetRule("IPv4Address")
	if ipv4Rule == nil {
		t.Error("Expected IPv4Address rule to be loaded")
	}

	emailRule := rm.GetRule("EmailAddress")
	if emailRule == nil {
		t.Error("Expected EmailAddress rule to be loaded")
	}

	// Test that rules are working
	result := rm.ApplyRules("192.168.1.1")
	if result != token.TokenIPv4 {
		t.Errorf("Expected IPv4 token for '192.168.1.1', got %v", result)
	}

	result = rm.ApplyRules("test@example.com")
	if result != token.TokenEmail {
		t.Errorf("Expected Email token for 'test@example.com', got %v", result)
	}
}

// Test the priority management functions
func TestRuleManager_GetRuleByPriority(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("High1", `high1`, "test", "High 1", token.TokenWord, 100, []string{"high1"})
	rm.AddRule("High2", `high2`, "test", "High 2", token.TokenWord, 100, []string{"high2"})
	rm.AddRule("Medium", `medium`, "test", "Medium", token.TokenWord, 50, []string{"medium"})

	highRules := rm.GetRuleByPriority(100)
	if len(highRules) != 2 {
		t.Errorf("Expected 2 rules with priority 100, got %d", len(highRules))
	}

	mediumRules := rm.GetRuleByPriority(50)
	if len(mediumRules) != 1 {
		t.Errorf("Expected 1 rule with priority 50, got %d", len(mediumRules))
	}

	noRules := rm.GetRuleByPriority(999)
	if len(noRules) != 0 {
		t.Errorf("Expected 0 rules with priority 999, got %d", len(noRules))
	}
}

func TestRuleManager_GetHighestPriorityRules(t *testing.T) {
	rm := NewRuleManager()

	// Empty rule manager
	highRules := rm.GetHighestPriorityRules()
	if len(highRules) != 0 {
		t.Errorf("Expected 0 highest priority rules for empty manager, got %d", len(highRules))
	}

	rm.AddRule("High1", `high1`, "test", "High 1", token.TokenWord, 100, []string{"high1"})
	rm.AddRule("High2", `high2`, "test", "High 2", token.TokenWord, 100, []string{"high2"})
	rm.AddRule("Medium", `medium`, "test", "Medium", token.TokenWord, 50, []string{"medium"})

	highRules = rm.GetHighestPriorityRules()
	if len(highRules) != 2 {
		t.Errorf("Expected 2 highest priority rules, got %d", len(highRules))
	}

	for _, rule := range highRules {
		if rule.Priority != 100 {
			t.Errorf("Expected priority 100, got %d", rule.Priority)
		}
	}
}

func TestRuleManager_UpdateRulePriority(t *testing.T) {
	rm := NewRuleManager()

	rm.AddRule("TestRule", `test`, "test", "Test", token.TokenWord, 50, []string{"test"})

	err := rm.UpdateRulePriority("TestRule", 100)
	if err != nil {
		t.Fatalf("Failed to update rule priority: %v", err)
	}

	rule := rm.GetRule("TestRule")
	if rule == nil {
		t.Fatal("Rule not found after priority update")
	}
	if rule.Priority != 100 {
		t.Errorf("Expected priority 100, got %d", rule.Priority)
	}

	// Test updating non-existent rule
	err = rm.UpdateRulePriority("NonExistent", 200)
	if err == nil {
		t.Error("Expected error when updating non-existent rule")
	}
}

func TestRuleManager_CategoryDescription(t *testing.T) {
	rm := NewRuleManager()

	// Test empty description
	desc := rm.GetCategoryDescription("network")
	if desc != "" {
		t.Errorf("Expected empty description for non-existent category, got '%s'", desc)
	}

	// Set category description
	rm.SetCategoryDescription("network", "Network-related rules")
	desc = rm.GetCategoryDescription("network")
	if desc != "Network-related rules" {
		t.Errorf("Expected 'Network-related rules', got '%s'", desc)
	}

	// Add a rule to existing category and check description is preserved
	rm.AddRule("IPv4", `ipv4`, "network", "IPv4", token.TokenIPv4, 100, []string{"ipv4"})
	desc = rm.GetCategoryDescription("network")
	if desc != "Network-related rules" {
		t.Errorf("Expected description to be preserved, got '%s'", desc)
	}

	// Update existing category description
	rm.SetCategoryDescription("network", "Updated network description")
	desc = rm.GetCategoryDescription("network")
	if desc != "Updated network description" {
		t.Errorf("Expected 'Updated network description', got '%s'", desc)
	}
}

// Test global functions that provide external access to terminal rules
func TestGlobalTerminalRuleFunctions(t *testing.T) {
	// Test GetTerminalRules
	rules := GetTerminalRules()
	if len(rules) == 0 {
		t.Error("Expected GetTerminalRules to return non-empty list")
	}

	// Test GetRulesByCategory
	networkRules := GetRulesByCategory("network")
	if len(networkRules) == 0 {
		t.Error("Expected GetRulesByCategory('network') to return rules")
	}

	// Test GetRuleCategories
	categories := GetRuleCategories()
	if len(categories) == 0 {
		t.Error("Expected GetRuleCategories to return non-empty list")
	}

	// Test AddTerminalRule
	err := AddTerminalRule(
		"TestGlobalRule",
		`^test$`,
		"test",
		"Global test rule",
		token.TokenWord,
		25,
		[]string{"test"},
	)
	if err != nil {
		t.Errorf("Failed to add terminal rule: %v", err)
	}

	// Verify the rule was added
	allRules := GetTerminalRules()
	found := false
	for _, rule := range allRules {
		if rule.Name == "TestGlobalRule" {
			found = true
			break
		}
	}
	if !found {
		t.Error("TestGlobalRule not found after adding")
	}

	// Test GetRuleStats
	stats := GetRuleStats()
	if stats.TotalRules == 0 {
		t.Error("Expected GetRuleStats to return non-zero total rules")
	}
}
