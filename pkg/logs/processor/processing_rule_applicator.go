// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/tokens"
)

// ProcessingRuleApplicator applies token-based processing rules
type ProcessingRuleApplicator struct {
	rules []*config.ProcessingRule
}

// NewApplicator creates a new processing rule applicator
func NewApplicator(rules []*config.ProcessingRule) *ProcessingRuleApplicator {
	return &ProcessingRuleApplicator{
		rules: rules,
	}
}

// Apply applies detection and redaction/replacement in a single pass.
// Returns the updated content and a list of rule names that matched.
// This method is thread-safe when each goroutine provides its own tokenizer.
func (p *ProcessingRuleApplicator) Apply(content []byte, tokenizer *automultilinedetection.Tokenizer, ruleType string) ([]byte, []string) {
	if len(content) == 0 {
		return content, nil
	}

	matchedRules := make([]string, 0)
	result := make([]byte, len(content))
	copy(result, content)

	// Tokenize once
	if tokenizer != nil && len(p.rules) > 0 {
		toks, indices := tokenizer.Tokenize(result)
		if len(toks) > 0 {
			result, matchedRules = p.applyRules(result, toks, indices, matchedRules, ruleType)
		}
	}

	return result, matchedRules
}

// applyRules applies mask_sequences rules with overlap detection
func (p *ProcessingRuleApplicator) applyRules(content []byte, toks []tokens.Token, indices []int, matchedRules []string, ruleType string) ([]byte, []string) {
	type match struct {
		start int
		end   int
		rule  *config.ProcessingRule
	}
	matches := make([]match, 0)

	// Find all token pattern matches for mask_sequences rules
	for _, rule := range p.rules {
		if rule.Type != ruleType {
			continue
		}

		patternLen := len(rule.TokenPattern)
		if patternLen == 0 {
			continue
		}

		// Prefilter check
		if !hasPrefilterKeywords(content, rule.PrefilterKeywordsRaw) {
			continue
		}

		// Sliding window over tokens
		for i := 0; i <= len(toks)-patternLen; i++ {
			if matchesTokenPatternWithConstraints(toks[i:i+patternLen], rule.TokenPattern, rule.LengthConstraints) {
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
	for _, m := range matches {
		newContent := make([]byte, 0, len(content)-(m.end-m.start)+len(m.rule.Placeholder))
		newContent = append(newContent, content[:m.start]...)
		newContent = append(newContent, m.rule.Placeholder...)
		newContent = append(newContent, content[m.end:]...)
		content = newContent
		matchedRules = append(matchedRules, m.rule.Name)
	}

	return content, matchedRules
}

// matchesTokenPatternWithConstraints checks if actual tokens match the expected pattern
// and validates length constraints for variable-length patterns (e.g., IPv4)
func matchesTokenPatternWithConstraints(actual, expected []tokens.Token, constraints []config.LengthConstraint) bool {
	if len(actual) != len(expected) {
		return false
	}

	for i := range actual {
		if !actual[i].Equals(expected[i]) {
			return false
		}

		// Validate length constraints (only if token has a literal)
		if actual[i].Lit != "" {
			for _, constraint := range constraints {
				if i == constraint.TokenIndex {
					length := len(actual[i].Lit)
					if length < constraint.MinLength || length > constraint.MaxLength {
						return false // Length out of range
					}
				}
			}
		}
	}
	return true
}
