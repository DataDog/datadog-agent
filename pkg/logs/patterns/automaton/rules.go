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
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern '%s': %v", pattern, err)
	}

	rule := &TerminalRule{
		Name:        name,
		Pattern:     regex,
		TokenType:   tokenType,
		Priority:    priority,
		Category:    category,
		Description: description,
		Examples:    examples,
	}

	for _, example := range examples {
		if !regex.MatchString(example) {
			return fmt.Errorf("example '%s' does not match pattern '%s'", example, pattern)
		}
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

// Helper methods

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
		{
			Name:        "ISO8601Date",
			Pattern:     regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`),
			TokenType:   token.TokenDate,
			Priority:    PriorityMedium,
			Category:    "time",
			Description: "Matches ISO 8601 datetime format",
			Examples:    []string{"2023-12-25T14:30:00", "2023-01-01T00:00:00Z", "2023-06-15T09:30:00.123Z"},
		},
		{
			Name:        "SimpleDate",
			Pattern:     regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
			TokenType:   token.TokenDate,
			Priority:    PriorityMedium,
			Category:    "time",
			Description: "Matches simple YYYY-MM-DD date format",
			Examples:    []string{"2023-12-25", "2023-01-01", "2024-02-29"},
		},
		{
			Name:        "USDate",
			Pattern:     regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`),
			TokenType:   token.TokenDate,
			Priority:    PriorityMedium,
			Category:    "time",
			Description: "Matches US date format MM/DD/YYYY",
			Examples:    []string{"12/25/2023", "01/01/2024", "02/29/2024"},
		},
		{
			Name:        "HTTPStatus",
			Pattern:     regexp.MustCompile(`^[1-5][0-9][0-9]$`),
			TokenType:   token.TokenHttpStatus,
			Priority:    PriorityMedium,
			Category:    "http",
			Description: "Matches HTTP status codes",
			Examples:    []string{"200", "404", "500", "301", "403"},
		},
		{
			Name:        "AbsolutePath",
			Pattern:     regexp.MustCompile(`^/[^\s]+$`),
			TokenType:   token.TokenAbsolutePath,
			Priority:    PriorityMedium,
			Category:    "filesystem",
			Description: "Matches absolute file/URL paths",
			Examples:    []string{"/api/users", "/var/log/app.log", "/home/user/documents"},
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

// GetRuleByPriority returns rules with a specific priority
func (rm *RuleManager) GetRuleByPriority(priority int) []*TerminalRule {
	result := make([]*TerminalRule, 0)
	for _, rule := range rm.rules {
		if rule.Priority == priority {
			result = append(result, rule)
		}
	}
	return result
}

// GetHighestPriorityRules returns rules with the highest priority
func (rm *RuleManager) GetHighestPriorityRules() []*TerminalRule {
	if len(rm.rules) == 0 {
		return []*TerminalRule{}
	}

	highestPriority := rm.rules[0].Priority
	result := make([]*TerminalRule, 0)

	for _, rule := range rm.rules {
		if rule.Priority == highestPriority {
			result = append(result, rule)
		} else {
			break // Rules are sorted by priority
		}
	}
	return result
}

// UpdateRulePriority changes the priority of an existing rule
func (rm *RuleManager) UpdateRulePriority(name string, newPriority int) error {
	rule := rm.GetRule(name)
	if rule == nil {
		return fmt.Errorf("rule '%s' not found", name)
	}

	// Remove the rule and re-add with new priority
	if !rm.RemoveRule(name) {
		return fmt.Errorf("failed to remove rule '%s'", name)
	}

	return rm.AddRule(
		rule.Name,
		rule.Pattern.String(),
		rule.Category,
		rule.Description,
		rule.TokenType,
		newPriority,
		rule.Examples,
	)
}

// GetCategoryDescription returns the description for a category
func (rm *RuleManager) GetCategoryDescription(category string) string {
	if cat, exists := rm.categories[category]; exists {
		return cat.Description
	}
	return ""
}

// SetCategoryDescription updates the description for a category
func (rm *RuleManager) SetCategoryDescription(category, description string) {
	if rm.categories[category] == nil {
		rm.categories[category] = &RuleCategory{
			Name:        category,
			Description: description,
			Rules:       make([]*TerminalRule, 0),
		}
	} else {
		rm.categories[category].Description = description
	}
}
