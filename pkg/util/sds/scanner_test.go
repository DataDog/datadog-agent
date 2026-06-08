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

// testRules is a known rule set the tests reconfigure the process-wide scanner
// with, so they don't depend on the lazily-built default or on the global
// scanner state left behind by another test.
var testRules = []RuleDefinition{
	{ID: "email", Regex: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`},
	{ID: "ip", Regex: `(?:\d+\.){3}\d+`},
}

// TestScan validates that, after reconfiguring the scanner, plain-string scans
// report the expected matches (rule id and path) and none on other events. The
// Path is empty for a plain-string scan. require.ElementsMatch is used because
// the order of matches is not part of the contract.
func TestScan(t *testing.T) {
	require.NoError(t, Reconfigure(testRules), "reconfiguring the scanner should not fail")

	cases := []struct {
		name  string
		event string
		want  []Match
	}{
		{
			name:  "email match",
			event: "contact me at john@example.com please",
			want:  []Match{{RuleID: "email"}},
		},
		{
			name:  "ipv4 match",
			event: "server ip is 192.168.1.10 today",
			want:  []Match{{RuleID: "ip"}},
		},
		{
			name:  "email and ipv4 in one event",
			event: "from a@b.co at 1.2.3.4 now",
			want:  []Match{{RuleID: "email"}, {RuleID: "ip"}},
		},
		{
			name:  "no match",
			event: "no sensitive data here",
			want:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			matches, err := Scan([]byte(c.event))
			require.NoError(t, err, "scanning should not fail")
			require.ElementsMatch(t, c.want, matches, "the scan should report exactly the expected matches")
		})
	}
}

// TestScanMap validates that scanning structured events (database rows) reports
// the expected matches (rule id and the column path that fired), including
// nested, column-oriented data. The order of matches is not deterministic (map
// keys are iterated in random order when the event is encoded), so matches are
// compared order-independently with require.ElementsMatch.
func TestScanMap(t *testing.T) {
	require.NoError(t, Reconfigure(testRules), "reconfiguring the scanner should not fail")

	cases := []struct {
		name  string
		event map[string]interface{}
		want  []Match
	}{
		{
			name: "single email column",
			event: map[string]interface{}{
				"id":    float64(104),
				"name":  "dave",
				"email": "dave@example.com",
			},
			want: []Match{{RuleID: "email", Path: "email"}},
		},
		{
			name: "no sensitive data",
			event: map[string]interface{}{
				"id":   float64(1),
				"name": "no sensitive data",
			},
			want: nil,
		},
		{
			name: "column-oriented result with metadata and data columns",
			event: map[string]interface{}{
				"metadata": map[string]interface{}{
					"host":     "10.0.0.1",
					"table":    "users",
					"schema":   "public",
					"database": "shop",
				},
				"data": map[string]interface{}{
					"id":    []interface{}{float64(1), float64(2), float64(3)},
					"name":  []interface{}{"alice", "bob", "carol"},
					"email": []interface{}{"alice@example.com", "bob@corp.io", "carol@dd.dev"},
				},
			},
			want: []Match{
				{RuleID: "ip", Path: "metadata.host"},
				{RuleID: "email", Path: "data.email[0]"},
				{RuleID: "email", Path: "data.email[1]"},
				{RuleID: "email", Path: "data.email[2]"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			matches, err := ScanMap(c.event)
			require.NoError(t, err, "scanning a row should not fail")
			require.ElementsMatch(t, c.want, matches, "the scan should report exactly the expected matches")
		})
	}
}

// TestReconfigure validates that reconfiguring the process-wide scanner with a
// new rule set takes effect: only the new rules match.
func TestReconfigure(t *testing.T) {
	require := require.New(t)

	require.NoError(Reconfigure([]RuleDefinition{
		{ID: "custom-email-rule", Regex: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`},
	}), "reconfiguring the scanner should not fail")

	matches, err := Scan([]byte("contact me at john@example.com please"))
	require.NoError(err, "scanning should not fail")
	require.Equal([]Match{{RuleID: "custom-email-rule"}}, matches, "the reconfigured email rule should report a single match")

	// The IP rule is gone after reconfiguring, so an IP no longer matches.
	matches, err = Scan([]byte("server ip is 192.168.1.10 today"))
	require.NoError(err, "scanning should not fail")
	require.Empty(matches, "the IP rule should no longer match after reconfiguring")

	// Restore the built-in-like rule set for any test that runs afterwards.
	require.NoError(Reconfigure(testRules), "restoring the scanner should not fail")
}
