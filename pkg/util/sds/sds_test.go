// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sds

package sds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var testRules = []RuleDefinition{
	{ID: "email", Regex: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`},
	{ID: "ip", Regex: `(?:\d+\.){3}\d+`},
}

// TestScannerScan exercises the cgo-backed scanner end to end (proving
// libdd_sds links) on plain-string events.
func TestScannerScan(t *testing.T) {
	s, err := NewScanner(testRules)
	require.NoError(t, err, "creating the scanner should not fail")
	t.Cleanup(func() { _ = s.Close() })

	matches, err := s.Scan([]byte("contact me at john@example.com from 1.2.3.4"))
	require.NoError(t, err, "scanning should not fail")
	require.ElementsMatch(t, []Match{{RuleID: "email"}, {RuleID: "ip"}}, matches)
}

// TestScannerScanMap exercises structured (map) scanning and path reporting.
func TestScannerScanMap(t *testing.T) {
	s, err := NewScanner(testRules)
	require.NoError(t, err, "creating the scanner should not fail")
	t.Cleanup(func() { _ = s.Close() })

	matches, err := s.ScanMap(map[string]interface{}{
		"id":    float64(1),
		"name":  "dave",
		"email": "dave@example.com",
	})
	require.NoError(t, err, "scanning a row should not fail")
	require.ElementsMatch(t, []Match{{RuleID: "email", Path: "email"}}, matches)
}
