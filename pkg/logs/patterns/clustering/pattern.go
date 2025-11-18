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

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering/merging"
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
	CreatedAt time.Time // When pattern was first created
	UpdatedAt time.Time // When pattern was last modified
}

// newPattern creates a new pattern from a single token list.
func newPattern(tokenList *token.TokenList, patternID uint64) *Pattern {
	now := time.Now()
	return &Pattern{
		Template:  tokenList, // First log becomes initial template
		Positions: []int{},   // No wildcards yet
		PatternID: patternID,
		Sample:    tokenList, // Store first log as sample
		LogCount:  1,         // First log
		CreatedAt: now,
		UpdatedAt: now,
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

// GetWildcardCount returns the number of wildcard positions in this pattern.
// This matches the ParamCount that will be sent in PatternDefine.
func (p *Pattern) GetWildcardCount() int {
	return len(p.Positions)
}

// GetWildcardCharPositions returns character indices where wildcards appear in the pattern string.
// This matches the PosList that will be sent in PatternDefine.
// Example: "User * logged in from *" returns [7, 12]
func (p *Pattern) GetWildcardCharPositions() []int {
	if p.Template == nil {
		return nil
	}

	var charPositions []int
	currentPos := 0

	for _, tok := range p.Template.Tokens {
		// Clean the token value for proper length calculation
		cleaned := sanitizeForTemplate(tok.Value)

		if tok.Wildcard == token.IsWildcard {
			// Record the current character position for this wildcard
			charPositions = append(charPositions, currentPos)
			// Wildcard is represented as "*" (1 character)
			currentPos += 1
		} else if cleaned != "" {
			// Add the length of the cleaned token value
			currentPos += len(cleaned)
		}
	}

	return charPositions
}

// GetWildcardValues extracts the wildcard values from a specific TokenList.
// This is called per-log to get that log's specific wildcard parameter values.
//
// NOTE: AddTokenListToPatterns now verifies that tokenList matches p.Template before
// assigning it to a pattern, so this function should only be called when structures match.
// However, we keep the defensive check below as a safety measure.
func (p *Pattern) GetWildcardValues(tokenList *token.TokenList) []string {
	if p.Template == nil || len(p.Positions) == 0 {
		return []string{}
	}

	// CRITICAL CHECK: Verify tokenList matches p.Template structure
	// Note: CanMergeTokenLists is not symmetric - template (with IsWildcard) vs tokenList (with PotentialWildcard)
	// works one way but not the other. Since AddTokenListToPatterns already verified compatibility,
	// we check both directions here as a safety measure.
	templateMatches := merging.CanMergeTokenLists(p.Template, tokenList) || merging.CanMergeTokenLists(tokenList, p.Template)
	if !templateMatches {
		// tokenList doesn't match p.Template structure in either direction
		// This shouldn't happen if AddTokenListToPatterns worked correctly, but handle gracefully
		// Return nil slice (not empty slice) to signal mismatch - caller should send raw log
		return nil
	}

	// Ensure lengths match (CanMergeTokenLists already checks this, but be safe)
	if tokenList.Length() != p.Template.Length() {
		// Length mismatch - return nil to signal error
		return nil
	}

	// Preallocate slice with exact size to ensure count matches ParamCount
	wildcardValues := make([]string, len(p.Positions))

	// p.Positions are token indices in p.Template where wildcards are
	// Since tokenList matches p.Template structure (verified above),
	// we can use the same indices to extract values from tokenList
	for i, templatePos := range p.Positions {
		if templatePos < tokenList.Length() {
			wildcardValues[i] = tokenList.Tokens[templatePos].Value
		} else {
			// Position out of bounds - use empty string to maintain count
			// This shouldn't happen if structure matches correctly
			wildcardValues[i] = ""
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
