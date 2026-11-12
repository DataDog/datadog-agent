// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sort

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseHumanParts(t *testing.T) {
	cases := []struct {
		in       string
		neg      bool
		isZero   bool
		order    int
		intPart  string
		fracPart string
	}{
		// No suffix.
		{"5", false, false, 0, "5", ""},
		{"0", false, true, 0, "0", ""},
		{"42", false, false, 0, "42", ""},
		// Single-letter SI suffixes.
		{"1K", false, false, 1, "1", ""},
		{"1k", false, false, 1, "1", ""},
		{"500M", false, false, 2, "500", ""},
		{"2G", false, false, 3, "2", ""},
		{"3T", false, false, 4, "3", ""},
		{"1P", false, false, 5, "1", ""},
		{"1E", false, false, 6, "1", ""},
		{"1Z", false, false, 7, "1", ""},
		{"1Y", false, false, 8, "1", ""},
		{"1R", false, false, 9, "1", ""},
		{"1Q", false, false, 10, "1", ""},
		// Fractions.
		{"1.5G", false, false, 3, "1", "5"},
		{"0.5K", false, false, 1, "0", "5"},
		{".5K", false, false, 1, "0", "5"},
		// Negatives.
		{"-1K", true, false, 1, "1", ""},
		{"-2.5M", true, false, 2, "2", "5"},
		// Negative zero canonicalises sign.
		{"-0", false, true, 0, "0", ""},
		{"-0K", false, true, 1, "0", ""},
		// Multi-letter "Ki" — only the leading 'K' is consumed.
		{"1Ki", false, false, 1, "1", ""},
		// Trailing junk after suffix is ignored.
		{"1KB", false, false, 1, "1", ""},
		// Leading blanks are skipped.
		{"   3M", false, false, 2, "3", ""},
		{"\t7G", false, false, 3, "7", ""},
		// Unparseable: no digits at all → treated as zero with no suffix.
		{"", false, true, 0, "0", ""},
		{"abc", false, true, 0, "0", ""},
		{"K", false, true, 0, "0", ""},
		{"-", false, true, 0, "0", ""},
		// '+' is not a valid sign in GNU sort -h.
		{"+5K", false, true, 0, "0", ""},
		// Lowercase suffixes other than 'k' are not recognised.
		{"1m", false, false, 0, "1", ""},
		{"1g", false, false, 0, "1", ""},
		// Leading-zero stripping in integer part.
		{"007K", false, false, 1, "7", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			neg, isZero, order, intPart, fracPart := parseHumanParts(tc.in)
			assert.Equal(t, tc.neg, neg, "neg")
			assert.Equal(t, tc.isZero, isZero, "isZero")
			assert.Equal(t, tc.order, order, "order")
			assert.Equal(t, tc.intPart, intPart, "intPart")
			assert.Equal(t, tc.fracPart, fracPart, "fracPart")
		})
	}
}

func TestCompareHuman(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// No-suffix < any suffix when comparing equal-sign positives.
		{"5", "1K", -1},
		// 0K is value zero (sign category 0); 5 is positive — 5 > 0K.
		{"5", "0K", 1},
		// SI suffix ordering.
		{"1K", "1M", -1},
		{"1M", "1G", -1},
		{"1G", "1T", -1},
		{"1Y", "1Z", 1},
		{"1Y", "1R", -1},
		{"1R", "1Q", -1},
		// 1000Y < 1R because Y < R despite the value.
		{"1000Y", "1R", -1},
		// Within same suffix, compare numerically.
		{"500M", "1G", -1},
		{"1.5G", "2G", -1},
		{"1.5G", "1.2G", 1},
		// Fraction tie-break.
		{"1.05K", "1.5K", -1},
		// Negatives sort below positives.
		{"-1K", "0", -1},
		{"-1K", "1K", -1},
		// Within negatives, larger magnitude is smaller.
		{"-1M", "-1K", -1},
		{"-2K", "-1K", -1},
		// Unparseable lines compare as zero (and equal).
		{"abc", "xyz", 0},
		{"abc", "0", 0},
		{"abc", "0M", 0},
		// Equal values.
		{"1K", "1K", 0},
		// du -sh-style mixed input.
		{"20K", "500M", -1},
		{"500M", "1.2G", -1},
		{"20K", "1.2G", -1},
	}
	for _, tc := range cases {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			got := compareHuman(tc.a, tc.b)
			// Normalise sign — only sign matters, not magnitude.
			if got < 0 {
				got = -1
			} else if got > 0 {
				got = 1
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
