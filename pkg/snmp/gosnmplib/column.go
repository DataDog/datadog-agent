// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"strings"
)

// ExtractColumnSignature returns a string identifying the "column" of a table OID.
// This is used for filtering duplicate rows when walking SNMP tables, NOT for
// constructing new OIDs to send to devices.
//
// The algorithm uses the same heuristic as SkipOIDRowsNaive (finding the last ".1."
// which typically marks the table entry), but the result is only used as a local
// grouping key. Even if the heuristic misidentifies the column boundary, the worst
// case is that we keep extra rows - we never send fabricated OIDs to devices.
//
// For scalar OIDs (ending in .0), the entire OID is returned as its own "column".
func ExtractColumnSignature(oid string) string {
	oid = strings.TrimLeft(oid, ".")

	// Scalar OIDs (ending in .0) are their own column
	if strings.HasSuffix(oid, ".0") {
		return oid
	}

	// Find the last ".1." which typically marks the table entry
	// This is the same heuristic as SkipOIDRowsNaive
	idx := strings.LastIndex(oid, ".1.")
	if idx == -1 {
		// Not a table OID (no .1. found), treat entire OID as column
		return oid
	}

	// Extract table OID (everything before .1.)
	tableOID := oid[:idx]

	// Extract the part after .1. (column number + row index)
	rest := oid[idx+3:] // +3 to skip ".1."

	// Get just the column number (first segment after .1.)
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) == 0 {
		return oid
	}

	// Column signature = tableOID.1.columnNumber
	return tableOID + ".1." + parts[0]
}
