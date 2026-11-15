// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package boundedreplace

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBudgetAdd(t *testing.T) {
	b := NewBudget(10)
	assert.True(t, b.Add(4))
	assert.True(t, b.Add(6))
	assert.False(t, b.Exceeded(), "exactly the limit fits")
	assert.True(t, b.Add(0))
	assert.True(t, b.Add(-5), "negative charges are ignored")
	assert.False(t, b.Add(1))
	assert.True(t, b.Exceeded())
	assert.False(t, b.Add(0), "an exceeded budget stays exceeded")
}

func TestBudgetNegativeLimit(t *testing.T) {
	b := NewBudget(-1)
	assert.True(t, b.Exceeded())
}

func TestBudgetAddMulDoesNotOverflow(t *testing.T) {
	b := NewBudget(math.MaxInt)
	assert.False(t, b.AddMul(math.MaxInt, 2))
	assert.True(t, b.Exceeded())

	b = NewBudget(12)
	assert.True(t, b.AddMul(3, 4))
	assert.False(t, b.AddMul(1, 1))

	b = NewBudget(1)
	assert.True(t, b.AddMul(math.MaxInt, 0))
	assert.True(t, b.AddMul(0, math.MaxInt))
	assert.True(t, b.AddMul(-3, 7))
}

func TestAddReplaceAllMatchesStringsReplaceAll(t *testing.T) {
	cases := []struct{ s, old, new string }{
		{"", "{}", "x"},
		{"{}", "{}", ""},
		{"a{}b{}c", "{}", "LONG-VALUE"},
		{"{}{}{}", "{}", "x"},
		{"{{}}", "{}", "{}"},
		{"aaaa", "aa", "b"},
		{"héllo", "", "-"},
		{"no marker", "@", "value"},
	}
	for _, tc := range cases {
		want := len(strings.ReplaceAll(tc.s, tc.old, tc.new))
		for limit := max(0, want-2); limit <= want+2; limit++ {
			b := NewBudget(limit)
			fits := b.AddReplaceAll(tc.s, tc.old, tc.new)
			assert.Equal(t, want <= limit, fits, "%q/%q/%q limit %d (result %d)", tc.s, tc.old, tc.new, limit, want)
			assert.Equal(t, !fits, b.Exceeded())
		}
	}
}

func TestAddReplaceAllHugeResultDoesNotOverflow(t *testing.T) {
	b := NewBudget(MaxCommandLineBytes)
	assert.False(t, b.AddReplaceAll(strings.Repeat("{}", 1<<20), "{}", strings.Repeat("x", 1<<20)))
}
