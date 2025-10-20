// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"slices"
	"sync"
)

type row2 struct {
	tokenizedMessage *TokenizedMessage
	count            int64
	lastIndex        int64
}

// DiagnosticRow is a struct that represents a diagnostic view of a row in the PatternTable.
type DiagnosticRow2 struct {
	TokenString     string
	LabelString     string
	labelAssignedBy string
	Count           int64
	LastIndex       int64
}

// PatternTable is a table of patterns that occur over time from a log source.
// The pattern table is always sorted by the frequency of the patterns. When the table
// becomes full, the least recently updated pattern is evicted.
type PatternTable2 struct {
	table          []*row2
	index          int64
	maxTableSize   int
	matchThreshold float64

	// Pattern table can be queried by the agent status command.
	// We must lock access to the table when it is being queried or updated.
	lock sync.Mutex
}

// NewPatternTable returns a new PatternTable heuristic.
func NewPatternTable2(maxTableSize int, matchThreshold float64) *PatternTable2 {
	pt := &PatternTable2{
		table:          make([]*row2, 0, maxTableSize),
		index:          0,
		maxTableSize:   maxTableSize,
		matchThreshold: matchThreshold,
		lock:           sync.Mutex{},
	}
	return pt
}

// insert adds a pattern to the table and returns the index
func (p *PatternTable2) insert(context *TokenizedMessage) int {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.index++
	foundIdx := -1
	for i, r := range p.table {
		if isMatch(r.tokenizedMessage.tokens, context.tokens, p.matchThreshold) {
			r.count++
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

	p.table = append(p.table, &row2{
		tokenizedMessage: context,
		count:            1,
		lastIndex:        p.index,
	})
	return len(p.table) - 1

}

// siftUp moves the row at the given index up the table until it is in the correct position.
func (p *PatternTable2) siftUp(idx int) int {
	for idx != 0 && p.table[idx].count > p.table[idx-1].count {
		p.table[idx], p.table[idx-1] = p.table[idx-1], p.table[idx]
		idx--
	}
	return idx
}

// evictLRU removes the least recently updated row from the table.
func (p *PatternTable2) evictLRU() {
	mini := 0
	minIndex := p.index
	for i, r := range p.table {
		if r.lastIndex < minIndex {
			minIndex = r.lastIndex
			mini = i
		}
	}
	p.table = slices.Delete(p.table, mini, mini+1)
}

// DumpTable returns a slice of DiagnosticRow structs that represent the current state of the table.
func (p *PatternTable2) DumpTable() []DiagnosticRow {
	p.lock.Lock()
	defer p.lock.Unlock()

	debug := make([]DiagnosticRow, 0, len(p.table))
	for _, r := range p.table {
		debug = append(debug, DiagnosticRow{
			TokenString: tokensToString(r.tokenizedMessage.tokens),
			Count:       r.count,
			LastIndex:   r.lastIndex,
		})
	}
	return debug
}
