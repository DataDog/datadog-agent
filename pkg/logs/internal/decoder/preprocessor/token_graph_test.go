// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchEmpty(t *testing.T) {
	emptyTokenGraph := NewTokenGraph(2, nil)
	assert.Equal(t, float64(0), emptyTokenGraph.MatchProbability([]Token{}).probability)
}

func TestExpectedMatch(t *testing.T) {
	graph := NewTokenGraph(0, [][]Token{{1, 2, 3}})
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{1, 2, 3}).probability, "Input should match exactly")
	assert.Equal(t, float64(-1), graph.MatchProbability([]Token{3, 2, 1}).probability, "Backwards input should not match because the graph is directed")
	assert.Equal(t, float64(-1), graph.MatchProbability([]Token{4, 5, 6}).probability, "Unknown input should not match")

	graph = NewTokenGraph(0, [][]Token{{1, 2, 3}, {3, 2, 1}})
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{1, 2, 3}).probability, "Input should match exactly")
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{3, 2, 1}).probability, "Backwards input should match")
	assert.Equal(t, float64(-1), graph.MatchProbability([]Token{4, 5, 6}).probability, "Unknown input should not match")

	graph = NewTokenGraph(0, [][]Token{{1, 2, 3, 4, 5, 6}})
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{7, 2, 3, 4, 5, 8}).probability, "Input should match because unmatch tokens are trimmed")

	// The transitions here are 1, 1, 1, 1, 1, -1, 1, 1, -1. Every one of them is inside the
	// largest subsequence because the trailing run is net positive, so averaging over that
	// subsequence reports 0.66 for an input that opens with a perfect match.
	graph = NewTokenGraph(3, [][]Token{{1, 2, 3, 1}})
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{1, 2, 3, 1, 2, 3, 2, 3, 1, 3}).probability, "A perfect match should not be scored down by a net-positive suffix")
}

func TestMaxSumSubsequence(t *testing.T) {
	tests := []struct {
		input    []int
		expected []int
	}{
		{[]int{}, []int{}},
		{[]int{1, 1, 1, 1, 1}, []int{1, 1, 1, 1, 1}},
		{[]int{-1, -1, 1, -1, -1}, []int{1}},
		{[]int{-1, 1, 1}, []int{1, 1}},
		{[]int{1, 1, -1}, []int{1, 1}},
		{[]int{-1, 1, 1, 1, -1, -1, -1, -1, 1, 1, 1, 1, -1, -1, -1, 1, 1}, []int{1, 1, 1, 1}},
		{[]int{-1, 1, 1, 1, -1, -1, -1, 1, 1, 1, 1, -1, -1, -1, 1, 1}, []int{1, 1, 1, -1, -1, -1, 1, 1, 1, 1}},
		{[]int{1, 1, 1, -1, -1, -1, -1, 1, -1, 1, 1, 1}, []int{1, 1, 1}},
		{[]int{1, -1, 1, 1, 1, -1, -1, -1, -1, 1, 1, 1}, []int{1, -1, 1, 1, 1}},
	}

	for _, test := range tests {
		start, end := maxSumSubsequence(len(test.input), func(idx int) int {
			return test.input[idx]
		})

		assert.Equal(t, test.expected, test.input[start:end])
	}
}

func TestMaxAverageSubsequence(t *testing.T) {
	tests := []struct {
		name        string
		input       []int
		minLength   int
		expected    []int
		expectedAvg float64
	}{
		{"whole input is the only candidate", []int{1, 1, 1, 1, 1}, 5, []int{1, 1, 1, 1, 1}, 1},
		{"no minimum picks the single best value", []int{-1, -1, 1, -1, -1}, 1, []int{1}, 1},
		{"all values negative", []int{-1, -1, -1, -1}, 2, []int{-1, -1}, -1},

		// The largest subsequence here is the whole input, because the trailing run is net
		// positive. Its average is 0.66, which is not the density of the best window in it.
		{"perfect prefix is not scored down by a net-positive suffix",
			[]int{1, 1, 1, 1, 1, -1, -1, 1, 1, 1, 1, 1}, 5, []int{1, 1, 1, 1, 1}, 1},

		// The largest subsequence bridges the two runs of ones through the negative values
		// between them, for an average of 0.4. The second run on its own is the better window.
		{"two runs are not bridged through the gap between them",
			[]int{-1, 1, 1, 1, -1, -1, -1, 1, 1, 1, 1, -1, -1, -1, 1, 1}, 4, []int{1, 1, 1, 1}, 1},

		// No window of the minimum length is a perfect match, so the best average is a
		// fraction and the window that achieves it is longer than the minimum.
		{"best window can be longer than the minimum",
			[]int{1, 1, -1, 1, 1, -1, 1, 1, -1}, 4, []int{1, 1, -1, 1, 1}, 0.6},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			avg, start, end := maxAverageSubsequence(0, len(test.input), test.minLength, func(idx int) int {
				return test.input[idx]
			})

			assert.Equal(t, test.expected, test.input[start:end])
			assert.Equal(t, test.expectedAvg, avg)
		})
	}
}

func TestMaxAverageSubsequenceSearchesOnlyTheGivenRange(t *testing.T) {
	// A perfect run sits outside [from, to) on both sides. Only the range handed in, which
	// is the largest subsequence when this is called for real, may be considered.
	input := []int{1, 1, 1, 1, -1, -1, 1, -1, -1, 1, 1, 1, 1}

	avg, start, end := maxAverageSubsequence(4, 9, 3, func(idx int) int {
		assert.GreaterOrEqual(t, idx, 4, "looked up an index before the start of the range")
		assert.Less(t, idx, 9, "looked up an index past the end of the range")
		return input[idx]
	})

	assert.Equal(t, []int{-1, -1, 1}, input[start:end])
	assert.InDelta(t, -1.0/3.0, avg, 1e-9)
}
