// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automaton provides terminal rules for token classification.
package automaton

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Priority constants for rule evaluation order
//
// Rules are sorted by priority (highest first) and evaluated sequentially until the first match.
// Priority is based on the specificity of the pattern. The more specific the pattern, the higher the priority.
// Higher priority = evaluated first = more specific classification.
const (
	PriorityHigh   = 3 // Very specific patterns like IPv4, IPv6, Email
	PriorityMedium = 2 // Structured patterns like URI, Dates, HTTPStatus
	PriorityLow    = 1 // Generic fallback patterns like Numeric
)

// TerminalRule represents a classification rule
type TerminalRule struct {
	Name        string
	Pattern     *regexp.Regexp
	TokenType   token.TokenType
	Priority    int // Use PriorityHigh/Medium/Low constants - higher values evaluated first
	Category    string
	Description string
	Examples    []string
}

// RuleCategory represents a grouping of rules
type RuleCategory struct {
	Name        string
	Description string
	Rules       []*TerminalRule
}

// RuleManager manages terminal rules
type RuleManager struct {
	rules      []*TerminalRule
	categories map[string]*RuleCategory
}

// NewRuleManager creates a new rule manager
func NewRuleManager() *RuleManager {
	return &RuleManager{
		rules:      make([]*TerminalRule, 0),
		categories: make(map[string]*RuleCategory),
	}
}

// AddRule adds a new terminal rule
func (rm *RuleManager) AddRule(name, pattern, category, description string, tokenType token.TokenType, priority int, examples []string) error {
	// Check for duplicate rule name
	if rm.GetRule(name) != nil {
		return fmt.Errorf("rule '%s' already exists", name)
	}

	// Compile and validate regex pattern
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern '%s': %v", pattern, err)
	}

	// Validate examples match the pattern
	for _, example := range examples {
		if !regex.MatchString(example) {
			return fmt.Errorf("example '%s' does not match pattern '%s'", example, pattern)
		}
	}

	// Create and insert rule
	rule := &TerminalRule{
		Name:        name,
		Pattern:     regex,
		TokenType:   tokenType,
		Priority:    priority,
		Category:    category,
		Description: description,
		Examples:    examples,
	}

	rm.insertRuleByPriority(rule)
	rm.addToCategory(rule)

	return nil
}

// RemoveRule removes a rule by name
func (rm *RuleManager) RemoveRule(name string) bool {
	for i, rule := range rm.rules {
		if rule.Name == name {
			// Remove from rules list
			rm.rules = append(rm.rules[:i], rm.rules[i+1:]...)

			// Remove from category
			rm.removeFromCategory(rule)
			return true
		}
	}
	return false
}

// ApplyRules applies terminal rules in priority order to classify a token
// Returns TokenWord if no rule matches (generic word fallback)
func (rm *RuleManager) ApplyRules(value string) token.TokenType {
	for _, rule := range rm.rules {
		if rule.Pattern.MatchString(value) {
			return rule.TokenType
		}
	}
	return token.TokenWord
}

// LoadPredefinedRules loads predefined rules
func (rm *RuleManager) LoadPredefinedRules() error {
	predefined := GetPredefinedRules()

	for _, rule := range predefined {
		err := rm.AddRule(
			rule.Name,
			rule.Pattern.String(),
			rule.Category,
			rule.Description,
			rule.TokenType,
			rule.Priority,
			rule.Examples,
		)
		if err != nil {
			return fmt.Errorf("failed to load rule '%s': %v", rule.Name, err)
		}
	}

	return nil
}

// ================================================
// Helper methods
// ================================================

func (rm *RuleManager) insertRuleByPriority(rule *TerminalRule) {
	// Insert in priority order (higher priority first)
	inserted := false
	for i, existing := range rm.rules {
		if rule.Priority > existing.Priority {
			// Insert at position i
			rm.rules = append(rm.rules[:i], append([]*TerminalRule{rule}, rm.rules[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		rm.rules = append(rm.rules, rule)
	}
}

func (rm *RuleManager) addToCategory(rule *TerminalRule) {
	if rm.categories[rule.Category] == nil {
		rm.categories[rule.Category] = &RuleCategory{
			Name:        rule.Category,
			Description: fmt.Sprintf("Rules for %s tokens", rule.Category),
			Rules:       make([]*TerminalRule, 0),
		}
	}

	rm.categories[rule.Category].Rules = append(rm.categories[rule.Category].Rules, rule)
}

func (rm *RuleManager) removeFromCategory(rule *TerminalRule) {
	if category, exists := rm.categories[rule.Category]; exists {
		for i, r := range category.Rules {
			if r.Name == rule.Name {
				category.Rules = append(category.Rules[:i], category.Rules[i+1:]...)
				break
			}
		}

		// Remove category if empty
		if len(category.Rules) == 0 {
			delete(rm.categories, rule.Category)
		}
	}
}

// GetRule retrieves a rule by name
func (rm *RuleManager) GetRule(name string) *TerminalRule {
	for _, rule := range rm.rules {
		if rule.Name == name {
			return rule
		}
	}
	return nil
}

// ListRules returns all rules sorted by priority
func (rm *RuleManager) ListRules() []*TerminalRule {
	// Return a copy to prevent external modification
	result := make([]*TerminalRule, len(rm.rules))
	copy(result, rm.rules)
	return result
}

// GetRulesByCategory returns rules in a specific category
func (rm *RuleManager) GetRulesByCategory(category string) []*TerminalRule {
	if cat, exists := rm.categories[category]; exists {
		result := make([]*TerminalRule, len(cat.Rules))
		copy(result, cat.Rules)
		return result
	}
	return []*TerminalRule{}
}

// GetCategories returns all rule categories
func (rm *RuleManager) GetCategories() []string {
	categories := make([]string, 0, len(rm.categories))
	for name := range rm.categories {
		categories = append(categories, name)
	}
	sort.Strings(categories)
	return categories
}

// GetRuleStats returns statistics about the rule system
func (rm *RuleManager) GetRuleStats() RuleStats {
	stats := RuleStats{
		TotalRules:  len(rm.rules),
		Categories:  len(rm.categories),
		ByCategory:  make(map[string]int),
		ByTokenType: make(map[token.TokenType]int),
	}

	for _, rule := range rm.rules {
		stats.ByCategory[rule.Category]++
		stats.ByTokenType[rule.TokenType]++
	}

	return stats
}

// RuleStats contains statistics about the rule system
type RuleStats struct {
	TotalRules  int
	Categories  int
	ByCategory  map[string]int
	ByTokenType map[token.TokenType]int
}

// GetPredefinedRules returns the standard set of terminal rules
func GetPredefinedRules() []*TerminalRule {
	rules := []*TerminalRule{

		// =============================================================================
		// DATE & TIME PATTERNS (Combined)
		// =============================================================================
		{
			Name:        "DateTime",
			Pattern:     regexp.MustCompile(`^(?:(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[\+\-]\d{2}:?\d{2})?|(\d+)-(\d+)-(\d+)([A-Za-z_]+)(\d+):(\d+):(\d+)\.(\d+)([A-Za-z_]+)(\d+):(\d+)|(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})(,\d+)?|([A-Za-z_]+), (\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+):(\d+) ([A-Za-z_]+)|([A-Za-z_]+), (\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+):(\d+) (-\d+)|([A-Za-z_]+), (\d+)-([A-Za-z_]+)-(\d+) (\d+):(\d+):(\d+) ([A-Za-z_]+)|(\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+) ([A-Za-z_]+)|(\d+) ([A-Za-z_]+) (\d+) (\d+):(\d+) (-\d+)|([A-Za-z_]+) ([A-Za-z_]+) +(\d+) (\d+):(\d+):(\d+) (\d+)|([A-Za-z_]+) ([A-Za-z_]+) +(\d+) (\d+):(\d+):(\d+)( [A-Za-z_]+ (\d+))?|([A-Za-z_]+) ([A-Za-z_]+) (\d+) (\d+):(\d+):(\d+) ([\-\+]\d+) (\d+)|([A-Za-z_]+) (\d+), (\d{4}) (\d+):(\d+):(\d+) (AM|PM)|(\d{4})/(\d{2})/(\d{2}) (\d{2}):(\d{2}):(\d{2})|(\d{4})-(1[012]|0?[1-9])-([12][0-9]|3[01]|0?[1-9])$)`),
			TokenType:   token.TokenDate,
			Priority:    PriorityHigh,
			Category:    "time",
			Description: "Matches many types of date times",
			Examples: []string{
				"2024-01-15T10:30:45Z",
				"Mon, 02 Jan 2006 15:04:05 MST",
			},
		},

		// =============================================================================
		// NETWORK PATTERNS (Priority: High)
		// =============================================================================

		{
			Name:        "IPv4Address",
			Pattern:     regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`),
			TokenType:   token.TokenIPv4,
			Priority:    PriorityHigh,
			Category:    "network",
			Description: "Matches IPv4 addresses in dotted decimal notation",
			Examples:    []string{"192.168.1.1", "10.0.0.1", "255.255.255.255", "0.0.0.0"},
		},
		{
			Name:        "IPv6Address",
			Pattern:     regexp.MustCompile(`^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`),
			TokenType:   token.TokenIPv6,
			Priority:    PriorityHigh,
			Category:    "network",
			Description: "Matches basic IPv6 addresses",
			Examples:    []string{"2001:0db8:85a3:0000:0000:8a2e:0370:7334", "fe80:0000:0000:0000:0000:0000:0000:0001"},
		},
		{
			Name:        "EmailAddress",
			Pattern:     regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`),
			TokenType:   token.TokenEmail,
			Priority:    PriorityHigh,
			Category:    "network",
			Description: "Matches email addresses",
			Examples:    []string{"user@example.com", "test.email+tag@domain.org", "admin@company.co.uk"},
		},
		{
			Name:        "URI",
			Pattern:     regexp.MustCompile(`^https?://[^\s]+$`),
			TokenType:   token.TokenURI,
			Priority:    PriorityMedium,
			Category:    "network",
			Description: "Matches HTTP and HTTPS URIs",
			Examples:    []string{"http://example.com", "https://api.domain.com/v1/users", "https://cdn.example.org/assets/style.css"},
		},

		// =============================================================================
		// HTTP PATTERNS (Priority: Medium)
		// =============================================================================

		{
			Name:        "HTTPStatus",
			Pattern:     regexp.MustCompile(`^[1-5][0-9][0-9]$`),
			TokenType:   token.TokenHTTPStatus,
			Priority:    PriorityMedium,
			Category:    "http",
			Description: "Matches HTTP status codes",
			Examples:    []string{"200", "404", "500", "301", "403"},
		},

		// =============================================================================
		// FILESYSTEM PATTERNS (Priority: Medium)
		// =============================================================================

		{
			Name:        "AbsolutePath",
			Pattern:     regexp.MustCompile(`^/[^\s]+$`),
			TokenType:   token.TokenAbsolutePath,
			Priority:    PriorityMedium,
			Category:    "filesystem",
			Description: "Matches absolute file/URL paths",
			Examples:    []string{"/api/users", "/var/log/app.log", "/home/user/documents"},
		},

		// =============================================================================
		// NUMERIC PATTERNS (Priority: Low - Fallback)
		// =============================================================================

		{
			Name:        "Numeric",
			Pattern:     regexp.MustCompile(`^\d+$`),
			TokenType:   token.TokenNumeric,
			Priority:    PriorityLow,
			Category:    "numeric",
			Description: "Matches pure numeric values",
			Examples:    []string{"123", "0", "999999", "42"},
		},
	}

	return rules
}
