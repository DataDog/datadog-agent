// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

// TokenGraph is a graph of tokens that model the relationship between any two tokens.
// It is used to calculate the probability of a sequence of tokens.
type TokenGraph struct {
	adjacencys         [][]bool
	minimumTokenLength int
}

// NewTokenGraph returns a new TokenGraph.
func NewTokenGraph(minimumTokenLength int, inputData [][]Token) *TokenGraph {
	g := &TokenGraph{
		adjacencys:         make([][]bool, end),
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
		if m.adjacencys[lastToken] == nil {
			m.adjacencys[lastToken] = make([]bool, end)
		}
		m.adjacencys[lastToken][token] = true
		lastToken = token
	}
}

// MatchProbability returns the probability of a sequence of tokens.
func (m *TokenGraph) MatchProbability(tokens []Token) float64 {
	out := make([]byte, len(tokens)-1)

	lastToken := tokens[0]
	for i, token := range tokens[1:] {
		if m.adjacencys[lastToken] != nil && m.adjacencys[lastToken][token] {
			out[i] = 1
		}
		lastToken = token
	}

	// Trim leading and trailing un matched tokens
	trimmed := trimStateSet(out)

	// Reject sequences of tokens that are less than the minimum token length.
	if len(trimmed) < m.minimumTokenLength {
		return 0
	}
	return avg(trimmed)
}

// trimStateSet trims the leading and trailing zeros from a byte slice.
// leading and trailing zeros represent tokens that were not matched
// in the graph. Since timestamps are usually continuous, removing
// leading and trailing unmatched tokens can improve results.
func trimStateSet(states []byte) []byte {
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
