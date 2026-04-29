// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"strconv"
	"strings"
)

// This file contains test functions that exercise continuation with string
// pointer chasing. A slice of strings where each string's backing data is
// chased independently lets us verify that:
//   - Many medium strings (~512 bytes each) are captured across continuation
//     fragments.
//   - One very large string (>32KiB) is correctly skipped because it cannot
//     fit in a single scratch buffer.

// makeTestString builds a string of exactly n bytes. The format is the decimal
// length followed by 'x' characters to fill the remainder.
func makeTestString(n int) string {
	prefix := strconv.Itoa(n)
	if n <= len(prefix) {
		return prefix[:n]
	}
	return prefix + strings.Repeat("x", n-len(prefix))
}

//nolint:all
//go:noinline
func testManyStrings(ss []string) {}

//nolint:all
func executeContinuationStringFuncs() {
	// Build ~250 strings of 512 bytes each ≈ 128KiB total, plus one 40KiB
	// string that exceeds the 32KiB scratch buffer on its own.
	ss := make([]string, 0, 252)
	for i := range 125 {
		_ = i
		ss = append(ss, makeTestString(512))
	}
	ss = append(ss, makeTestString(40*1024)) // 40KiB — too large for single buffer
	for i := range 126 {
		_ = i
		ss = append(ss, makeTestString(512))
	}
	testManyStrings(ss)
}
