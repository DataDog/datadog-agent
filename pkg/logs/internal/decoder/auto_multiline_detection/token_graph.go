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

type matchContext struct {
	probability float64
	start       int
	end         int
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
func (m *TokenGraph) MatchProbability(tokens []Token) matchContext {
	if len(tokens) < 2 {
		return matchContext{}
	}

	out := make([]byte, len(tokens)-1)

	lastToken := tokens[0]
	for i, token := range tokens[1:] {
		if m.adjacencies[lastToken] != nil && m.adjacencies[lastToken][token] {
			out[i] = 1
		}
		lastToken = token
	}

	start, end := maxSubsequence(out)
	subSeq := out[start:end]

	// Reject sequences of tokens that are less than the minimum token length.
	if len(subSeq) < m.minimumTokenLength {
		return matchContext{}
	}

	return matchContext{
		probability: avg(subSeq),
		start:       start,
		end:         end,
	}
}

// maxSubsequence is a modified Kadane’s Algorithm.
// The input sequence of 1 and 0 is evaluated as 1 and -1 to compute the max sum.
// The idea is to find the subsequence that will produce the highest average
// when taking the average of the 1 and 0 sequence.
func maxSubsequence(arr []byte) (int, int) {
	v := int(arr[0])
	if v == 0 {
		v = -1
	}
	maxSum := v
	currentSum := v
	start := 0
	end := 0
	tempStart := 0

	for i := 1; i < len(arr); i++ {
		v := int(arr[i])
		if v == 0 {
			v = -1
		}
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

func avg(states []byte) float64 {
	sum := float64(0)
	for _, n := range states {
		sum += float64(n)
	}

	return sum / float64(len(states))
}
