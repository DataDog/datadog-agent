// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"

	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

// ProcessingRuleType indicates what type of detection the redactor uses, i.e. token-based
type ProcessingRuleType string

const (
	// RuleTypeToken indicates token-based pattern matching
	RuleTypeToken ProcessingRuleType = "token"
)

// TokenBasedProcessingRule represents a single detection and redaction rule
type TokenBasedProcessingRule struct {
	Name         string
	Type         ProcessingRuleType
	TokenPattern []tokens.Token // For token-based rules
	Replacement  []byte
	PrefilterKeywords [][]byte       // Literal strings that must be present for rule to apply
}

type ProcessingRuleApplicatorConfig struct {
}

type ProcessingRuleApplicator struct {
	tokenRules []*TokenBasedProcessingRule // Fast token-based rules
}

// NewRedactor creates a new hybrid PII detector with predefined rules
// filtered by the enabled PII types in the config
func NewRedactor(config ProcessingRuleApplicatorConfig) *ProcessingRuleApplicator {
	applicator := &ProcessingRuleApplicator{
		tokenRules: make([]*TokenBasedProcessingRule, 0),
	}

	allTokenRules := getTokenRules(config)

	applicator.tokenRules = allTokenRules

	return applicator
}

// Apply applies detection and redaction/replacement in a single pass.
// Returns the updated content and a list of rule names that matched.
// This method is thread-safe when each goroutine provides its own tokenizer.
func (h *ProcessingRuleApplicator) Apply(content []byte, tokenizer *automultilinedetection.Tokenizer) ([]byte, []string) {
	if len(content) == 0 {
		return content, nil
	}

	matchedRules := make([]string, 0)
	result := make([]byte, len(content))
	copy(result, content)

	// Pass 1: Fast token-based detection for structured PII
	// Tokenizer is provided by caller to avoid thread-safety issues
	if tokenizer != nil && len(h.tokenRules) > 0 {
		toks, indices := tokenizer.Tokenize(result)
		if len(toks) > 0 {
			result, matchedRules = h.applyTokenRules(result, toks, indices, matchedRules)
		}
	}

	return result, matchedRules
}

// applyTokenRules applies token-based detection rules
func (h *ProcessingRuleApplicator) applyTokenRules(content []byte, toks []tokens.Token, indices []int, matchedRules []string) ([]byte, []string) {
	type match struct {
		start int
		end   int
		rule  *TokenBasedProcessingRule
	}
	matches := make([]match, 0)

	// Find all token pattern matches
	for _, rule := range h.tokenRules {
		patternLen := len(rule.TokenPattern)
		if patternLen == 0 {
			continue
		}

		if !h.hasPrefilterKeywords(content, rule.PrefilterKeywords) {
			continue // skip further token matching
		}

		// Sliding window over tokens
		for i := 0; i <= len(toks)-patternLen; i++ {
			if h.matchesTokenPattern(toks[i:i+patternLen], rule.TokenPattern) {
				// Token pattern matched! Get the byte range
				startIdx := indices[i]
				endIdx := len(content)
				if i+patternLen < len(indices) {
					endIdx = indices[i+patternLen]
				}

				// Check for overlaps with existing matches (keep first match)
				overlaps := false
				for _, existing := range matches {
					if (startIdx >= existing.start && startIdx < existing.end) ||
						(endIdx > existing.start && endIdx <= existing.end) {
						overlaps = true
						break
					}
				}

				if !overlaps {
					matches = append(matches, match{
						start: startIdx,
						end:   endIdx,
						rule:  rule,
					})
				}

				// Skip ahead to avoid overlapping matches within the same pattern
				i += patternLen - 1
			}
		}
	}

	// Sort matches by start position (descending) to replace from end to start
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].start > matches[i].start {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Apply replacements from end to start to maintain indices
	// Work backwards so earlier replacements don't affect later indices
	for _, m := range matches {
		// Build new content: before + replacement + after
		newContent := make([]byte, 0, len(content)-(m.end-m.start)+len(m.rule.Replacement))
		newContent = append(newContent, content[:m.start]...)
		newContent = append(newContent, m.rule.Replacement...)
		newContent = append(newContent, content[m.end:]...)
		content = newContent
		matchedRules = append(matchedRules, m.rule.Name)
	}

	return content, matchedRules
}

// matchesTokenPattern checks if two token sequences match exactly
func (h *ProcessingRuleApplicator) matchesTokenPattern(actual, expected []tokens.Token) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

// hasPrefilterKeywords checks if all required literal strings are present in content
// This is a fast check to avoid expensive token matching when patterns can't possibly match
func (h *ProcessingRuleApplicator) hasPrefilterKeywords(content []byte, keywords [][]byte) bool {
	if len(keywords) == 0 {
		return true // No prefilter requirements
	}
	
	for _, keyword := range keywords {
		if !bytes.Contains(content, keyword) {
			return false // Missing required keyword
		}
	}
	return true
}
