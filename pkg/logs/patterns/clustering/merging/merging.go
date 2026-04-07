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

// shouldProtectPosition determines if the token at this position is the first word token.
// The first word token is protected from wildcarding to preserve semantic meaning,
// regardless of what position it appears at (e.g., after timestamps/dates).
func shouldProtectPosition(position int, tokenType token.TokenType, tl *token.TokenList) bool {
	// Only word tokens can be protected
	if tokenType != token.TokenWord {
		return false
	}

	// Check if any word token appears before this position
	for i := 0; i < position; i++ {
		if tl.Tokens[i].Type == token.TokenWord {
			return false // Not the first word token
		}
	}

	// This is the first word token
	return true
}

// CanMergeTokenLists checks if incoming log (tl2) can merge with existing pattern's sample (tl1).
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

		// For wildcard result, check first word protection rule
		if result == token.Wildcard && shouldProtectPosition(i, tok1.Type, tl1) {
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
			if shouldProtectPosition(i, tok1.Type, tl1) {
				return nil
			}
			// Create wildcard, preserving the first token's value as representative
			merged.AddToken(tok1.Type, tok1.Value, token.IsWildcard)
		}
	}

	return merged
}
