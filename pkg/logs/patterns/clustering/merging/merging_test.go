// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestFirstWordProtection_ViaCanMerge(t *testing.T) {
	// Verify first-word protection semantics through the public CanMergeTokenLists API,
	// replacing direct tests of the now-deleted shouldProtectPosition helper.

	// First word at position 0 should be protected (merge blocked)
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "ERROR", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "failed", token.NotWildcard),
	})
	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "WARN", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "failed", token.NotWildcard),
	})
	assert.False(t, CanMergeTokenLists(tl1, tl2, true), "First word at position 0 should be protected")

	// First numeric at position 0 is NOT protected (only words are)
	tl3 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2025", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
	})
	tl4 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2026", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
	})
	assert.True(t, CanMergeTokenLists(tl3, tl4, true), "Numeric at position 0 should not be protected")

	// Second word (not first) is NOT protected
	tl5 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "failed", token.PotentialWildcard),
	})
	tl6 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "success", token.PotentialWildcard),
	})
	assert.True(t, CanMergeTokenLists(tl5, tl6, true), "Second word should not be protected")

	// First word after timestamp should be protected
	tl7 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2025-11-16", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "07:03:09", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "ERROR", token.PotentialWildcard),
	})
	tl8 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2025-11-17", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "08:00:00", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "WARN", token.PotentialWildcard),
	})
	assert.False(t, CanMergeTokenLists(tl7, tl8, true), "First word after timestamp should be protected")
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

	assert.True(t, CanMergeTokenLists(tl1, tl2, true))
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

	assert.True(t, CanMergeTokenLists(tl1, tl2, true))
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

	assert.False(t, CanMergeTokenLists(tl1, tl2, true))
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

	assert.False(t, CanMergeTokenLists(tl1, tl2, true))
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

	assert.False(t, CanMergeTokenLists(tl1, tl2, true), "First word should be protected from wildcarding")
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

	merged := MergeTokenLists(tl1, tl2, true)
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

	merged := MergeTokenLists(tl1, tl2, true)
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
	merged := MergeTokenLists(tl1, tl2, true)
	assert.Nil(t, merged, "Should not merge when first word differs (protected)")
}

func TestCanMergeTokenLists_TimestampPrefixedLogs(t *testing.T) {
	// Test that first WORD (not severity level) after timestamp is protected
	// Severity levels CAN wildcard, but the first actual word is protected
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2025-11-16", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "07:03:09", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenSeverityLevel, "ERROR", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "Failed", token.NotWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2025-11-16", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "07:03:11", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenSeverityLevel, "WARN", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "Memory", token.NotWildcard),
	})

	// Should NOT merge because first word (Failed vs Memory) differs and is protected
	// Note: Severity levels (ERROR vs WARN) CAN wildcard - they're not the "first word"
	assert.False(t, CanMergeTokenLists(tl1, tl2, true), "First word token (after severity) should be protected")
}

func TestMergeTokenLists_TimestampPrefixedLogsSameFirstWord(t *testing.T) {
	// Test that logs with same first word can merge, even with different timestamps and severity levels
	// Pattern: * * * Failed *
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2025-11-15", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "07:03:09", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenSeverityLevel, "ERROR", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "Failed", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "user123", token.PotentialWildcard),
	})

	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenNumeric, "2025-11-16", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "07:03:11", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenSeverityLevel, "WARN", token.PotentialWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "Failed", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenWord, "admin456", token.PotentialWildcard),
	})

	// Should merge - timestamps wildcard, severity wildcard, "Failed" is protected but identical, last word wildcards
	merged := MergeTokenLists(tl1, tl2, true)
	assert.NotNil(t, merged, "Should merge when first word matches")
	assert.Equal(t, token.IsWildcard, merged.Tokens[0].Wildcard, "Date should be wildcarded")
	assert.Equal(t, token.IsWildcard, merged.Tokens[2].Wildcard, "Time should be wildcarded")
	assert.Equal(t, token.IsWildcard, merged.Tokens[4].Wildcard, "Severity level should be wildcarded")
	assert.Equal(t, "Failed", merged.Tokens[6].Value, "Failed (first word) should be preserved")
	assert.Equal(t, token.NotWildcard, merged.Tokens[6].Wildcard, "Failed should not be wildcarded (protected)")
	assert.Equal(t, token.IsWildcard, merged.Tokens[8].Wildcard, "Last word should be wildcarded")
}

func TestTryMergeTokenLists_Equivalence(t *testing.T) {
	type fixture struct {
		name string
		tl1  *token.TokenList
		tl2  *token.TokenList
	}

	// --- fixtures where CanMergeTokenLists returns true ---
	canMergeFixtures := []fixture{
		{
			name: "identical lists",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "hello", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "world", token.NotWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "hello", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "world", token.NotWildcard),
			}),
		},
		{
			name: "wildcard-eligible words (non-first position)",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "logged", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "user123", token.PotentialWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "logged", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "admin456", token.PotentialWildcard),
			}),
		},
		{
			name: "numeric at position 0 (not protected)",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenNumeric, "2025", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenNumeric, "2026", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
			}),
		},
		{
			name: "second word not protected",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "failed", token.PotentialWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "ERROR", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "success", token.PotentialWildcard),
			}),
		},
		{
			name: "timestamp-prefixed logs same first word",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenNumeric, "2025-11-15", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenNumeric, "07:03:09", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenSeverityLevel, "ERROR", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "Failed", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "user123", token.PotentialWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenNumeric, "2025-11-16", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenNumeric, "07:03:11", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenSeverityLevel, "WARN", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "Failed", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "admin456", token.PotentialWildcard),
			}),
		},
	}

	// --- fixtures where CanMergeTokenLists returns false in BOTH directions ---
	cannotMergeFixtures := []fixture{
		{
			name: "generic words no wildcard flag",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "bob", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "likes", token.NotWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "cat", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "likes", token.NotWildcard),
			}),
		},
		{
			name: "different lengths",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "hello", token.NotWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "world", token.NotWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "hello", token.NotWildcard),
			}),
		},
		{
			name: "first word at position 0 protected",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "ERROR", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "failed", token.NotWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "WARN", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "failed", token.NotWildcard),
			}),
		},
		{
			name: "first word protected (first-word protection on position 0)",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "user123", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "logged", token.NotWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenWord, "admin456", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "logged", token.NotWildcard),
			}),
		},
		{
			name: "timestamp-prefixed logs different first word after severity",
			tl1: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenNumeric, "2025-11-16", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenNumeric, "07:03:09", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "ERROR", token.PotentialWildcard),
			}),
			tl2: token.NewTokenListWithTokens([]token.Token{
				token.NewToken(token.TokenNumeric, "2025-11-17", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenNumeric, "08:00:00", token.PotentialWildcard),
				token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
				token.NewToken(token.TokenWord, "WARN", token.PotentialWildcard),
			}),
		},
	}

	t.Run("CanMerge=true implies TryMerge returns non-nil and matches MergeTokenLists", func(t *testing.T) {
		for _, f := range canMergeFixtures {
			t.Run(f.name, func(t *testing.T) {
				assert.True(t, CanMergeTokenLists(f.tl1, f.tl2, true), "prerequisite: CanMerge should be true")

				tryResult := TryMergeTokenLists(f.tl1, f.tl2, true)
				assert.NotNil(t, tryResult, "TryMerge should return non-nil when CanMerge is true")

				mergeResult := MergeTokenLists(f.tl1, f.tl2, true)
				assert.NotNil(t, mergeResult, "MergeTokenLists should return non-nil when CanMerge is true")

				// Both must produce the same token sequence
				assert.Equal(t, len(mergeResult.Tokens), len(tryResult.Tokens), "token count must match")
				for i := range mergeResult.Tokens {
					assert.Equal(t, mergeResult.Tokens[i].Type, tryResult.Tokens[i].Type,
						"token[%d].Type mismatch", i)
					assert.Equal(t, mergeResult.Tokens[i].Wildcard, tryResult.Tokens[i].Wildcard,
						"token[%d].Wildcard mismatch", i)
					// Only check Value for non-wildcard tokens (wildcard tokens have empty Value in TryMerge, matching MergeTokenLists)
					if mergeResult.Tokens[i].Wildcard != token.IsWildcard {
						assert.Equal(t, mergeResult.Tokens[i].Value, tryResult.Tokens[i].Value,
							"token[%d].Value mismatch", i)
					}
				}
			})
		}
	})

	t.Run("CanMerge=false in both directions implies TryMerge returns nil", func(t *testing.T) {
		for _, f := range cannotMergeFixtures {
			t.Run(f.name, func(t *testing.T) {
				// Confirm CanMerge is false in at least the forward direction for these fixtures
				// (for different-length fixtures CanMerge is false in both directions trivially)
				canFwd := CanMergeTokenLists(f.tl1, f.tl2, true)
				canRev := CanMergeTokenLists(f.tl2, f.tl1, true)
				assert.False(t, canFwd && canRev, "prerequisite: CanMerge should be false in at least one direction")

				// TryMerge is symmetric; if CanMerge is false in both directions it must return nil
				if !canFwd && !canRev {
					tryFwd := TryMergeTokenLists(f.tl1, f.tl2, true)
					tryRev := TryMergeTokenLists(f.tl2, f.tl1, true)
					assert.Nil(t, tryFwd, "TryMerge(tl1,tl2) should return nil when both CanMerge directions are false")
					assert.Nil(t, tryRev, "TryMerge(tl2,tl1) should return nil when both CanMerge directions are false")
				}
			})
		}
	})
}

func TestTryMergeTokenLists_IdentityOnIdentical(t *testing.T) {
	tl1 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "123", token.PotentialWildcard),
	})

	// tl2 is a separate allocation with identical token values
	tl2 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "123", token.PotentialWildcard),
	})

	result := TryMergeTokenLists(tl1, tl2, true)

	// When all tokens are identical TryMerge must return tl1 itself (identity / zero alloc path)
	assert.True(t, result == tl1, "TryMerge on identical lists must return the same pointer as tl1")
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.Length(), "token count must be preserved")
	assert.Equal(t, "Request", result.Tokens[0].Value)
	assert.Equal(t, token.NotWildcard, result.Tokens[0].Wildcard)
	assert.Equal(t, "123", result.Tokens[2].Value)

	// Second merge: tl3 also identical → template must still be correct
	tl3 := token.NewTokenListWithTokens([]token.Token{
		token.NewToken(token.TokenWord, "Request", token.NotWildcard),
		token.NewToken(token.TokenWhitespace, " ", token.NotWildcard),
		token.NewToken(token.TokenNumeric, "123", token.PotentialWildcard),
	})

	result2 := TryMergeTokenLists(result, tl3, true)
	assert.True(t, result2 == result, "second TryMerge on identical lists must still return the same pointer")
	assert.Equal(t, 3, result2.Length())
	assert.Equal(t, "Request", result2.Tokens[0].Value)
	assert.Equal(t, token.NotWildcard, result2.Tokens[0].Wildcard)
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
	merged12 := MergeTokenLists(tl1, tl2, true)
	assert.NotNil(t, merged12)
	assert.Equal(t, token.IsWildcard, merged12.Tokens[2].Wildcard)

	// Merge result with third
	merged123 := MergeTokenLists(merged12, tl3, true)
	assert.NotNil(t, merged123)
	assert.Equal(t, 3, merged123.Length())
	assert.Equal(t, "Request", merged123.Tokens[0].Value)
	// Wildcard token has empty value - the Wildcard field tracks status
	assert.Equal(t, token.IsWildcard, merged123.Tokens[2].Wildcard)
	assert.Equal(t, token.TokenNumeric, merged123.Tokens[2].Type)
}

func TestForceWiden_IdenticalReturnsPointerSame(t *testing.T) {
	tl := token.NewTokenListWithTokens([]token.Token{
		{Value: "hello", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "world", Type: token.TokenWord, Wildcard: token.NotWildcard},
	})
	result := ForceWiden(tl, tl)
	assert.Same(t, tl, result, "identical input should return same pointer (zero-alloc)")
}

func TestForceWiden_DifferingPositionBecomesWildcard(t *testing.T) {
	template := token.NewTokenListWithTokens([]token.Token{
		{Value: "ERROR", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "something", Type: token.TokenWord, Wildcard: token.NotWildcard},
	})
	incoming := token.NewTokenListWithTokens([]token.Token{
		{Value: "WARN", Type: token.TokenWord, Wildcard: token.NotWildcard},
		{Value: " ", Type: token.TokenWhitespace},
		{Value: "something", Type: token.TokenWord, Wildcard: token.NotWildcard},
	})
	result := ForceWiden(template, incoming)
	require.NotNil(t, result)
	assert.NotSame(t, template, result, "different inputs should allocate new list")
	assert.Equal(t, token.IsWildcard, result.Tokens[0].Wildcard, "differing pos 0 should be wildcard")
	assert.Equal(t, token.TokenWord, result.Tokens[0].Type, "type should be preserved")
	assert.Equal(t, token.NotWildcard, result.Tokens[2].Wildcard, "identical pos 2 should stay")
	assert.Equal(t, "something", result.Tokens[2].Value, "identical value preserved")
}

func TestForceWiden_LengthMismatchReturnsNil(t *testing.T) {
	short := token.NewTokenListWithTokens([]token.Token{
		{Value: "A", Type: token.TokenWord},
	})
	long := token.NewTokenListWithTokens([]token.Token{
		{Value: "A", Type: token.TokenWord},
		{Value: "B", Type: token.TokenWord},
	})
	assert.Nil(t, ForceWiden(short, long))
	assert.Nil(t, ForceWiden(long, short))
}
