// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists,
// extracting patterns wildcard, and managing pattern lifecycle through eviction policies.
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

	// Timestamp tracking for stateful encoding and eviction
	CreatedAt    time.Time // When pattern was first created (used for age-based decay)
	UpdatedAt    time.Time // When pattern was last modified (structure changed)
	LastAccessAt time.Time // When pattern last matched a log (used for recency in eviction)
}

// newPattern creates a new pattern from a single token list.
func newPattern(tokenList *token.TokenList, patternID uint64) *Pattern {
	now := time.Now()
	return &Pattern{
		Template:     tokenList, // First log becomes initial template
		Positions:    []int{},   // No wildcards yet
		PatternID:    patternID,
		Sample:       tokenList, // Store first log as sample
		LogCount:     1,         // First log (HitCount for eviction)
		CreatedAt:    now,       // Pattern birth time (for age decay)
		UpdatedAt:    now,       // Last structure modification
		LastAccessAt: now,       // Last match time (for recency in eviction)
	}
}

// size returns the number of logs in this pattern.
func (p *Pattern) size() int {
	return p.LogCount
}

// EstimatedBytes returns an approximate memory footprint (in bytes) of this pattern.
//
// This is NOT exact Go heap usage; it is a heuristic used to trigger eviction before unbounded growth.
// It focuses on the dominant contributors: token value strings, wildcard positions, and token slices.
func (p *Pattern) EstimatedBytes() int64 {
	var b int64

	// Positions slice (ints)
	b += int64(len(p.Positions)) * 8

	// Estimate token lists (avoid double counting if Sample == Template)
	b += estimateTokenListBytes(p.Template)
	if p.Sample != nil && p.Sample != p.Template {
		b += estimateTokenListBytes(p.Sample)
	}

	return b
}

func estimateTokenListBytes(tl *token.TokenList) int64 {
	if tl == nil {
		return 0
	}

	var b int64
	// Token slice header/struct overhead is ignored; we approximate dominant string storage.
	for _, tok := range tl.Tokens {
		b += int64(len(tok.Value))
	}
	return b
}

// GetPatternString returns the pattern template.
// Pattern template has no wildcard placeholders and wildcard tokens are completely omitted
func (p *Pattern) GetPatternString() string {
	if p.Template == nil {
		return ""
	}

	var parts []string
	for _, tok := range p.Template.Tokens {
		// Skip wildcard tokens entirely
		if tok.Wildcard == token.IsWildcard {
			continue
		}
		// Only use printable ASCII/UTF-8 characters in the template
		cleaned := sanitizeForTemplate(tok.Value)
		if cleaned != "" {
			parts = append(parts, cleaned)
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

// GetWildcardCharPositions returns character indices where dynamic values should be injected.
// The template does NOT contain wildcard placeholders - wildcards are omitted entirely.
// Positions mark the injection points in the template string.
// Example: Template "User  logged" (wildcard omitted) returns [5] (inject after "User ")
func (p *Pattern) GetWildcardCharPositions() []int {
	if p.Template == nil {
		return nil
	}

	var charPositions []int
	currentPos := 0

	for _, tok := range p.Template.Tokens {
		cleaned := sanitizeForTemplate(tok.Value)

		if tok.Wildcard == token.IsWildcard {
			// Mark the injection point (current position in template which excludes wildcards)
			charPositions = append(charPositions, currentPos)
			// Wildcard tokens are NOT in the template, so don't advance currentPos
		} else if cleaned != "" {
			// Add the length of the cleaned token value
			currentPos += len(cleaned)
		}
	}

	return charPositions
}

// GetWildcardValues extracts the wildcard values from a specific TokenList.
func (p *Pattern) GetWildcardValues(tokenList *token.TokenList) []string {
	if p.Template == nil || len(p.Positions) == 0 {
		return []string{}
	}

	// Check if tokenList matches p.Template structure
	templateMatches := merging.CanMergeTokenLists(p.Template, tokenList) || merging.CanMergeTokenLists(tokenList, p.Template)
	if !templateMatches {
		return nil
	}

	wildcardValues := make([]string, len(p.Positions))

	for i, templatePos := range p.Positions {
		if templatePos < tokenList.Length() {
			wildcardValues[i] = tokenList.Tokens[templatePos].Value
		} else {
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
