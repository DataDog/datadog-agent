// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

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
		adjacencies:        make([][]bool, end),
		minimumTokenLength: minimumTokenLength,
	}
	for _, tokens := range inputData {
		g.add(tokens)
	}
	return g
}

// add adds a sequence of tokens to the graph.
func (m *TokenGraph) add(tokens []Token) {
	lastToken := tokens[0]
	for _, token := range tokens[1:] {
		if m.adjacencies[lastToken] == nil {
			m.adjacencies[lastToken] = make([]bool, end)
		}
		m.adjacencies[lastToken][token] = true
		lastToken = token
	}
}

// MatchProbability returns the probability of a sequence of tokens being represented by the graph.
func (m *TokenGraph) MatchProbability(tokens []Token) MatchContext {
	if len(tokens) < 2 {
		return MatchContext{}
	}

	matches := make([]int, len(tokens)-1)

	lastToken := tokens[0]
	for i, token := range tokens[1:] {
		if m.adjacencies[lastToken] != nil && m.adjacencies[lastToken][token] {
			matches[i] = 1
		} else {
			matches[i] = -1
		}
		lastToken = token
	}

	// At this point we have a sequence of 1 and -1 where 1 represents a valid transition between tokens (a match).
	// However a sequecne of matchs could occur anywhere in the input so we need to find the subsequence that will produce
	// best probability when taking the average of the 1 and -1 sequence.
	start, end := maxSubsequence(matches)
	subSeq := matches[start:end]

	// Reject sequences of tokens that are less than the minimum token length.
	if len(subSeq) < m.minimumTokenLength {
		return MatchContext{}
	}

	return MatchContext{
		probability: avg(subSeq),
		start:       start,
		end:         end,
	}
}

// maxSubsequence is a modified Kadane’s Algorithm that returns the start and end indices of the largest subsequence
func maxSubsequence(arr []int) (int, int) {
	maxSum := arr[0]
	currentSum := arr[0]
	start := 0
	end := 0
	tempStart := 0

	for i := 1; i < len(arr); i++ {
		v := arr[i]
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
	return start, end + 1
}

func avg(states []int) float64 {
	sum := float64(0)
	for _, n := range states {
		sum += float64(n)
	}

	return sum / float64(len(states))
}
