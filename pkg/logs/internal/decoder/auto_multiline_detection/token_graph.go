// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"

// TokenGraph is a directed cyclic graph of tokens that model the relationship between any two tokens.
// It is used to calculate the probability of an unknown sequence of tokens being represented by the graph.
type TokenGraph struct {
	adjacencies        [][]bool
	minimumTokenLength int
}

// MatchContext is the context of a match.
type MatchContext struct {
	probability float64
	// start and end are the indices of the token subsequence that produced the highest probability.
	start int
	end   int
}

// NewTokenGraph returns a new TokenGraph.
func NewTokenGraph(minimumTokenLength int, inputData [][]tokens.Token) *TokenGraph {
	g := &TokenGraph{
		adjacencies:        make([][]bool, tokens.End),
		minimumTokenLength: minimumTokenLength,
	}
	for _, tokens := range inputData {
		g.add(tokens)
	}
	return g
}

// add adds a sequence of tokens to the graph.
func (m *TokenGraph) add(ts []tokens.Token) {
	lastToken := ts[0]
	for _, token := range ts[1:] {
		if m.adjacencies[lastToken] == nil {
			m.adjacencies[lastToken] = make([]bool, tokens.End)
		}
		m.adjacencies[lastToken][token] = true
		lastToken = token
	}
}

// MatchProbability returns the probability of a sequence of tokens being represented by the graph.
func (m *TokenGraph) MatchProbability(ts []tokens.Token) MatchContext {
	if len(ts) < m.minimumTokenLength {
		return MatchContext{}
	}

	lastToken := ts[0]
	// A function used by maxSubsequence to look up a match in the graph for a pair of tokens.
	matchForIndex := func(idx int) int {
		match := -1
		if m.adjacencies[lastToken] != nil && m.adjacencies[lastToken][ts[idx+1]] {
			match = 1
		}
		lastToken = ts[idx+1]
		return match
	}

	// Look up each token transition and mark it with a 1 (match) or -1 (no match). From this
	// we must compute the subsequences that have the highest probability of being a match.
	// This code may seem overcomplicated but it's designed this way to avoid allocating an additional buffer to
	// store the matches while remaining testable and clear.
	avg, start, end := maxSubsequence(len(ts)-1, matchForIndex)

	// Reject sequences of tokens that are less than the minimum token length.
	if end-start < m.minimumTokenLength {
		return MatchContext{}
	}

	return MatchContext{
		probability: avg,
		start:       start,
		end:         end,
	}
}

// maxSubsequence is a modified Kadane’s Algorithm that returns the average, start, and end of the largest subsequence.
// It takes a length of the target input, and a function used to look up values for each index.
func maxSubsequence(length int, matchForIndex func(idx int) int) (float64, int, int) {
	if length == 0 {
		return 0, 0, 0
	}
	maxSum := matchForIndex(0)
	currentSum := maxSum
	start := 0
	end := 0
	tempStart := 0

	for i := 1; i < length; i++ {
		v := matchForIndex(i)
		if v > currentSum+v {
			currentSum = v
			tempStart = i
		} else {
			currentSum += v
		}

		if currentSum > maxSum {
			maxSum = currentSum
			start = tempStart
			end = i
		}
	}
	end++
	return float64(maxSum) / float64(end-start), start, end
}
