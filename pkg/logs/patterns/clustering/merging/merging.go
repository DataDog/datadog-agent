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

// CanMergeTokenLists checks if two TokenLists can merge — that is, whether all token positions
// are either identical or mergeable according to their comparison results and protection rules.
func CanMergeTokenLists(tl1, tl2 *token.TokenList, firstWordProtection bool) bool {
	n := len(tl1.Tokens)
	if n != len(tl2.Tokens) {
		return false
	}

	// Fast pre-check: if first tokens conflict, skip the full loop.
	// This is the most common rejection path — different log types rarely share position-0 tokens.
	if n > 0 && tl1.Tokens[0].Type != tl2.Tokens[0].Type {
		return false
	}

	// Precompute the position of the first word token in tl1 (only if protection is enabled).
	firstWordPos1 := -1
	if firstWordProtection {
		for i, tok := range tl1.Tokens {
			if tok.Type == token.TokenWord {
				firstWordPos1 = i
				break
			}
		}
	}

	tokens1 := tl1.Tokens[:n]
	tokens2 := tl2.Tokens[:n]

	for i := range n {
		tok1 := &tokens1[i]
		tok2 := &tokens2[i]

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
		if result == token.Wildcard && tok1.Type == token.TokenWord && i == firstWordPos1 {
			return false
		}
	}

	return true
}

// TryMergeTokenLists combines CanMergeTokenLists and MergeTokenLists into a single pass.
// Returns the merged TokenList if compatible, or nil if incompatible.
// This avoids walking the token arrays twice (once for CanMerge, once for Merge).
// The first-word protection is symmetric: it protects the first word token position
// from either list, making the function order-independent.
func TryMergeTokenLists(tl1, tl2 *token.TokenList, firstWordProtection bool) *token.TokenList {
	n := len(tl1.Tokens)
	if n != len(tl2.Tokens) {
		return nil
	}

	// Precompute the position of the first word token in each list (only if protection is enabled).
	firstWordPos1 := -1
	firstWordPos2 := -1
	if firstWordProtection {
		for i, tok := range tl1.Tokens {
			if tok.Type == token.TokenWord {
				firstWordPos1 = i
				break
			}
		}
		for i, tok := range tl2.Tokens {
			if tok.Type == token.TokenWord {
				firstWordPos2 = i
				break
			}
		}
	}

	tokens1 := tl1.Tokens[:n]
	tokens2 := tl2.Tokens[:n]

	var tokens []token.Token

	for i := range n {
		tok1 := &tokens1[i]
		tok2 := &tokens2[i]

		result := tok1.Compare(tok2)

		switch result {
		case token.Conflict:
			return nil

		case token.Identical:
			if tokens != nil {
				tokens = append(tokens, *tok1)
			}

		case token.Wildcard:
			// tok1.Type == tok2.Type is guaranteed here (Compare returns Wildcard only when types match)
			if tok1.Type == token.TokenWord && (i == firstWordPos1 || i == firstWordPos2) {
				return nil
			}
			if tokens == nil {
				// First wildcard: allocate and copy all identical tokens seen so far
				tokens = make([]token.Token, i, n)
				copy(tokens, tokens1[:i])
			}
			tokens = append(tokens, token.Token{Type: tok1.Type, Value: tok1.Value, Wildcard: token.IsWildcard})
		}
	}

	if tokens == nil {
		return tl1 // all tokens identical — reuse existing list, zero allocation
	}
	return token.NewTokenListWithTokens(tokens)
}

// ForceWiden merges tokenList into template by wildcarding all non-identical positions.
// No first-word protection. Returns nil if lengths differ; returns template (pointer-same)
// if all positions are already identical (zero-alloc path).
func ForceWiden(template, tokenList *token.TokenList) *token.TokenList {
	if len(template.Tokens) != len(tokenList.Tokens) {
		return nil
	}
	changed := false
	for i := range template.Tokens {
		if template.Tokens[i].Compare(&tokenList.Tokens[i]) != token.Identical {
			changed = true
			break
		}
	}
	if !changed {
		return template
	}
	tokens := make([]token.Token, len(template.Tokens))
	for i := range template.Tokens {
		if template.Tokens[i].Compare(&tokenList.Tokens[i]) == token.Identical {
			tokens[i] = template.Tokens[i]
		} else {
			tokens[i] = token.Token{Type: template.Tokens[i].Type, Wildcard: token.IsWildcard}
		}
	}
	return token.NewTokenListWithTokens(tokens)
}

// MergeTokenLists performs the actual merge of two TokenLists, creating a new TokenList
// with wildcards at positions where tokens differ but are mergeable.
// Returns nil if the TokenLists cannot be merged.
func MergeTokenLists(tl1, tl2 *token.TokenList, firstWordProtection bool) *token.TokenList {
	n := len(tl1.Tokens)
	if n != len(tl2.Tokens) {
		return nil
	}

	// Precompute the position of the first word token in tl1 (only if protection is enabled).
	firstWordPos1 := -1
	if firstWordProtection {
		for i, tok := range tl1.Tokens {
			if tok.Type == token.TokenWord {
				firstWordPos1 = i
				break
			}
		}
	}

	tokens := make([]token.Token, 0, n)

	for i := range n {
		tok1 := &tl1.Tokens[i]
		tok2 := &tl2.Tokens[i]

		result := tok1.Compare(tok2)

		switch result {
		case token.Conflict:
			return nil

		case token.Identical:
			tokens = append(tokens, *tok1)

		case token.Wildcard:
			if tok1.Type == token.TokenWord && i == firstWordPos1 {
				return nil
			}
			tokens = append(tokens, token.Token{Type: tok1.Type, Value: tok1.Value, Wildcard: token.IsWildcard})
		}
	}

	return token.NewTokenListWithTokens(tokens)
}
