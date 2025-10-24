// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"regexp"

	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

// PIIRuleType indicates whether a rule uses token-based or regex-based detection
type PIIRuleType string

const (
	// PIIRuleTypeToken indicates fast token-based pattern matching
	PIIRuleTypeToken PIIRuleType = "token"
	// PIIRuleTypeRegex indicates regex-based pattern matching
	PIIRuleTypeRegex PIIRuleType = "regex"
)

// PIIDetectionRule represents a single PII detection and redaction rule
type PIIDetectionRule struct {
	Name              string
	Type              PIIRuleType
	TokenPattern      []tokens.Token // For token-based rules
	Regex             *regexp.Regexp // For regex-based rules
	RegexConfirmation *regexp.Regexp // Optional: regex to confirm token match (for ambiguous patterns)
	Replacement       []byte
}

// PIITypeConfig holds configuration for which PII types are enabled
type PIITypeConfig struct {
	Email      bool
	CreditCard bool
	SSN        bool
	Phone      bool
	IP         bool
}

// HybridPIIDetector combines fast token-based detection for structured PII
// with regex fallback for variable/complex patterns
type HybridPIIDetector struct {
	tokenRules []*PIIDetectionRule // Fast token-based rules (SSN, formatted cards/phones)
	regexRules []*PIIDetectionRule // Regex fallback rules (emails, IPs, unformatted patterns)
}

// NewHybridPIIDetector creates a new hybrid PII detector with predefined rules
// filtered by the enabled PII types in the config
func NewHybridPIIDetector(config PIITypeConfig) *HybridPIIDetector {
	detector := &HybridPIIDetector{
		tokenRules: make([]*PIIDetectionRule, 0),
		regexRules: make([]*PIIDetectionRule, 0),
	}

	// Token-based rules for structured PII (fast path)
	// These run first because they're faster than regex
	allTokenRules := []*PIIDetectionRule{}

	// Add rules based on enabled PII types
	if config.SSN {
		allTokenRules = append(allTokenRules, getSSNRules()...)
	}
	if config.CreditCard {
		allTokenRules = append(allTokenRules, getCreditCardRules()...)
	}
	if config.Phone {
		allTokenRules = append(allTokenRules, getPhoneRules()...)
	}

	detector.tokenRules = allTokenRules

	// Regex-based rules for variable/complex PII (fallback path)
	// These handle patterns that are too variable for token matching
	allRegexRules := []*PIIDetectionRule{}

	if config.Email {
		allRegexRules = append(allRegexRules, getEmailRules()...)
	}
	if config.IP {
		allRegexRules = append(allRegexRules, getIPRules()...)
	}

	detector.regexRules = allRegexRules

	return detector
}

// Redact applies PII detection and redaction using the hybrid approach in a single pass.
// Returns the redacted content and a list of rule names that matched.
// This method is thread-safe when each goroutine provides its own tokenizer.
func (h *HybridPIIDetector) Redact(content []byte, tokenizer *automultilinedetection.Tokenizer) ([]byte, []string) {
	if len(content) == 0 {
		return content, nil
	}

	matchedRules := make([]string, 0)
	result := make([]byte, len(content))
	copy(result, content)

	// Pass 1: Fast token-based detection for structured PII
	// Tokenizer is provided by caller to avoid thread-safety issues
	if tokenizer != nil && len(h.tokenRules) > 0 {
		toks, indices := tokenizer.TokenizeBytes(result)
		if len(toks) > 0 {
			result, matchedRules = h.applyTokenRules(result, toks, indices, matchedRules)
		}
	}

	// Pass 2: Regex-based detection for variable/complex PII
	if len(h.regexRules) > 0 {
		result, matchedRules = h.applyRegexRules(result, matchedRules)
	}

	return result, matchedRules
}

// applyTokenRules applies token-based detection rules with optional regex confirmation
func (h *HybridPIIDetector) applyTokenRules(content []byte, toks []tokens.Token, indices []int, matchedRules []string) ([]byte, []string) {
	type match struct {
		start int
		end   int
		rule  *PIIDetectionRule
	}
	matches := make([]match, 0)

	// Find all token pattern matches
	for _, rule := range h.tokenRules {
		patternLen := len(rule.TokenPattern)
		if patternLen == 0 {
			continue
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

				// If this rule has a regex confirmation, apply it
				if rule.RegexConfirmation != nil {
					matchedBytes := content[startIdx:endIdx]
					if !rule.RegexConfirmation.Match(matchedBytes) {
						continue // Confirmation failed, skip this match
					}
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

// applyRegexRules applies regex-based detection rules
func (h *HybridPIIDetector) applyRegexRules(content []byte, matchedRules []string) ([]byte, []string) {
	for _, rule := range h.regexRules {
		if rule.Regex.Match(content) {
			originalContent := content
			content = rule.Regex.ReplaceAll(content, rule.Replacement)

			// Only record the rule if content was actually modified
			if !bytes.Equal(originalContent, content) {
				matchedRules = append(matchedRules, rule.Name)
			}
		}
	}
	return content, matchedRules
}

// matchesTokenPattern checks if two token sequences match exactly
func (h *HybridPIIDetector) matchesTokenPattern(actual, expected []tokens.Token) bool {
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
