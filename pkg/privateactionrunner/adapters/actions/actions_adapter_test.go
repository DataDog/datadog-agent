// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSplitFQN_SimpleCase verifies the basic split: the bundle name is everything before
// the last dot, and the action name is everything after it.
func TestSplitFQN_SimpleCase(t *testing.T) {
	bundle, action := SplitFQN("com.datadoghq.http.sendRequest")
	assert.Equal(t, "com.datadoghq.http", bundle)
	assert.Equal(t, "sendRequest", action)
}

// TestSplitFQN_NoDot verifies that a string with no dot returns two empty strings,
// which lets callers detect an invalid FQN without a separate validation step.
func TestSplitFQN_NoDot(t *testing.T) {
	bundle, action := SplitFQN("sendRequest")
	assert.Empty(t, bundle)
	assert.Empty(t, action)
}

// TestSplitFQN_SingleDot verifies that "bundle.action" (one dot) is split correctly,
// the most minimal valid FQN.
func TestSplitFQN_SingleDot(t *testing.T) {
	bundle, action := SplitFQN("mybundle.myaction")
	assert.Equal(t, "mybundle", bundle)
	assert.Equal(t, "myaction", action)
}

// TestSplitFQN_Empty verifies that an empty string returns two empty strings.
func TestSplitFQN_Empty(t *testing.T) {
	bundle, action := SplitFQN("")
	assert.Empty(t, bundle)
	assert.Empty(t, action)
}

// TestSplitFQN_TrailingDot verifies that a trailing dot produces an empty action name
// and the full prefix as the bundle name, reflecting the last-dot split rule.
func TestSplitFQN_TrailingDot(t *testing.T) {
	bundle, action := SplitFQN("com.datadoghq.")
	assert.Equal(t, "com.datadoghq", bundle)
	assert.Equal(t, "", action)
}

// TestIsHttpBundle_MatchesHttpBundleId verifies that the canonical HTTP bundle ID returns true.
func TestIsHttpBundle_MatchesHttpBundleId(t *testing.T) {
	assert.True(t, IsHttpBundle("com.datadoghq.http"))
}

// TestIsHttpBundle_RejectsOtherBundleIds verifies that non-HTTP bundle IDs return false,
// including a similar-looking but distinct ID.
func TestIsHttpBundle_RejectsOtherBundleIds(t *testing.T) {
	assert.False(t, IsHttpBundle("com.datadoghq.script"))
	assert.False(t, IsHttpBundle("com.datadoghq.http.extra"))
	assert.False(t, IsHttpBundle(""))
}
