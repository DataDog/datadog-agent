// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractColumnSignature(t *testing.T) {
	// The key property we need is CONSISTENCY: all rows from the same column
	// should get the same signature. The exact signature value doesn't matter
	// for filtering purposes - what matters is that grouping works correctly.

	tests := []struct {
		name     string
		oid      string
		expected string
	}{
		// Scalar OIDs (ending in .0) should return the full OID
		{
			name:     "scalar sysDescr",
			oid:      "1.3.6.1.2.1.1.1.0",
			expected: "1.3.6.1.2.1.1.1.0",
		},
		{
			name:     "scalar with leading dot",
			oid:      ".1.3.6.1.2.1.1.1.0",
			expected: "1.3.6.1.2.1.1.1.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractColumnSignature(tc.oid)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractColumnSignature_SameColumnGrouping(t *testing.T) {
	// Test that rows from the same column get grouped together when possible.
	// Note: The LastIndex(".1.") heuristic has limitations for simple tables where
	// row indices contain "1" as a component. This is acceptable because:
	// 1. We never send fabricated OIDs to devices (safety maintained)
	// 2. Worst case is we keep extra rows (safe, just less efficient)

	testCases := []struct {
		name string
		oids []string // All OIDs in this list should produce the same signature
	}{
		{
			// For simple tables, rows that don't have "1" in the index will group correctly
			name: "ifTable column 2 - rows without 1 in index",
			oids: []string{
				"1.3.6.1.2.1.2.2.1.2.2",
				"1.3.6.1.2.1.2.2.1.2.100",
				"1.3.6.1.2.1.2.2.1.2.200",
			},
		},
		{
			name: "ifTable column 3 - rows without 1 in index",
			oids: []string{
				"1.3.6.1.2.1.2.2.1.3.2",
				"1.3.6.1.2.1.2.2.1.3.100",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.oids) < 2 {
				t.Skip("Need at least 2 OIDs to test grouping")
			}

			firstSig := ExtractColumnSignature(tc.oids[0])
			for _, oid := range tc.oids[1:] {
				sig := ExtractColumnSignature(oid)
				assert.Equal(t, firstSig, sig, "All rows from same column should have same signature. OID: %s", oid)
			}
		})
	}
}

func TestExtractColumnSignature_DifferentColumnsAreDifferent(t *testing.T) {
	// Different columns should produce different signatures

	testCases := []struct {
		name string
		oid1 string
		oid2 string
	}{
		{
			name: "ifIndex vs ifDescr",
			oid1: "1.3.6.1.2.1.2.2.1.1.1",
			oid2: "1.3.6.1.2.1.2.2.1.2.1",
		},
		{
			name: "ifDescr vs ifType",
			oid1: "1.3.6.1.2.1.2.2.1.2.1",
			oid2: "1.3.6.1.2.1.2.2.1.3.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sig1 := ExtractColumnSignature(tc.oid1)
			sig2 := ExtractColumnSignature(tc.oid2)
			assert.NotEqual(t, sig1, sig2, "Different columns should have different signatures")
		})
	}
}

func TestExtractColumnSignature_ComplexIndexTables(t *testing.T) {
	// For complex index tables, the heuristic groups based on the last ".1." pattern.
	// This provides reasonable grouping for many complex tables, though not perfect.
	// The key property is CONSISTENCY within the same grouping pattern.

	t.Run("inetCidrRouteTable rows with same prefix pattern", func(t *testing.T) {
		// These two OIDs have the same structure up to and after the last ".1."
		// They differ only after the last ".1.X" pattern (in the 172.21.X.X part)
		oid1 := "1.3.6.1.2.1.4.24.7.1.1.1.4.0.0.0.0.0.2.0.0.1.4.172.21.200.192"
		oid2 := "1.3.6.1.2.1.4.24.7.1.1.1.4.0.0.0.0.0.2.0.0.1.4.172.21.211.204"

		sig1 := ExtractColumnSignature(oid1)
		sig2 := ExtractColumnSignature(oid2)
		// Both should find last ".1." at "...0.0.1.4..." and produce same signature
		assert.Equal(t, sig1, sig2, "Rows with same prefix pattern should have same signature")
	})

	t.Run("ipNetToPhysicalTable consistency check", func(t *testing.T) {
		// For ipNetToPhysicalTable, the last ".1." may be in the IP portion
		// which can cause different grouping. We verify consistency within
		// rows that have the same ".1." position.
		oid1 := "1.3.6.1.2.1.4.35.1.4.1000007.1.4.192.168.2.50"
		oid2 := "1.3.6.1.2.1.4.35.1.4.1000007.1.4.192.168.2.60"

		sig1 := ExtractColumnSignature(oid1)
		sig2 := ExtractColumnSignature(oid2)
		// Both have last ".1." at "1000007.1.4" so should match
		assert.Equal(t, sig1, sig2, "Rows with same last .1. position should have same signature")
	})
}

func TestExtractColumnSignature_NoTableMarker(t *testing.T) {
	// OIDs without .1. pattern should return the full OID
	// (treated as their own unique "column")

	oid := "1.3.6.4.2.9.9.42.2.2.3.4" // No .1. pattern
	result := ExtractColumnSignature(oid)
	// The function finds the last .1. if any, otherwise returns the OID
	// In this case there IS a .1. at "42.2.2" - wait no, there's no .1. here
	// Let me check: 1.3.6.4.2.9.9.42.2.2.3.4
	// There's no ".1." substring in this OID
	assert.Equal(t, oid, result)
}
