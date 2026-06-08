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

// testRuleIDs are the ids used by testRules, mirroring the built-in rule order
// (email at index 0, IPv4 at index 1).
var testRuleIDs = []string{
	"PuXiVTCkTHOtj0Yad1ppsw", // Email
	"aDA3jUjSSLOezHV2y-Rn_w", // IPv4
}

// testRules is a known rule set the tests reconfigure the process-wide scanner
// with, so they don't depend on the lazily-built default or on the global
// scanner state left behind by another test.
var testRules = []RuleDefinition{
	{ID: testRuleIDs[0], Regex: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`},
	{ID: testRuleIDs[1], Regex: `(?:\d+\.){3}\d+`},
}

// TestScan validates that, after reconfiguring the scanner, it reports matches
// on emails and IPs and none on non-matching events.
func TestScan(t *testing.T) {
	require := require.New(t)
	require.NoError(Reconfigure(testRules), "reconfiguring the scanner should not fail")

	matches, err := Scan([]byte("contact me at john@example.com please"))
	require.NoError(err, "scanning should not fail")
	require.Len(matches, 1, "the email rule should report exactly one match")
	require.Equal(uint32(0), matches[0].RuleIdx, "the match should reference the email rule")

	matches, err = Scan([]byte("server ip is 192.168.1.10 today"))
	require.NoError(err, "scanning should not fail")
	require.Len(matches, 1, "the IP rule should report exactly one match")
	require.Equal(uint32(1), matches[0].RuleIdx, "the match should reference the IP rule")

	matches, err = Scan([]byte("no email here"))
	require.NoError(err, "scanning should not fail")
	require.Empty(matches, "a non-matching event should report no match")
}

// TestScanMap validates that scanning a structured event (a database row)
// reports matches on the email column and none when there is no sensitive data.
func TestScanMap(t *testing.T) {
	require := require.New(t)
	require.NoError(Reconfigure(testRules), "reconfiguring the scanner should not fail")

	matches, err := ScanMap(map[string]interface{}{
		"id":    float64(104),
		"name":  "dave",
		"email": "dave@example.com",
	})
	require.NoError(err, "scanning a row should not fail")
	require.NotEmpty(matches, "the email column should report at least one match")
	require.Equal(uint32(0), matches[0].RuleIdx, "the match should reference the email rule")

	matches, err = ScanMap(map[string]interface{}{
		"id":   float64(1),
		"name": "no sensitive data",
	})
	require.NoError(err, "scanning a row should not fail")
	require.Empty(matches, "a row without sensitive data should report no match")
}

// TestReconfigure validates that reconfiguring the process-wide scanner with a
// new rule set takes effect: only the new rules match, and RuleID resolves the
// match index back to the reconfigured rule id.
func TestReconfigure(t *testing.T) {
	require := require.New(t)

	require.NoError(Reconfigure([]RuleDefinition{
		{ID: "custom-email-rule", Regex: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`},
	}), "reconfiguring the scanner should not fail")

	matches, err := Scan([]byte("contact me at john@example.com please"))
	require.NoError(err, "scanning should not fail")
	require.Len(matches, 1, "the reconfigured email rule should report exactly one match")
	require.Equal(uint32(0), matches[0].RuleIdx, "the match should reference the only rule")
	require.Equal("custom-email-rule", RuleID(matches[0].RuleIdx), "RuleID should resolve the reconfigured id")

	// The IP rule is gone after reconfiguring, so an IP no longer matches.
	matches, err = Scan([]byte("server ip is 192.168.1.10 today"))
	require.NoError(err, "scanning should not fail")
	require.Empty(matches, "the IP rule should no longer match after reconfiguring")

	// Restore the built-in-like rule set for any test that runs afterwards.
	require.NoError(Reconfigure(testRules), "restoring the scanner should not fail")
}
