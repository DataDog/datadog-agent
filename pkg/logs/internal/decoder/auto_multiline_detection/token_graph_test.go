// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchEmpty(t *testing.T) {
	emptyTokenGraph := NewTokenGraph(0, nil)
	assert.Equal(t, float64(0), emptyTokenGraph.MatchProbability([]Token{}))
}

func TestExpectedMatch(t *testing.T) {
	graph := NewTokenGraph(0, [][]Token{{1, 2, 3}})
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{1, 2, 3}), "Input should match exactly")
	assert.Equal(t, float64(0), graph.MatchProbability([]Token{3, 2, 1}), "Backwards input should not match because the graph is directed")
	assert.Equal(t, float64(0), graph.MatchProbability([]Token{4, 5, 6}), "Unknown input should not match")

	graph = NewTokenGraph(0, [][]Token{{1, 2, 3}, {3, 2, 1}})
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{1, 2, 3}), "Input should match exactly")
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{3, 2, 1}), "Backwards input should match")
	assert.Equal(t, float64(0), graph.MatchProbability([]Token{4, 5, 6}), "Unknown input should not match")

	graph = NewTokenGraph(0, [][]Token{{1, 2, 3, 4, 5, 6}})
	assert.Equal(t, float64(1), graph.MatchProbability([]Token{7, 2, 3, 4, 5, 8}), "Input should match because unmatch tokens are trimmed")
	// Given tokens:          {1, 2, 3, 4, 9, 5, 6}
	// Expected relationships: [1, 1, 1, 0, 0, 1]
	// No trimming
	// 4 / 6 = 0.6666666666666666
	assert.Equal(t, float64(4)/float64(6), graph.MatchProbability([]Token{1, 2, 3, 4, 9, 5, 6}), "Input should mostly match")
}
