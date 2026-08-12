// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

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
func NewTokenGraph(minimumTokenLength int, inputData [][]Token) *TokenGraph {
	g := &TokenGraph{
		adjacencies:        make([][]bool, End),
		minimumTokenLength: minimumTokenLength,
	}
	for _, tokens := range inputData {
		g.add(tokens)
	}
	return g
}

// add adds a sequence of tokens to the graph.
func (m *TokenGraph) add(ts []Token) {
	lastToken := ts[0]
	for _, token := range ts[1:] {
		if m.adjacencies[lastToken] == nil {
			m.adjacencies[lastToken] = make([]bool, End)
		}
		m.adjacencies[lastToken][token] = true
		lastToken = token
	}
}

// MatchProbability returns the probability of a sequence of tokens being represented by the graph.
func (m *TokenGraph) MatchProbability(ts []Token) MatchContext {
	if len(ts) < m.minimumTokenLength {
		return MatchContext{}
	}

	lastToken := ts[0]
	prefixRun := 0
	inPrefixRun := true
	// A function used by maxSubsequence to look up a match in the graph for a pair of tokens.
	// maxSubsequence walks the indices in order, so the same pass also measures the
	// leading run of matches.
	matchForIndex := func(idx int) int {
		match := -1
		if m.adjacencies[lastToken] != nil && m.adjacencies[lastToken][ts[idx+1]] {
			match = 1
		}
		lastToken = ts[idx+1]
		if inPrefixRun {
			if match == 1 {
				prefixRun++
			} else {
				inPrefixRun = false
			}
		}
		return match
	}

	// Look up each token transition and mark it with a 1 (match) or -1 (no match). From this
	// we must compute the subsequences that have the highest probability of being a match.
	// This code may seem overcomplicated but it's designed this way to avoid allocating an additional buffer to
	// store the matches while remaining testable and clear.
	avg, start, end := maxSubsequence(len(ts)-1, matchForIndex)

	// A long enough run of matches at the very start of the input is a full match,
	// whatever follows it. maxSubsequence maximises the sum rather than the average,
	// so it keeps extending past such a run while the running total still climbs,
	// and the extra tokens dilute the average it reports. A line that opens with an
	// exact match would otherwise be scored on that diluted window.
	//
	// A run of zero is not a match, even where minimumTokenLength is also zero.
	if prefixRun > 0 && prefixRun >= m.minimumTokenLength {
		return MatchContext{
			probability: 1,
			start:       0,
			end:         prefixRun,
		}
	}

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
//
// matchForIndex is called exactly once per index, in ascending order from 0, and never
// after an early return. Callers rely on that order to accumulate state as the walk
// proceeds, so preserve it when changing this function.
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
