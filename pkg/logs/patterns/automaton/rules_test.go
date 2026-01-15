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
	assert.Equal(t, 0, len(rm.rules), "Expected empty rules slice")
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

	assert.NotEqual(t, 0, len(rm.rules), "Expected predefined rules to be loaded")

	// Verify rules are sorted by priority (highest first)
	for i := 1; i < len(rm.rules); i++ {
		assert.GreaterOrEqual(t, rm.rules[i-1].Priority, rm.rules[i].Priority,
			"Rules should be sorted by priority (highest first)")
	}

	// Test that rules are working
	result := rm.ApplyRules("192.168.1.1")
	assert.Equal(t, token.TokenIPv4, result, "Expected IPv4 token for '192.168.1.1'")

	result = rm.ApplyRules("test@example.com")
	assert.Equal(t, token.TokenEmail, result, "Expected Email token for 'test@example.com'")

	result = rm.ApplyRules("200")
	assert.Equal(t, token.TokenHTTPStatus, result, "Expected HTTPStatus token for '200'")

	result = rm.ApplyRules("unknown")
	assert.Equal(t, token.TokenWord, result, "Expected Word token for unmatched value")
}
