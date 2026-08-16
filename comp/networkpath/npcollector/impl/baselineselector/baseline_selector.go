// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package baselineselector selects representative network paths from connection observations.
package baselineselector

import (
	"sort"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
)

const (
	baselineCandidatesPerPool     = 128
	baselineSelectionsPerSnapshot = 3
)

type baselineCandidate struct {
	pathtest common.Pathtest
	hash     uint64
	timeout  bool
	count    uint64
}

type baselinePool struct {
	items           map[uint64]*baselineCandidate
	better          func(a, b *baselineCandidate) bool
	protectTimeouts bool
}

func newBaselinePool(better func(a, b *baselineCandidate) bool, protectTimeouts bool) baselinePool {
	return baselinePool{
		items:           make(map[uint64]*baselineCandidate, baselineCandidatesPerPool),
		better:          better,
		protectTimeouts: protectTimeouts,
	}
}

func (p *baselinePool) weakest() *baselineCandidate {
	var weakest *baselineCandidate
	for _, candidate := range p.items {
		if weakest == nil || p.better(weakest, candidate) {
			weakest = candidate
		}
	}
	return weakest
}

func (p *baselinePool) add(hash uint64, pathtest common.Pathtest, timeout bool, weight uint64) {
	if candidate, found := p.items[hash]; found {
		candidate.timeout = candidate.timeout || timeout
		candidate.count += weight
		return
	}

	if len(p.items) < baselineCandidatesPerPool {
		p.items[hash] = &baselineCandidate{pathtest: pathtest, hash: hash, timeout: timeout, count: weight}
		return
	}

	weakest := p.weakest()
	incoming := baselineCandidate{pathtest: pathtest, hash: hash, timeout: timeout, count: weight}
	if p.protectTimeouts && weakest.timeout && !incoming.timeout {
		return
	}
	delete(p.items, weakest.hash)
	// Reuse the evicted entry. High-cardinality snapshots should not allocate a
	// candidate object for every connection that passes through a bounded pool.
	incoming.count = weakest.count + weight
	*weakest = incoming
	p.items[hash] = weakest
}

func diagnosticCandidateBetter(a, b *baselineCandidate) bool {
	if a.timeout != b.timeout {
		return a.timeout
	}
	if a.count != b.count {
		return a.count > b.count
	}
	return a.hash < b.hash
}

func healthyCandidateBetter(a, b *baselineCandidate) bool {
	if a.count != b.count {
		return a.count > b.count
	}
	return a.hash < b.hash
}

func (p *baselinePool) sorted() []*baselineCandidate {
	result := make([]*baselineCandidate, 0, len(p.items))
	for _, candidate := range p.items {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return p.better(result[i], result[j]) })
	return result
}

// Selector retains and ranks a bounded set of baseline path candidates from one connection snapshot.
type Selector struct {
	diagnostic baselinePool
	healthy    baselinePool
}

// New creates a baseline selector.
func New() *Selector {
	return &Selector{
		diagnostic: newBaselinePool(diagnosticCandidateBetter, true),
		healthy:    newBaselinePool(healthyCandidateBetter, false),
	}
}

// Add records a network path observation.
func (s *Selector) Add(pathtest common.Pathtest, conn npmodel.NetworkPathConnection) {
	hash := pathtest.GetHash()
	diagnostic := conn.TimeoutOrRTO || conn.Retransmits > 0
	if diagnostic {
		delete(s.healthy.items, hash)
		s.diagnostic.add(hash, pathtest, conn.TimeoutOrRTO, conn.Retransmits)
		return
	}
	if _, found := s.diagnostic.items[hash]; found {
		return
	}
	s.healthy.add(hash, pathtest, false, conn.Bytes)
}

// Select returns the highest-ranked baseline paths.
func (s *Selector) Select() []common.Pathtest {
	selected := make([]common.Pathtest, 0, baselineSelectionsPerSnapshot)
	for _, candidate := range s.diagnostic.sorted() {
		selected = append(selected, candidate.pathtest)
		if len(selected) == baselineSelectionsPerSnapshot {
			return selected
		}
	}
	for _, candidate := range s.healthy.sorted() {
		selected = append(selected, candidate.pathtest)
		if len(selected) == baselineSelectionsPerSnapshot {
			break
		}
	}
	return selected
}
