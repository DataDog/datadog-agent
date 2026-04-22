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
	"unicode/utf8"

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

	// Saturation tracking: once a pattern has converged (all variable positions are wildcards),
	// TryMergeTokenLists returns the template pointer unchanged (zero-alloc identical path).
	// After saturatedThreshold consecutive identical merges, CanMergeTokenLists pre-check is skipped.
	consecutiveIdenticalMerges int
	saturated                  bool
}

// newPattern creates a new pattern from a single token list.
func newPattern(tokenList *token.TokenList, patternID uint64) *Pattern {
	now := time.Now()
	return &Pattern{
		Template:     tokenList, // First log becomes initial template
		Positions:    []int{},   // No wildcards yet
		PatternID:    patternID,
		Sample:       tokenList, // Store first log as sample
		LogCount:     1,         // First log
		CreatedAt:    now,       // Pattern birth time (for age decay)
		UpdatedAt:    now,       // Last structure modification
		LastAccessAt: now,       // Last match time (for recency in eviction)
	}
}

// GetFrequency returns the usage frequency for eviction scoring
func (p *Pattern) GetFrequency() float64 {
	return float64(p.LogCount)
}

// GetCreatedAt returns when this pattern was created
func (p *Pattern) GetCreatedAt() time.Time {
	return p.CreatedAt
}

// GetLastAccessAt returns when this pattern was last accessed
func (p *Pattern) GetLastAccessAt() time.Time {
	return p.LastAccessAt
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

	var builder strings.Builder
	estimatedLen := 0
	for _, tok := range p.Template.Tokens {
		// Skip wildcard tokens entirely
		if tok.Wildcard == token.IsWildcard {
			continue
		}
		estimatedLen += len(tok.Value)
	}
	builder.Grow(estimatedLen)

	for _, tok := range p.Template.Tokens {
		if tok.Wildcard == token.IsWildcard {
			continue
		}
		sanitizeForTemplateInto(&builder, tok.Value)
	}

	return builder.String()
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
		if tok.Wildcard == token.IsWildcard {
			// Mark the injection point (current position in template which excludes wildcards)
			charPositions = append(charPositions, currentPos)
			// Wildcard tokens are NOT in the template, so don't advance currentPos
		} else {
			// Use rune count (not byte count) so positions match Java String indices.
			// Java String.length() returns UTF-16 code units; for BMP characters
			// (U+0000–U+FFFF, which covers all common log content including →, ≥, etc.)
			// this equals Unicode codepoint count.
			currentPos += sanitizeForTemplateRuneLen(tok.Value)
		}
	}

	return charPositions
}

// GetWildcardValues extracts the wildcard values from a specific TokenList.
// The caller (processMessage) must ensure tokenList is compatible with this pattern's
// template — this was verified during AddTokenListToPatterns/TryMergeTokenLists.
func (p *Pattern) GetWildcardValues(tokenList *token.TokenList) []string {
	if p.Template == nil || len(p.Positions) == 0 {
		return []string{}
	}

	n := len(tokenList.Tokens)
	wildcardValues := make([]string, len(p.Positions))

	for i, templatePos := range p.Positions {
		if templatePos < n {
			wildcardValues[i] = tokenList.Tokens[templatePos].Value
		}
	}

	return wildcardValues
}

// sanitizeForTemplate removes non-printable characters from template strings
func sanitizeForTemplate(s string) string {
	var builder strings.Builder
	written := sanitizeForTemplateInto(&builder, s)
	if written == len(s) {
		return s
	}
	return builder.String()
}

// sanitizeForTemplateLen returns the byte length of the sanitized string without allocating.
// Used for memory estimation (EstimatedBytes). For wire-protocol positions use sanitizeForTemplateRuneLen.
func sanitizeForTemplateLen(s string) int {
	return sanitizeForTemplateInto(nil, s)
}

// sanitizeForTemplateRuneLen returns the Unicode codepoint count of the sanitized string.
// Used in GetWildcardCharPositions so positions match Java String.length() (UTF-16 code units).
// For BMP characters (U+0000–U+FFFF, the vast majority of log content) codepoint count
// equals UTF-16 code unit count. Supplementary-plane characters (emoji etc.) are uncommon
// in log templates and would still be off by the surrogate count — an acceptable tradeoff.
func sanitizeForTemplateRuneLen(s string) int {
	count := 0
	for _, r := range s {
		if (r >= ' ' && r != 0x7F) || r == '\t' || r == '\n' || r == '\r' {
			if r != utf8.RuneError && r < 0xFFFD {
				count++
			}
		}
	}
	return count
}

// sanitizeForTemplateInto appends the sanitized string into builder when non-nil.
// Uses an ASCII fast path: bytes < 0x80 are checked directly without rune decoding.
// Preserved: printable ASCII (0x20–0x7E), horizontal tab (0x09), newline (0x0A), carriage return (0x0D).
// Stripped:  other control characters (0x00–0x08, 0x0B–0x0C, 0x0E–0x1F), DEL (0x7F).
func sanitizeForTemplateInto(builder *strings.Builder, s string) int {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < utf8.RuneSelf {
			if (b >= ' ' && b != 0x7F) || b == '\t' || b == '\n' || b == '\r' {
				continue
			}
			// ASCII control character found — flush clean prefix then filter the rest
			if builder != nil {
				builder.WriteString(s[:i])
			}
			written := i
			i++
			for i < len(s) {
				b = s[i]
				if b < utf8.RuneSelf {
					if (b >= ' ' && b != 0x7F) || b == '\t' || b == '\n' || b == '\r' {
						if builder != nil {
							builder.WriteByte(b)
						}
						written++
					}
					i++
					continue
				}
				r, size := utf8.DecodeRuneInString(s[i:])
				if r >= ' ' && r != 0x7F && r != utf8.RuneError && r < 0xFFFD {
					if builder != nil {
						builder.WriteString(s[i : i+size])
					}
					written += size
				}
				i += size
			}
			return written
		}
		// Non-ASCII byte — decode rune
		r, size := utf8.DecodeRuneInString(s[i:])
		if r < ' ' || r == 0x7F || r == utf8.RuneError || r >= 0xFFFD {
			if builder != nil {
				builder.WriteString(s[:i])
			}
			written := i
			i += size
			for i < len(s) {
				b = s[i]
				if b < utf8.RuneSelf {
					if (b >= ' ' && b != 0x7F) || b == '\t' || b == '\n' || b == '\r' {
						if builder != nil {
							builder.WriteByte(b)
						}
						written++
					}
					i++
					continue
				}
				r, size = utf8.DecodeRuneInString(s[i:])
				if r >= ' ' && r != 0x7F && r != utf8.RuneError && r < 0xFFFD {
					if builder != nil {
						builder.WriteString(s[i : i+size])
					}
					written += size
				}
				i += size
			}
			return written
		}
		i += size - 1 // outer loop increments i
	}

	if builder != nil {
		builder.WriteString(s)
	}
	return len(s)
}
