// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merging

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestShouldProtectPosition(t *testing.T) {
	tests := []struct {
		name      string
		position  int
		tokenType token.TokenType
		expected  bool
	}{
		{
			name:      "First word should be protected",
			position:  0,
			tokenType: token.TokenWord,
			expected:  true,
		},
		{
			name:      "First numeric should not be protected",
			position:  0,
			tokenType: token.TokenNumeric,
			expected:  false,
		},
		{
			name:      "Second word should not be protected",
			position:  1,
			tokenType: token.TokenWord,
			expected:  false,
		},
		{
			name:      "First whitespace should not be protected",
			position:  0,
			tokenType: token.TokenWhitespace,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldProtectPosition(tt.position, tt.tokenType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanMergeTokenLists_IdenticalLists(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "world", token.NotWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "world", token.NotWildcard),
	})

	assert.True(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_PossiblyWildcardTokens(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "user123", token.PotentialWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "admin456", token.PotentialWildcard),
	})

	assert.True(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_GenericWords(t *testing.T) {
	// Generic words without possiblyWildcard flag should not merge
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "bob", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "likes", token.NotWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "cat", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "likes", token.NotWildcard),
	})

	assert.False(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_DifferentLengths(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "world", token.NotWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello", token.NotWildcard),
	})

	assert.False(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_FirstWordProtection(t *testing.T) {
	// First word protection should prevent merge even with possiblyWildcard
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "user123", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "logged", token.NotWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "admin456", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "logged", token.NotWildcard),
	})

	assert.False(t, CanMergeTokenLists(tl1, tl2), "First word should be protected from wildcarding")
}

func TestMergeTokenLists_CreateWildcard(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "user123", token.PotentialWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "admin456", token.PotentialWildcard),
	})

	merged := MergeTokenLists(tl1, tl2)
	assert.NotNil(t, merged)
	assert.Equal(t, 3, merged.Length())
	assert.Equal(t, "logged", merged.Tokens[0].Value)
	assert.Equal(t, token.NotWildcard, merged.Tokens[0].Wildcard)
	assert.Equal(t, " ", merged.Tokens[1].Value)
	// Wildcard token has empty value - the Wildcard field tracks status
	assert.Equal(t, token.IsWildcard, merged.Tokens[2].Wildcard)
	assert.Equal(t, token.TokenWord, merged.Tokens[2].Type)
}

func TestMergeTokenLists_UnmergeableReturnsNil(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "bob", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "likes", token.NotWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "cat", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "likes", token.NotWildcard),
	})

	merged := MergeTokenLists(tl1, tl2)
	assert.Nil(t, merged, "Unmergeable TokenLists should return nil")
}

func TestMergeTokenLists_ProtectionRulesEnforced(t *testing.T) {
	// Try to merge when first token is a word but differs
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Login", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "successful", token.NotWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Logout", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "successful", token.NotWildcard),
	})

	// Should fail because first word is protected
	merged := MergeTokenLists(tl1, tl2)
	assert.Nil(t, merged, "Should not merge when first word differs (protected)")
}

func TestMergeTokenLists_ProgressiveMerging(t *testing.T) {
	// Test merging multiple TokenLists progressively
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "123", token.PotentialWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "456", token.PotentialWildcard),
	})

	tl3 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "789", token.PotentialWildcard),
	})

	// Merge first two
	merged12 := MergeTokenLists(tl1, tl2)
	assert.NotNil(t, merged12)
	assert.Equal(t, token.IsWildcard, merged12.Tokens[2].Wildcard)

	// Merge result with third
	merged123 := MergeTokenLists(merged12, tl3)
	assert.NotNil(t, merged123)
	assert.Equal(t, 3, merged123.Length())
	assert.Equal(t, "Request", merged123.Tokens[0].Value)
	// Wildcard token has empty value - the Wildcard field tracks status
	assert.Equal(t, token.IsWildcard, merged123.Tokens[2].Wildcard)
	assert.Equal(t, token.TokenNumeric, merged123.Tokens[2].Type)
}
