// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automaton provides log message tokenization using finite state automaton
// and trie-based pattern matching for token classification.
package automaton

import (
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TrieNode represents a node in the classification trie
type TrieNode struct {
	children   map[rune]*TrieNode
	tokenType  token.TokenType
	isTerminal bool
}

// Trie implements a prefix tree for token classification
type Trie struct {
	root          *TrieNode
	terminalRules []*TerminalRule
}

// GlobalRuleManager manages terminal rules
var globalRuleManager *RuleManager

// globalTrie is the singleton trie instance
var globalTrie *Trie

// init initializes the global trie and rule manager
// todo: componentilize this eventually
func init() {
	globalTrie = NewTrie()
	globalRuleManager = NewRuleManager()
	globalRuleManager.LoadPredefinedRules()
	globalTrie.buildPredefinedPatterns()
}

// NewTrie creates a new trie
func NewTrie() *Trie {
	return &Trie{
		root: &TrieNode{
			children: make(map[rune]*TrieNode),
		},
		terminalRules: make([]*TerminalRule, 0),
	}
}

// Match performs token classification
func (trie *Trie) Match(value string) token.TokenType {
	if len(value) == 0 {
		return token.TokenUnknown
	}

	if tokenType := trie.exactMatch(value); tokenType != token.TokenUnknown {
		return tokenType
	}

	return trie.applyTerminalRules(value)
}

// exactMatch performs exact string matching
func (trie *Trie) exactMatch(value string) token.TokenType {
	node := trie.root

	for _, char := range value {
		child, exists := node.children[char]
		if !exists {
			return token.TokenUnknown
		}
		node = child
	}

	if node.isTerminal {
		return node.tokenType
	}

	return token.TokenUnknown
}

// applyTerminalRules applies regex-based terminal rules
func (trie *Trie) applyTerminalRules(value string) token.TokenType {
	return globalRuleManager.ApplyRules(value)
}

// AddExactPattern adds an exact string pattern to the trie
func (trie *Trie) AddExactPattern(pattern string, tokenType token.TokenType) {
	node := trie.root

	for _, char := range pattern {
		if _, exists := node.children[char]; !exists {
			node.children[char] = &TrieNode{
				children: make(map[rune]*TrieNode),
			}
		}
		node = node.children[char]
	}

	node.isTerminal = true
	node.tokenType = tokenType
}

// AddTerminalRule adds a regex-based pattern rule
func (trie *Trie) AddTerminalRule(pattern string, tokenType token.TokenType, priority int) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	rule := &TerminalRule{
		Name:        "AnonymousRule",
		Pattern:     regex,
		TokenType:   tokenType,
		Priority:    priority,
		Category:    "default",
		Description: "Anonymous terminal rule",
		Examples:    []string{},
	}

	inserted := false
	for i, existing := range trie.terminalRules {
		if priority > existing.Priority {
			trie.terminalRules = append(trie.terminalRules[:i], append([]*TerminalRule{rule}, trie.terminalRules[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		trie.terminalRules = append(trie.terminalRules, rule)
	}

	return nil
}

// buildPredefinedPatterns populates the trie with predefined patterns
func (trie *Trie) buildPredefinedPatterns() {
	httpMethods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "TRACE", "CONNECT"}
	for _, method := range httpMethods {
		trie.AddExactPattern(method, token.TokenHttpMethod)
	}

	severityLevels := []string{"TRACE", "DEBUG", "INFO", "WARN", "WARNING", "ERROR", "FATAL", "PANIC", "EMERGENCY", "ALERT", "CRITICAL", "NOTICE"}
	for _, level := range severityLevels {
		trie.AddExactPattern(level, token.TokenSeverityLevel)
		trie.AddExactPattern(strings.ToLower(level), token.TokenSeverityLevel)
	}

	trie.AddExactPattern(" ", token.TokenWhitespace)
	trie.AddExactPattern("\t", token.TokenWhitespace)
	trie.AddExactPattern("\n", token.TokenWhitespace)
	trie.AddExactPattern("\r\n", token.TokenWhitespace)

	trie.AddTerminalRule(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`, token.TokenIPv4, PriorityHigh)
	trie.AddTerminalRule(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, token.TokenEmail, PriorityHigh)
	trie.AddTerminalRule(`^https?://[^\s]+$`, token.TokenURI, PriorityMedium)
	trie.AddTerminalRule(`^\d{4}-\d{2}-\d{2}`, token.TokenDate, PriorityMedium)
	trie.AddTerminalRule(`^\d{2}/\d{2}/\d{4}`, token.TokenDate, PriorityMedium)
	trie.AddTerminalRule(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, token.TokenDate, PriorityMedium)
	trie.AddTerminalRule(`^[1-5][0-9][0-9]$`, token.TokenHttpStatus, PriorityMedium)
	trie.AddTerminalRule(`^/[^\s]+$`, token.TokenAbsolutePath, PriorityMedium)
	trie.AddTerminalRule(`^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`, token.TokenIPv6, PriorityHigh)
	trie.AddTerminalRule(`^\d+$`, token.TokenNumeric, PriorityLow)
}

// TokenizeString is the main entry point
func TokenizeString(input string) *token.TokenList {
	if len(input) == 0 {
		return token.NewTokenList()
	}

	tokenizer := NewTokenizer(input)
	return tokenizer.Tokenize()
}

// Helper functions

// GetTerminalRules returns all terminal rules
func GetTerminalRules() []*TerminalRule {
	return globalRuleManager.ListRules()
}

// GetRulesByCategory returns rules by category
func GetRulesByCategory(category string) []*TerminalRule {
	return globalRuleManager.GetRulesByCategory(category)
}

// GetRuleCategories returns all rule categories
func GetRuleCategories() []string {
	return globalRuleManager.GetCategories()
}

// AddTerminalRule adds a new terminal rule
func AddTerminalRule(name, pattern, category, description string, tokenType token.TokenType, priority int, examples []string) error {
	return globalRuleManager.AddRule(name, pattern, category, description, tokenType, priority, examples)
}

// GetRuleStats returns rule statistics
func GetRuleStats() RuleStats {
	return globalRuleManager.GetRuleStats()
}

// Statistics

// Stats returns trie statistics
type TrieStats struct {
	ExactPatterns int
	TerminalRules int
	TrieNodes     int
	MaxDepth      int
}

// GetStats returns trie statistics
func (trie *Trie) GetStats() TrieStats {
	nodeCount, maxDepth := trie.countNodes(trie.root, 0)

	return TrieStats{
		ExactPatterns: trie.countExactPatterns(trie.root),
		TerminalRules: len(trie.terminalRules),
		TrieNodes:     nodeCount,
		MaxDepth:      maxDepth,
	}
}

func (trie *Trie) countNodes(node *TrieNode, depth int) (int, int) {
	count := 1
	maxDepth := depth

	for _, child := range node.children {
		childCount, childDepth := trie.countNodes(child, depth+1)
		count += childCount
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}

	return count, maxDepth
}

func (trie *Trie) countExactPatterns(node *TrieNode) int {
	count := 0
	if node.isTerminal {
		count = 1
	}

	for _, child := range node.children {
		count += trie.countExactPatterns(child)
	}

	return count
}

// Validation helpers

// validateIPv4 validates IPv4 addresses
func validateIPv4(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}

		// Convert to number and check range
		num := 0
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
			num = num*10 + int(char-'0')
		}

		if num > 255 {
			return false
		}
	}
	return true
}

// validateEmail validates email addresses
func validateEmail(value string) bool {
	atCount := strings.Count(value, "@")
	if atCount != 1 {
		return false
	}

	parts := strings.Split(value, "@")
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
		return false
	}

	return strings.Contains(parts[1], ".")
}

// validateDate validates date strings
func validateDate(value string) bool {
	hasDateChars := strings.Contains(value, "-") || strings.Contains(value, ":") || strings.Contains(value, "/")
	if !hasDateChars {
		return false
	}

	hasDigits := false
	for _, char := range value {
		if char >= '0' && char <= '9' {
			hasDigits = true
			break
		}
	}

	return hasDigits && len(value) >= 8 && len(value) <= 64
}
