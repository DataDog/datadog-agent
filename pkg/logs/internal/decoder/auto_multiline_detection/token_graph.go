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
func (m *TokenGraph) MatchProbability(tokens []Token) float64 {
	if len(tokens) < 2 {
		return 0
	}

	out := make([]byte, len(tokens)-1)

	lastToken := tokens[0]
	for i, token := range tokens[1:] {
		if m.adjacencies[lastToken] != nil && m.adjacencies[lastToken][token] {
			out[i] = 1
		}
		lastToken = token
	}

	// Trim leading and trailing unmatched tokens
	trimmed := trimUnmatchedTokens(out)

	// Reject sequences of tokens that are less than the minimum token length.
	if len(trimmed) < m.minimumTokenLength {
		return 0
	}
	return avg(trimmed)
}

// trimUnmatchedTokens trims the leading and trailing zeros from a byte slice.
// Leading and trailing zeros represent tokens that were not matched
// in the graph. Since timestamps are usually contiguous, removing
// leading and trailing unmatched tokens will improve results.
func trimUnmatchedTokens(states []byte) []byte {
	start := 0
	for i, n := range states {
		if n != 0 {
			start = i
			break
		}
	}

	end := len(states)
	for i := len(states) - 1; i >= 0; i-- {
		if states[i] != 0 {
			end = i + 1
			break
		}
	}

	return states[start:end]
}

func avg(states []byte) float64 {
	sum := float64(0)
	for _, n := range states {
		sum += float64(n)
	}

	return sum / float64(len(states))
}
