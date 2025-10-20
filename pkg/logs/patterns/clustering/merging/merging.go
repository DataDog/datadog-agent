// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package merging provides intelligent mergeability logic for pattern generation.
// It determines which TokenLists can be merged into unified patterns with wildcards,
// and enforces protection rules to maintain semantic quality.
package merging

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// ShouldProtectPosition determines if a position should never be wildcarded.
// Protection rules ensure pattern quality by preventing wildcarding of
// semantically important positions.
func ShouldProtectPosition(position int, tokenType token.TokenType) bool {
	// Rule 1: Never wildcard the first word token
	// The first word typically indicates the action/command and is semantically critical
	// e.g., "Login successful" vs "Error occurred" should not merge to "* *"
	if position == 0 && tokenType == token.TokenWord {
		return true
	}

	// Future: Add more protection rules
	// - Never wildcard HTTP methods?
	// - Never wildcard severity levels?
	// - Protect first N tokens?

	return false
}

// CanMergeTokenLists checks if two TokenLists can be merged into a unified pattern.
// Returns true only if all token positions are either identical or mergeable according
// to their mergeability levels and protection rules.
func CanMergeTokenLists(tl1, tl2 *token.TokenList) bool {
	if tl1.Length() != tl2.Length() {
		return false
	}

	for i := 0; i < tl1.Length(); i++ {
		tok1 := &tl1.Tokens[i]
		tok2 := &tl2.Tokens[i]

		level := tok1.GetMergeabilityLevel(tok2)

		// If tokens match exactly, continue
		if level == token.FitsAsItIs {
			continue
		}

		// If tokens can't merge at all, reject
		if !level.IsMergeable() {
			return false
		}

		// Check protection rules - if position is protected and tokens differ, reject
		if ShouldProtectPosition(i, tok1.Type) {
			return false
		}
	}

	return true
}

// MergeTokenLists performs the actual merge of two TokenLists, creating a new TokenList
// with wildcards at positions where tokens differ but are mergeable.
// Returns nil if the TokenLists cannot be merged.
func MergeTokenLists(tl1, tl2 *token.TokenList) *token.TokenList {
	if !CanMergeTokenLists(tl1, tl2) {
		return nil
	}

	merged := token.NewTokenList()

	for i := 0; i < tl1.Length(); i++ {
		tok1 := &tl1.Tokens[i]
		tok2 := &tl2.Tokens[i]

		level := tok1.GetMergeabilityLevel(tok2)

		if level == token.FitsAsItIs {
			// Tokens are identical, keep as-is
			merged.Add(*tok1)
			continue
		}

		// Handle different merge types
		switch level {
		case token.MergeableWithWiderRange:
			// Special handling for structured tokens (e.g., dates with partial wildcards)
			if tok1.Type == token.TokenDate && tok1.DateInfo != nil && tok2.DateInfo != nil {
				merged.Add(createPartialDateWildcard(tok1.DateInfo, tok2.DateInfo))
			} else {
				// Fallback to full wildcard
				merged.AddWildcardToken(tok1.Type)
			}
		case token.MergeableAsWildcard:
			// Create a full wildcard for this position
			merged.AddWildcardToken(tok1.Type)
		default:
			// Shouldn't reach here if CanMergeTokenLists passed, but be defensive
			merged.Add(*tok1)
		}
	}

	return merged
}

// createPartialDateWildcard creates a date token with wildcards in differing components.
// This allows for more precise patterns like "2024-01-* 10:30:45" instead of just "*".
func createPartialDateWildcard(d1, d2 *token.DateComponents) token.Token {
	// Create a pattern where differing components become wildcards
	var pattern strings.Builder

	switch d1.Format {
	case "RFC3339", "ISO8601":
		// Format: YYYY-MM-DDTHH:MM:SS
		if d1.Year == d2.Year {
			pattern.WriteString(d1.Year)
		} else {
			pattern.WriteString("*")
		}
		pattern.WriteString("-")

		if d1.Month == d2.Month {
			pattern.WriteString(d1.Month)
		} else {
			pattern.WriteString("*")
		}
		pattern.WriteString("-")

		if d1.Day == d2.Day {
			pattern.WriteString(d1.Day)
		} else {
			pattern.WriteString("*")
		}
		pattern.WriteString("T")

		if d1.Hour == d2.Hour {
			pattern.WriteString(d1.Hour)
		} else {
			pattern.WriteString("*")
		}
		pattern.WriteString(":")

		if d1.Minute == d2.Minute {
			pattern.WriteString(d1.Minute)
		} else {
			pattern.WriteString("*")
		}
		pattern.WriteString(":")

		if d1.Second == d2.Second {
			pattern.WriteString(d1.Second)
		} else {
			pattern.WriteString("*")
		}

	default:
		// For other formats, just use a full wildcard
		return token.NewWildcardToken(token.TokenDate)
	}

	return token.Token{
		Type:       token.TokenDate,
		Value:      pattern.String(),
		IsWildcard: true,
		DateInfo:   d1, // Keep the first date's structure for reference
	}
}

// FindMergeableGroups analyzes a list of TokenLists and groups them by mergeability.
// This is used to detect heterogeneous clusters that should be split into multiple patterns.
// Returns a list of groups where each group contains mutually mergeable TokenLists.
func FindMergeableGroups(tokenLists []*token.TokenList) [][]*token.TokenList {
	if len(tokenLists) == 0 {
		return nil
	}

	if len(tokenLists) == 1 {
		return [][]*token.TokenList{tokenLists}
	}

	var groups [][]*token.TokenList
	processed := make(map[int]bool)

	for i := 0; i < len(tokenLists); i++ {
		if processed[i] {
			continue
		}

		// Start a new group with this TokenList
		group := []*token.TokenList{tokenLists[i]}
		processed[i] = true

		// Find all TokenLists that can merge with this one
		for j := i + 1; j < len(tokenLists); j++ {
			if processed[j] {
				continue
			}

			// Check if this TokenList can merge with all members of the current group
			canMergeWithGroup := true
			for _, groupMember := range group {
				if !CanMergeTokenLists(tokenLists[j], groupMember) {
					canMergeWithGroup = false
					break
				}
			}

			if canMergeWithGroup {
				group = append(group, tokenLists[j])
				processed[j] = true
			}
		}

		groups = append(groups, group)
	}

	return groups
}
