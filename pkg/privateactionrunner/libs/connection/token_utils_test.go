// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simpleToken is a minimal ConnectionToken used across all token utility tests.
type simpleToken struct {
	segments []string
}

func (s simpleToken) GetNameSegments() []string { return s.segments }

// tok is a shorthand constructor for simpleToken.
func tok(segments ...string) simpleToken { return simpleToken{segments: segments} }

// TestGetName_ReturnsLastSegment verifies that GetName extracts the final segment, which is
// the human-readable token identifier used in log messages and credential lookups.
func TestGetName_ReturnsLastSegment(t *testing.T) {
	assert.Equal(t, "password", GetName(tok("credentials", "password")))
	assert.Equal(t, "apikey", GetName(tok("apikey")))
	assert.Equal(t, "token", GetName(tok("group", "subgroup", "token")))
}

// TestGroupTokens_GroupsByParentPath verifies that tokens with the same parent path end up
// in the same group, and single-segment tokens form their own group keyed by their name.
func TestGroupTokens_GroupsByParentPath(t *testing.T) {
	tokens := []simpleToken{
		tok("credentials", "username"),
		tok("credentials", "password"),
		tok("apikey"),
	}

	result := GroupTokens(tokens)

	require.Len(t, result["credentials"], 2, "both tokens under 'credentials' must be grouped together")
	require.Len(t, result["apikey"], 1, "single-segment token should form its own group")
}

// TestGroupTokens_EmptySliceReturnsEmpty verifies that no groups are created for an empty input.
func TestGroupTokens_EmptySliceReturnsEmpty(t *testing.T) {
	result := GroupTokens([]simpleToken{})
	assert.Empty(t, result)
}

// TestGroupTokens_SingleToken verifies that a single-element slice produces one group.
func TestGroupTokens_SingleToken(t *testing.T) {
	result := GroupTokens([]simpleToken{tok("a", "b")})
	require.Len(t, result["a"], 1)
}

// TestGroupTokensByLevel_GroupsByElementAtLevel verifies that tokens are keyed by the
// element at the requested hierarchy level.
func TestGroupTokensByLevel_GroupsByElementAtLevel(t *testing.T) {
	tokens := []simpleToken{
		tok("root", "alpha", "leaf1"),
		tok("root", "alpha", "leaf2"),
		tok("root", "beta", "leaf3"),
	}

	result := GroupTokensByLevel(tokens, 1)

	require.Len(t, result["alpha"], 2, "two tokens share level-1 segment 'alpha'")
	require.Len(t, result["beta"], 1)
}

// TestGroupTokensByLevel_LevelBeyondLength_FallsBackToName verifies that when the requested
// level exceeds the token's segment count, GetName (last segment) is used as the key.
func TestGroupTokensByLevel_LevelBeyondLength_FallsBackToName(t *testing.T) {
	tokens := []simpleToken{tok("a", "b")}

	// level=5 exceeds len(segments)=2
	result := GroupTokensByLevel(tokens, 5)

	require.Len(t, result["b"], 1, "should fall back to GetName ('b') when level is out of range")
}

// TestGroupTokensByLevel_LevelZeroGroups verifies grouping by the first segment (index 0).
func TestGroupTokensByLevel_LevelZeroGroups(t *testing.T) {
	tokens := []simpleToken{
		tok("root", "x"),
		tok("root", "y"),
		tok("other", "z"),
	}

	result := GroupTokensByLevel(tokens, 0)

	require.Len(t, result["root"], 2)
	require.Len(t, result["other"], 1)
}

// TestGetSingleToken_ReturnsFirstElement verifies that the first element is returned when
// the slice is non-empty.
func TestGetSingleToken_ReturnsFirstElement(t *testing.T) {
	tokens := []simpleToken{tok("first"), tok("second")}
	result := GetSingleToken(tokens)
	assert.Equal(t, []string{"first"}, result.segments)
}

// TestGetSingleToken_EmptySliceReturnsZeroValue verifies that an empty slice returns the
// zero value for the type rather than panicking.
func TestGetSingleToken_EmptySliceReturnsZeroValue(t *testing.T) {
	result := GetSingleToken([]simpleToken{})
	assert.Nil(t, result.segments, "zero-value simpleToken must have nil segments")
}
