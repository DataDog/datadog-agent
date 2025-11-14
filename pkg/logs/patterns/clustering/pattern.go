// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists
// and identifying wildcard positions for pattern extraction.
package clustering

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Pattern represents a single pattern within a cluster.
// A cluster with the same signature may contain multiple incompatible patterns
// (e.g., different non-identical special characters that cannot merge).
type Pattern struct {
	Template  *token.TokenList // The pattern template with wildcards (matches proto "template")
	Positions []int            // Token indices that are wildcards (matches proto "pos_list")
	PatternID uint64           // Unique pattern ID (matches proto "pattern_id")
	Sample    *token.TokenList // First log sample (for multi-pattern matching)
	LogCount  int              // Total number of logs that matched this pattern

	// Timestamp tracking for stateful encoding
	CreatedAt  time.Time // When pattern was first created
	UpdatedAt  time.Time // When pattern was last modified
	LastSentAt time.Time // When we last sent this pattern to gRPC

	// State tracking for pattern messages
	SentTemplate string // The template string that was last sent (for detecting changes)
}

// newPattern creates a new pattern from a single token list.
func newPattern(tokenList *token.TokenList, patternID uint64) *Pattern {
	now := time.Now()
	return &Pattern{
		Template:   tokenList, // First log becomes initial template
		Positions:  []int{},   // No wildcards yet
		PatternID:  patternID,
		Sample:     tokenList, // Store first log as sample
		LogCount:   1,         // First log
		CreatedAt:  now,
		UpdatedAt:  now,
		LastSentAt: time.Time{}, // Zero time - never sent
	}
}

// size returns the number of logs in this pattern.
func (p *Pattern) size() int {
	return p.LogCount
}

// GetPatternString returns the pattern template as a string with wildcards marked as "*"
func (p *Pattern) GetPatternString() string {
	if p.Template == nil {
		return ""
	}

	var parts []string
	for _, tok := range p.Template.Tokens {
		// Use "*" for wildcard positions, actual value otherwise
		if tok.Wildcard == token.IsWildcard {
			parts = append(parts, "*")
		} else {
			// Only use printable ASCII/UTF-8 characters in the template
			cleaned := sanitizeForTemplate(tok.Value)
			if cleaned != "" {
				parts = append(parts, cleaned)
			}
		}
	}
	return strings.Join(parts, "")
}

// hasWildcards returns true if this pattern contains wildcard positions.
func (p *Pattern) hasWildcards() bool {
	return len(p.Positions) > 0
}

// GetWildcardValues extracts the wildcard values from a specific TokenList.
// This is called per-log to get that log's specific wildcard parameter values.
func (p *Pattern) GetWildcardValues(tokenList *token.TokenList) []string {
	if p.Template == nil || len(p.Positions) == 0 {
		return []string{}
	}

	wildcardValues := make([]string, 0, len(p.Positions))
	for _, pos := range p.Positions {
		if pos < tokenList.Length() {
			wildcardValues = append(wildcardValues, tokenList.Tokens[pos].Value)
		}
	}

	return wildcardValues
}

// sanitizeForTemplate removes non-printable characters from template strings
func sanitizeForTemplate(s string) string {
	runes := []rune(s)
	result := make([]rune, 0, len(runes))
	for _, r := range runes {
		// Keep only printable characters (space and above, excluding DEL)
		if r >= ' ' && r != 0x7F && r < 0xFFFD {
			result = append(result, r)
		}
	}
	return string(result)
}
