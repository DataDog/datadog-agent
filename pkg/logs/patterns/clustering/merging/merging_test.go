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
			result := ShouldProtectPosition(tt.position, tt.tokenType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanMergeTokenLists_IdenticalLists(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "world"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "world"),
	})

	assert.True(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_PossiblyWildcardTokens(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewPossiblyWildcardToken(token.TokenWord, "user123"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewPossiblyWildcardToken(token.TokenWord, "admin456"),
	})

	assert.True(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_GenericWords(t *testing.T) {
	// Generic words without possiblyWildcard flag should not merge
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "bob"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "likes"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "cat"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "likes"),
	})

	assert.False(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_DifferentLengths(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "world"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello"),
	})

	assert.False(t, CanMergeTokenLists(tl1, tl2))
}

func TestCanMergeTokenLists_FirstWordProtection(t *testing.T) {
	// First word protection should prevent merge even with possiblyWildcard
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewPossiblyWildcardToken(token.TokenWord, "user123"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "logged"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewPossiblyWildcardToken(token.TokenWord, "admin456"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "logged"),
	})

	assert.False(t, CanMergeTokenLists(tl1, tl2), "First word should be protected from wildcarding")
}

func TestMergeTokenLists_CreateWildcard(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewPossiblyWildcardToken(token.TokenWord, "user123"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewPossiblyWildcardToken(token.TokenWord, "admin456"),
	})

	merged := MergeTokenLists(tl1, tl2)
	assert.NotNil(t, merged)
	assert.Equal(t, 3, merged.Length())
	assert.Equal(t, "logged", merged.Tokens[0].Value)
	assert.False(t, merged.Tokens[0].IsWildcard)
	assert.Equal(t, " ", merged.Tokens[1].Value)
	assert.Equal(t, "*", merged.Tokens[2].Value)
	assert.True(t, merged.Tokens[2].IsWildcard)
}

func TestMergeTokenLists_UnmergeableReturnsNil(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "bob"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "likes"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "cat"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "likes"),
	})

	merged := MergeTokenLists(tl1, tl2)
	assert.Nil(t, merged, "Unmergeable TokenLists should return nil")
}

func TestMergeTokenLists_DateMerging(t *testing.T) {
	dateInfo1 := &token.DateComponents{
		Year:   "2024",
		Month:  "01",
		Day:    "15",
		Hour:   "10",
		Minute: "30",
		Second: "45",
		Format: "RFC3339",
	}

	dateInfo2 := &token.DateComponents{
		Year:   "2024",
		Month:  "01",
		Day:    "15",
		Hour:   "14",
		Minute: "22",
		Second: "30",
		Format: "RFC3339",
	}

	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Log"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewDateToken("2024-01-15T10:30:45Z", dateInfo1),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Log"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewDateToken("2024-01-15T14:22:30Z", dateInfo2),
	})

	merged := MergeTokenLists(tl1, tl2)
	assert.NotNil(t, merged)
	assert.Equal(t, 3, merged.Length())

	// Date token should have partial wildcard for time components
	dateToken := merged.Tokens[2]
	assert.True(t, dateToken.IsWildcard)
	assert.Equal(t, token.TokenDate, dateToken.Type)
	// Should preserve date, wildcard time: 2024-01-15T*:*:*
	assert.Contains(t, dateToken.Value, "2024-01-15")
}

func TestFindMergeableGroups_SingleGroup(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewPossiblyWildcardToken(token.TokenWord, "user123"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewPossiblyWildcardToken(token.TokenWord, "admin456"),
	})

	tl3 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewPossiblyWildcardToken(token.TokenWord, "guest789"),
	})

	groups := FindMergeableGroups([]*token.TokenList{tl1, tl2, tl3})
	assert.Equal(t, 1, len(groups), "All mergeable TokenLists should be in one group")
	assert.Equal(t, 3, len(groups[0]), "Group should contain all three TokenLists")
}

func TestFindMergeableGroups_MultipleGroups(t *testing.T) {
	// Group 1: mergeable user logs
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewPossiblyWildcardToken(token.TokenWord, "user123"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewPossiblyWildcardToken(token.TokenWord, "admin456"),
	})

	// Group 2: unmergeable generic words
	tl3 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewToken(token.TokenWord, "cat"),
	})

	tl4 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "logged"),
		token.NewToken(token.TokenWord, "dog"),
	})

	groups := FindMergeableGroups([]*token.TokenList{tl1, tl2, tl3, tl4})
	assert.Equal(t, 3, len(groups), "Should have 3 groups: user group + 2 separate generic word entries")

	// Find the largest group (should be the user group with 2 members)
	maxSize := 0
	for _, group := range groups {
		if len(group) > maxSize {
			maxSize = len(group)
		}
	}
	assert.Equal(t, 2, maxSize, "Largest group should have 2 TokenLists")
}

func TestFindMergeableGroups_EmptyInput(t *testing.T) {
	groups := FindMergeableGroups([]*token.TokenList{})
	assert.Nil(t, groups)
}

func TestFindMergeableGroups_SingleTokenList(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "hello"),
	})

	groups := FindMergeableGroups([]*token.TokenList{tl1})
	assert.Equal(t, 1, len(groups))
	assert.Equal(t, 1, len(groups[0]))
}

func TestMergeTokenLists_ProtectionRulesEnforced(t *testing.T) {
	// Try to merge when first token is a word but differs
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewPossiblyWildcardToken(token.TokenWord, "Login"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "successful"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewPossiblyWildcardToken(token.TokenWord, "Logout"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewToken(token.TokenWord, "successful"),
	})

	// Should fail because first word is protected
	merged := MergeTokenLists(tl1, tl2)
	assert.Nil(t, merged, "Should not merge when first word differs (protected)")
}

func TestMergeTokenLists_ProgressiveMerging(t *testing.T) {
	// Test merging multiple TokenLists progressively
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewPossiblyWildcardToken(token.TokenNumeric, "123"),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewPossiblyWildcardToken(token.TokenNumeric, "456"),
	})

	tl3 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request"),
		token.NewToken(token.TokenWhitespace, " "),
		token.NewPossiblyWildcardToken(token.TokenNumeric, "789"),
	})

	// Merge first two
	merged12 := MergeTokenLists(tl1, tl2)
	assert.NotNil(t, merged12)
	assert.True(t, merged12.Tokens[2].IsWildcard)

	// Merge result with third
	merged123 := MergeTokenLists(merged12, tl3)
	assert.NotNil(t, merged123)
	assert.Equal(t, 3, merged123.Length())
	assert.Equal(t, "Request", merged123.Tokens[0].Value)
	assert.Equal(t, "*", merged123.Tokens[2].Value)
	assert.True(t, merged123.Tokens[2].IsWildcard)
}
