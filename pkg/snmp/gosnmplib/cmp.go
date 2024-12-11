// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"fmt"
	"strconv"
	"strings"
)

// OIDToInts converts a string OID into a list of integers.
func OIDToInts(oid string) ([]int, error) {
	oid = strings.Trim(strings.TrimLeft(oid, "."), ".")
	if len(oid) == 0 {
		return nil, nil
	}
	var result []int
	for _, segment := range strings.Split(oid, ".") {
		val, err := strconv.ParseInt(segment, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("unparseable OID %q: %w", oid, err)
		}
		result = append(result, int(val))
	}
	return result, nil
}

// OIDRelation indicates how one OID relates to another
type OIDRelation uint8

const (
	// EQUAL indicates two OIDs are the same
	EQUAL OIDRelation = iota
	// GREATER indicates that at the first point where the two OIDs differ, the first is greater than the second.
	GREATER
	// LESS indicates that at the first point where the two OIDs differ, the first is less than the second.
	LESS
	// PARENT indicates that the first OID is a parent of the second
	PARENT
	// CHILD indicates that the first OID is a child of the second
	CHILD
)

// IsAfter returns true if the first OID comes lexicographically after the
// second, either because it is strictly greater than the second or because it
// is a child of the second.
func (o OIDRelation) IsAfter() bool {
	return o == GREATER || o == CHILD
}

// IsBefore returns true if the first OID comes lexicographically before the
// second, either because it is strictly less than the second or because it is a
// parent of the second.
func (o OIDRelation) IsBefore() bool {
	return o == LESS || o == PARENT
}

// CmpOIDs compares two OIDs (int slices) and indicates how the first relates to
// the second.
func CmpOIDs(oid1, oid2 []int) OIDRelation {
	for i, n := range oid1 {
		if i >= len(oid2) {
			return CHILD
		}
		if n < oid2[i] {
			return LESS
		}
		if n > oid2[i] {
			return GREATER
		}
	}
	if len(oid1) == len(oid2) {
		return EQUAL
	}
	return PARENT
}
