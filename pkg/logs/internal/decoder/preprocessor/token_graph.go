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

	// A function used to look up a match in the graph for the pair of tokens at idx. The
	// transition at idx is fully determined by ts, so this is free of position state and may
	// be called for any index, in any order, as many times as needed.
	matchForIndex := func(idx int) int {
		if m.adjacencies[ts[idx]] != nil && m.adjacencies[ts[idx]][ts[idx+1]] {
			return 1
		}
		return -1
	}

	// Look up each token transition and mark it with a 1 (match) or -1 (no match). From this
	// we must compute the subsequence that has the highest probability of being a match.
	// This code may seem overcomplicated but it's designed this way to avoid allocating an additional buffer to
	// store the matches while remaining testable and clear.
	//
	// Scoring happens in two steps. The largest subsequence establishes that the input holds a
	// run of transitions long enough, and matching enough, to be worth scoring at all. The
	// density of the best window inside that run is then the probability. Reporting the
	// average over the largest subsequence itself conflates the two: because the largest
	// subsequence maximises the sum, it keeps absorbing stretches that sum positive even
	// though they lower the average, so a perfectly matching timestamp is scored down by
	// whatever happens to follow it on the line.
	sumStart, sumEnd := maxSumSubsequence(len(ts)-1, matchForIndex)

	// Reject sequences of tokens that are less than the minimum token length. The floor of 1
	// keeps the window handed to maxAverageSubsequence non-empty when there is no minimum.
	if sumEnd-sumStart < max(m.minimumTokenLength, 1) {
		return MatchContext{}
	}

	avg, start, end := maxAverageSubsequence(sumStart, sumEnd, m.minimumTokenLength, matchForIndex)

	return MatchContext{
		probability: avg,
		start:       start,
		end:         end,
	}
}

// maxSumSubsequence is a modified Kadane’s Algorithm that returns the start and end (exclusive)
// of the largest subsequence. It takes a length of the target input, and a function used to look
// up values for each index.
func maxSumSubsequence(length int, matchForIndex func(idx int) int) (int, int) {
	if length <= 0 {
		return 0, 0
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
	return start, end
}

// prefixRingCapacity is the stack allocated capacity of the prefix sum ring in
// maxAverageSubsequence. A minLength past half of this falls back to the heap, which no caller
// does today: minimumTokenLength is 8, needing 16 entries.
const prefixRingCapacity = 64

// maxAverageSubsequence returns the average, start, and end (exclusive) of the window within
// [from, to) with the highest average value, among those at least minLength long. It takes a
// function used to look up values for each index, called at most once per index in ascending
// order, and every value it returns must be 1 or -1. If the range is shorter than minLength
// there is no such window and it returns 0, from, to.
//
// Only windows shorter than 2*minLength are searched. Any longer window splits into two parts
// that each satisfy the minimum, and a weighted average never exceeds both of its parts, so one
// part always scores at least as high.
func maxAverageSubsequence(from, to, minLength int, matchForIndex func(idx int) int) (float64, int, int) {
	if minLength < 1 {
		minLength = 1
	}

	// ring[i&mask] holds the sum of the values in [from, i). Windows reach back at most
	// 2*minLength-1 values, so only the trailing 2*minLength sums are ever read, and the entry
	// each iteration overwrites is always one older than that. The ring is rounded up to a
	// power of two so that wrapping an index is a mask rather than an integer division.
	size := 1
	for size < 2*minLength {
		size <<= 1
	}
	var scratch [prefixRingCapacity]int
	var ring []int
	if size <= prefixRingCapacity {
		ring = scratch[:size]
	} else {
		ring = make([]int, size)
	}
	mask := size - 1
	ring[from&mask] = 0

	// The best window so far is kept as a sum over a length rather than a float so that
	// comparing two candidates stays in integer arithmetic. Dividing once at the end instead of
	// once per candidate matters: this runs for every log line that reaches the detector.
	bestSum := 0
	bestLen := 0
	start := from
	end := to
	sum := 0

	for i := from + 1; i <= to; i++ {
		sum += matchForIndex(i - 1)
		ring[i&mask] = sum

		for windowLen := minLength; windowLen < 2*minLength && windowLen <= i-from; windowLen++ {
			windowStart := i - windowLen
			windowSum := sum - ring[windowStart&mask]
			// windowSum/windowLen > bestSum/bestLen, without the division.
			if bestLen == 0 || windowSum*bestLen > bestSum*windowLen {
				bestSum = windowSum
				bestLen = windowLen
				start = windowStart
				end = i
			}
		}

		// Every value is 1 or -1, so an all-match window is the highest average there is and
		// no later window can displace it. Most real timestamps are one, at the head of the
		// line, which is exactly where stopping early saves the most.
		if bestLen != 0 && bestSum == bestLen {
			break
		}
	}

	if bestLen == 0 {
		return 0, from, to
	}
	return float64(bestSum) / float64(bestLen), start, end
}
