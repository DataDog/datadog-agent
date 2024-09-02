// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type row struct {
	tokens    []tokens.Token
	label     Label
	count     int64
	lastIndex int64
}

// DiagnosticRow is a struct that represents a diagnostic view of a row in the PatternTable.
type DiagnosticRow struct {
	TokenString string
	LabelString string
	Count       int64
	LastIndex   int64
}

// PatternTable is a table of patterns that occur over time from a log source.
// The pattern table is always sorted by the frequency of the patterns. When the table
// becomes full, the least recently updated pattern is evicted.
type PatternTable struct {
	table          []*row
	index          int64
	maxTableSize   int
	matchThreshold float64
}

// NewPatternTable returns a new PatternTable heuristic.
func NewPatternTable(maxTableSize int, matchThreshold float64) *PatternTable {
	return &PatternTable{
		table:          make([]*row, 0, maxTableSize),
		index:          0,
		maxTableSize:   maxTableSize,
		matchThreshold: matchThreshold,
	}
}

// insert adds a pattern to the table and returns the index
func (p *PatternTable) insert(tokens []tokens.Token, label Label) int {
	p.index++
	foundIdx := -1
	for i, r := range p.table {
		if isMatch(r.tokens, tokens, p.matchThreshold) {
			r.count++
			r.label = label
			r.lastIndex = p.index
			foundIdx = i
			break
		}
	}

	if foundIdx == 0 {
		return foundIdx
	}

	if foundIdx > 0 {
		return p.siftUp(foundIdx)
	}

	// If the table is full, make room for a new entry
	if len(p.table) >= p.maxTableSize {
		p.evictLRU()
	}

	p.table = append(p.table, &row{
		tokens:    tokens,
		label:     label,
		count:     1,
		lastIndex: p.index,
	})
	return len(p.table) - 1

}

// siftUp moves the row at the given index up the table until it is in the correct position.
func (p *PatternTable) siftUp(idx int) int {
	for idx != 0 && p.table[idx].count > p.table[idx-1].count {
		p.table[idx], p.table[idx-1] = p.table[idx-1], p.table[idx]
		idx--
	}
	return idx
}

// evictLRU removes the least recently updated row from the table.
func (p *PatternTable) evictLRU() {
	mini := 0
	minIndex := p.index
	for i, r := range p.table {
		if r.lastIndex < minIndex {
			minIndex = r.lastIndex
			mini = i
		}
	}
	p.table = append(p.table[:mini], p.table[mini+1:]...)
}

// DumpTable returns a slice of DiagnosticRow structs that represent the current state of the table.
func (p *PatternTable) DumpTable() []DiagnosticRow {
	debug := make([]DiagnosticRow, 0, len(p.table))
	for _, r := range p.table {
		debug = append(debug, DiagnosticRow{
			TokenString: tokensToString(r.tokens),
			LabelString: labelToString(r.label),
			Count:       r.count,
			LastIndex:   r.lastIndex})
	}
	return debug
}

// ProcessAndContinue adds a pattern to the table and updates its label based on it's frequency.
// This implements the Herustic interface - so we should stop processing if the label was changed
// due to pattern detection.
func (p *PatternTable) ProcessAndContinue(context *messageContext) bool {

	if context.tokens == nil {
		log.Error("Tokens are required to process patterns")
		return true
	}

	p.insert(context.tokens, context.label)
	return true
}
