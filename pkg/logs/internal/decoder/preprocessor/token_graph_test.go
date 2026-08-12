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
}

func TestLeadingRunIsFullMatch(t *testing.T) {
	// Transitions 1->2, 2->3 ... 9->10 are known to the graph, so a token sequence
	// counting up from 1 produces a leading run as long as it keeps counting.
	// Tokens 50 and above are absent from the graph and so always mismatch.
	graph := NewTokenGraph(4, [][]Token{{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}})

	tooShort := graph.MatchProbability([]Token{1, 2, 3, 4, 50, 51, 52, 53, 54})
	assert.Equal(t, float64(0), tooShort.probability, "A leading run one short of the minimum should not match")

	// The leading run here is exactly the minimum, and the highest-summing window
	// is the whole input, 6 matches over 10 transitions. Scored that way the result
	// would be 0.6, so these assertions fail if the leading run stops being used.
	exact := graph.MatchProbability([]Token{1, 2, 3, 4, 5, 50, 1, 2, 3, 4, 5})
	assert.Equal(t, float64(1), exact.probability, "A leading run of exactly the minimum should be a full match")
	assert.Equal(t, 0, exact.start)
	assert.Equal(t, 4, exact.end, "The reported subsequence should be the leading run, not the highest-summing window")

	longer := graph.MatchProbability([]Token{1, 2, 3, 4, 5, 6, 7, 50, 51})
	assert.Equal(t, float64(1), longer.probability)
	assert.Equal(t, 6, longer.end, "The whole leading run should be reported, not just the minimum")

	// A run that does not start at the first token is scored the usual way, so the
	// reported subsequence starts where the match does rather than at zero.
	offset := graph.MatchProbability([]Token{50, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, float64(1), offset.probability)
	assert.Equal(t, 1, offset.start, "A run that is not leading should not take the leading-run path")
}

func TestMaxSubsequence(t *testing.T) {
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
		_, start, end := maxSubsequence(len(test.input), func(idx int) int {
			return test.input[idx]
		})

		assert.Equal(t, test.expected, test.input[start:end])
	}
}
