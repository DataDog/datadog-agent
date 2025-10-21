// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
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

// HybridPIIDetector combines fast token-based detection for structured PII
// with regex fallback for variable/complex patterns
type HybridPIIDetector struct {
	tokenRules []*PIIDetectionRule // Fast token-based rules (SSN, formatted cards/phones)
	regexRules []*PIIDetectionRule // Regex fallback rules (emails, IPs, unformatted patterns)
}

// NewHybridPIIDetector creates a new hybrid PII detector with predefined rules
func NewHybridPIIDetector() *HybridPIIDetector {
	detector := &HybridPIIDetector{
		tokenRules: make([]*PIIDetectionRule, 0),
		regexRules: make([]*PIIDetectionRule, 0),
	}

	// Token-based rules for structured PII (fast path)
	// These run first because they're faster than regex
	detector.tokenRules = []*PIIDetectionRule{
		// SSN with dashes: DDD-DD-DDDD (unambiguous pattern, no confirmation needed)
		{
			Name: "auto_redact_ssn",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Dash, tokens.D2, tokens.Dash, tokens.D4,
			},
			Replacement: []byte("[SSN_REDACTED]"),
		},
		// SSN with numbers only: DDDDDDDDD 
		{
			Name: "auto_redact_ssn_numbers_only",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D9,
			},
			Replacement: []byte("[SSN_REDACTED]"),
		},
		// SSN with spaces: DDD DD DDDD
		{
			Name: "auto_redact_ssn_spaces",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Space, tokens.D2, tokens.Space, tokens.D4,
			},
			Replacement: []byte("[SSN_REDACTED]"),
		},
		// Credit Card with dashes: DDDD-DDDD-DDDD-DDDD
		{
			Name: "auto_redact_credit_card_dashed",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D4, tokens.Dash, tokens.D4, tokens.Dash, tokens.D4, tokens.Dash, tokens.D4,
			},
			Replacement: []byte("[CC_REDACTED]"),
		},
		// Credit Card with spaces: DDDD DDDD DDDD DDDD
		{
			Name: "auto_redact_credit_card_spaced",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D4, tokens.Space, tokens.D4, tokens.Space, tokens.D4, tokens.Space, tokens.D4,
			},
			Replacement: []byte("[CC_REDACTED]"),
		},
		// Phone with parentheses: (DDD) DDD-DDDD
		// Regex confirms: area code starts with 2-9 (not 0 or 1, which would be invalid)
		{
			Name: "auto_redact_phone_parens",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.Parenopen, tokens.D3, tokens.Parenclose, tokens.Space, tokens.D3, tokens.Dash, tokens.D4,
			},
			RegexConfirmation: regexp.MustCompile(`^\([2-9][0-9]{2}\) [0-9]{3}-[0-9]{4}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
		// Phone with dashes: DDD-DDD-DDDD
		// Regex confirms: this is a phone (area code 2-9), not a timestamp (would start with 0-1)
		{
			Name: "auto_redact_phone_dashed",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Dash, tokens.D3, tokens.Dash, tokens.D4,
			},
			RegexConfirmation: regexp.MustCompile(`^[2-9][0-9]{2}-[0-9]{3}-[0-9]{4}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
		// Phone with dots: DDD.DDD.DDDD
		// Regex confirms: phone, not version number or partial IP
		{
			Name: "auto_redact_phone_dotted",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Period, tokens.D3, tokens.Period, tokens.D4,
			},
			RegexConfirmation: regexp.MustCompile(`^[2-9][0-9]{2}\.[0-9]{3}\.[0-9]{4}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
		// Phone unformatted: D10 = 10 consecutive digits
		// Regex confirms: area code starts with 2-9 (not 0 or 1)
		{
			Name: "auto_redact_phone_unformatted",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D10,
			},
			RegexConfirmation: regexp.MustCompile(`^[2-9][0-9]{9}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
	}

	// Regex-based rules for variable/complex PII (fallback path)
	// These handle patterns that are too variable for token matching
	detector.regexRules = []*PIIDetectionRule{
		// Email: too variable for token matching (many TLD lengths, subdomain variations)
		{
			Name:        "auto_redact_email",
			Type:        PIIRuleTypeRegex,
			Regex:       regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
			Replacement: []byte("[EMAIL_REDACTED]"),
		},
		// IPv4: variable digit lengths (D.D.D.D to DDD.DDD.DDD.DDD), hard for tokens
		{
			Name:        "auto_redact_ipv4",
			Type:        PIIRuleTypeRegex,
			Regex:       regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
			Replacement: []byte("[IP_REDACTED]"),
		},
	}

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
		newContent := make([]byte, 0, len(content)-( m.end-m.start)+len(m.rule.Replacement))
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
