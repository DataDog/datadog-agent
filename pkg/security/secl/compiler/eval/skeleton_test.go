// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSkeletonSpan(t *testing.T) {
	for _, test := range []struct {
		expr     string
		start    int
		expected string
	}{
		{expr: `a == "x" && b == "y"`, start: 0, expected: `a == "x"`},
		{expr: `a == "x" && b == "y"`, start: 12, expected: `b == "y"`},
		{expr: `a == "x" and b == "y"`, start: 0, expected: `a == "x"`},
		{expr: `(a == "x" || b == "y") && c == 1`, start: 1, expected: `a == "x"`},
		{expr: `(a == "x" || b == "y") && c == 1`, start: 13, expected: `b == "y"`},
		// operators inside a quoted literal are not operators
		{expr: `a == "x && y" || b == 1`, start: 0, expected: `a == "x && y"`},
		{expr: `a == "x) y" || b == 1`, start: 0, expected: `a == "x) y"`},
		{expr: `a == "x \" && y" || b == 1`, start: 0, expected: `a == "x \" && y"`},
		// an identifier ending in `or` is not the `or` operator
		{expr: `a.factor == 1 || b == 1`, start: 0, expected: `a.factor == 1`},
		{expr: `a == 1 || b.and_c == 1`, start: 10, expected: `b.and_c == 1`},
		// a comparison over a parenthesized boolean expression is one leaf
		{expr: `(a == 1 || b == 1) == true && c == 1`, start: 0, expected: `(a == 1 || b == 1) == true`},
	} {
		t.Run(test.expr, func(t *testing.T) {
			offset, length := skelSpan(test.expr, test.start)
			assert.Equal(t, test.expected, test.expr[offset:offset+length])
		})
	}
}

func TestSkeletonLeafName(t *testing.T) {
	for index, expected := range map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA"} {
		assert.Equal(t, expected, skelLeafName(index))
	}
}
