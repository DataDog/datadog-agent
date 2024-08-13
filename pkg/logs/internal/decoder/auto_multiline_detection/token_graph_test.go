// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

func TestMatchEmpty(t *testing.T) {
	emptyTokenGraph := NewTokenGraph(2, nil)
	assert.Equal(t, float64(0), emptyTokenGraph.MatchProbability([]tokens.Token{}).probability)
}

func TestExpectedMatch(t *testing.T) {
	graph := NewTokenGraph(0, [][]tokens.Token{{1, 2, 3}})
	assert.Equal(t, float64(1), graph.MatchProbability([]tokens.Token{1, 2, 3}).probability, "Input should match exactly")
	assert.Equal(t, float64(-1), graph.MatchProbability([]tokens.Token{3, 2, 1}).probability, "Backwards input should not match because the graph is directed")
	assert.Equal(t, float64(-1), graph.MatchProbability([]tokens.Token{4, 5, 6}).probability, "Unknown input should not match")

	graph = NewTokenGraph(0, [][]tokens.Token{{1, 2, 3}, {3, 2, 1}})
	assert.Equal(t, float64(1), graph.MatchProbability([]tokens.Token{1, 2, 3}).probability, "Input should match exactly")
	assert.Equal(t, float64(1), graph.MatchProbability([]tokens.Token{3, 2, 1}).probability, "Backwards input should match")
	assert.Equal(t, float64(-1), graph.MatchProbability([]tokens.Token{4, 5, 6}).probability, "Unknown input should not match")

	graph = NewTokenGraph(0, [][]tokens.Token{{1, 2, 3, 4, 5, 6}})
	assert.Equal(t, float64(1), graph.MatchProbability([]tokens.Token{7, 2, 3, 4, 5, 8}).probability, "Input should match because unmatch tokens are trimmed")
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
