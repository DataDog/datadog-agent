// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automaton provides log message tokenization using finite state automaton
// and trie-based pattern matching for token classification.
package automaton

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TrieNode represents a node in the classification trie
type TrieNode struct {
	children   map[rune]*TrieNode
	tokenType  token.TokenType
	isTerminal bool
}

// Trie implements a prefix tree for token classification
type Trie struct {
	root *TrieNode
}

// GlobalRuleManager manages terminal rules
var globalRuleManager *RuleManager

// globalTrie is the singleton trie instance
var globalTrie *Trie

// init initializes the global trie and rule manager
// todo: componentize this eventually
func init() {
	globalTrie = NewTrie()
	globalRuleManager = NewRuleManager()
	if err := globalRuleManager.LoadPredefinedRules(); err != nil {
		log.Error(err)
	}
	globalTrie.buildPredefinedPatterns()
}

// NewTrie creates a new trie
func NewTrie() *Trie {
	return &Trie{
		root: &TrieNode{
			children: make(map[rune]*TrieNode),
		},
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

// buildPredefinedPatterns populates the trie with exact-match patterns
// for fast classification of known strings (HTTP methods, severity levels, whitespace).
// Regex-based terminal rules are handled by globalRuleManager via LoadPredefinedRules().
func (trie *Trie) buildPredefinedPatterns() {
	// HTTP methods - exact string matching
	httpMethods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "TRACE", "CONNECT"}
	for _, method := range httpMethods {
		trie.AddExactPattern(method, token.TokenHTTPMethod)
	}

	// Severity levels - exact string matching (both uppercase and lowercase)
	severityLevels := []string{"TRACE", "DEBUG", "INFO", "WARN", "WARNING", "ERROR", "FATAL", "PANIC", "EMERGENCY", "ALERT", "CRITICAL", "NOTICE"}
	for _, level := range severityLevels {
		trie.AddExactPattern(level, token.TokenSeverityLevel)
		trie.AddExactPattern(strings.ToLower(level), token.TokenSeverityLevel)
	}

	// Whitespace - exact character matching
	trie.AddExactPattern(" ", token.TokenWhitespace)
	trie.AddExactPattern("\t", token.TokenWhitespace)
	trie.AddExactPattern("\n", token.TokenWhitespace)
	trie.AddExactPattern("\r\n", token.TokenWhitespace)
}

// TokenizeString is the main entry point
func TokenizeString(input string) *token.TokenList {
	if len(input) == 0 {
		return token.NewTokenList()
	}

	tokenizer := newTokenizerInternal(input)
	tokenList := tokenizer.tokenize()
	tokenizer.Release()
	return tokenList
}

// Statistics

// TrieStats is the stats of the trie
type TrieStats struct {
	ExactPatterns int
	TerminalRules int
	TrieNodes     int
	MaxDepth      int
}

// GetStats returns trie statistics for testing purposes
func (trie *Trie) GetStats() TrieStats {
	nodeCount, maxDepth := trie.countNodes(trie.root, 0)

	// Terminal rules are managed by globalRuleManager, not the trie itself
	terminalRuleCount := 0
	if globalRuleManager != nil {
		terminalRuleCount = len(globalRuleManager.rules)
	}

	return TrieStats{
		ExactPatterns: trie.countExactPatterns(trie.root),
		TerminalRules: terminalRuleCount,
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
