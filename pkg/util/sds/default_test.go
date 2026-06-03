// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sds

//nolint:revive
package sds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultScanner validates that the process-wide default scanner is
// configured with the built-in email rule and redacts email addresses.
func TestDefaultScanner(t *testing.T) {
	require := require.New(t)

	s := DefaultScanner()
	require.NotNil(s, "the default scanner should not be nil")
	require.True(s.IsReady(), "the default scanner should be ready with the built-in email rule")

	rule, err := s.GetRuleByIdx(0)
	require.NoError(err, "the default scanner should have one configured rule")
	require.Equal(DefaultEmailRuleID, rule.ID, "the default rule should be the built-in email rule")
	require.Equal("Standard Email Address Scanner", rule.Name, "unexpected default rule name")

	tests := map[string]struct {
		matched bool
		event   string
	}{
		"contact me at john@example.com please": {
			matched: true,
			event:   "contact me at [redacted] please",
		},
		"no email here": {
			matched: false,
			event:   "",
		},
	}

	for input, want := range tests {
		matched, processed, err := Scan([]byte(input))
		require.NoError(err, "scanning should not fail")
		require.Equal(want.matched, matched, "unexpected match/non-match for %q", input)
		require.Equal(want.event, string(processed), "unexpected processed event for %q", input)
	}
}
