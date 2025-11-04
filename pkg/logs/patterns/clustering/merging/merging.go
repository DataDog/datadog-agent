// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package merging provides intelligent mergeability logic for pattern generation.
// It determines which TokenLists can be merged into unified patterns with wildcards,
// and enforces protection rules to maintain semantic quality.
package merging

import (
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// shouldProtectPosition determines if a the token is the first word token and should be wildcarded.
func shouldProtectPosition(position int, tokenType token.TokenType) bool {
	if position == 0 && tokenType == token.TokenWord {
		return true
	}

	return false
}

// CanMergeTokenLists checks if two TokenLists can be merged into a unified pattern.
// Returns true only if all token positions are either identical or mergeable according
// to their comparison results and protection rules.
func CanMergeTokenLists(tl1, tl2 *token.TokenList) bool {
	if tl1.Length() != tl2.Length() {
		return false
	}

	for i := 0; i < tl1.Length(); i++ {
		tok1 := &tl1.Tokens[i]
		tok2 := &tl2.Tokens[i]

		result := tok1.Compare(tok2)

		// If tokens conflict, reject
		if result == token.Conflict {
			return false
		}

		// If tokens are identical, continue
		if result == token.Identical {
			continue
		}

		// For wildcard result, check protection rules
		if result == token.Wildcard && shouldProtectPosition(i, tok1.Type) {
			return false
		}
	}

	return true
}

// MergeTokenLists performs the actual merge of two TokenLists, creating a new TokenList
// with wildcards at positions where tokens differ but are mergeable.
// Returns nil if the TokenLists cannot be merged.
func MergeTokenLists(tl1, tl2 *token.TokenList) *token.TokenList {
	if tl1.Length() != tl2.Length() {
		return nil
	}

	merged := token.NewTokenList()

	for i := 0; i < tl1.Length(); i++ {
		tok1 := &tl1.Tokens[i]
		tok2 := &tl2.Tokens[i]

		result := tok1.Compare(tok2)

		switch result {
		case token.Conflict:
			return nil // Abort entire merge

		case token.Identical:
			merged.Add(*tok1) // Keep same

		case token.Wildcard:
			// Check protection rules before wildcarding
			if shouldProtectPosition(i, tok1.Type) {
				return nil
			}
			// Create wildcard, preserving the first token's value as representative
			merged.AddToken(tok1.Type, tok1.Value, token.IsWildcard)
		}
	}

	return merged
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
