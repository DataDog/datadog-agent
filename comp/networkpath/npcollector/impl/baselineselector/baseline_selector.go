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
	baselineDiagnosticCandidates = 128
	baselineHealthyCandidates    = 128
	baselineSelectionsPerWindow  = 3
)

type baselineCandidate struct {
	pathtest common.Pathtest
	hash     uint64
	timeout  bool
	count    uint64
}

type baselinePoolPolicy struct {
	better     func(a, b *baselineCandidate) bool
	canReplace func(weakest, incoming *baselineCandidate) bool
}

type baselinePool struct {
	capacity int
	items    map[uint64]*baselineCandidate
	policy   baselinePoolPolicy
}

func newBaselinePool(capacity int, policy baselinePoolPolicy) baselinePool {
	return baselinePool{
		capacity: capacity,
		items:    make(map[uint64]*baselineCandidate, capacity),
		policy:   policy,
	}
}

func (p *baselinePool) remove(hash uint64) { delete(p.items, hash) }

func (p *baselinePool) weakest() *baselineCandidate {
	var weakest *baselineCandidate
	for _, candidate := range p.items {
		if weakest == nil || p.policy.better(weakest, candidate) {
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

	if len(p.items) < p.capacity {
		p.items[hash] = &baselineCandidate{pathtest: pathtest, hash: hash, timeout: timeout, count: weight}
		return
	}

	weakest := p.weakest()
	incoming := baselineCandidate{pathtest: pathtest, hash: hash, timeout: timeout, count: weight}
	if !p.policy.canReplace(weakest, &incoming) {
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

func alwaysReplace(_, _ *baselineCandidate) bool { return true }

// A retransmit-only candidate cannot displace a timeout/RTO candidate. All
// other replacement decisions use Space-Saving estimates and the pool rank.
func canReplaceDiagnostic(weakest, incoming *baselineCandidate) bool {
	return !weakest.timeout || incoming.timeout
}

func (p *baselinePool) sorted() []*baselineCandidate {
	result := make([]*baselineCandidate, 0, len(p.items))
	for _, candidate := range p.items {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return p.policy.better(result[i], result[j]) })
	return result
}

// Selector retains and ranks a bounded set of baseline path candidates.
type Selector struct {
	diagnostic baselinePool
	healthy    baselinePool
}

// New creates a baseline selector.
func New() *Selector {
	return &Selector{
		diagnostic: newBaselinePool(baselineDiagnosticCandidates, baselinePoolPolicy{
			better:     diagnosticCandidateBetter,
			canReplace: canReplaceDiagnostic,
		}),
		healthy: newBaselinePool(baselineHealthyCandidates, baselinePoolPolicy{
			better:     healthyCandidateBetter,
			canReplace: alwaysReplace,
		}),
	}
}

// Add records a network path observation.
func (s *Selector) Add(pathtest common.Pathtest, conn npmodel.NetworkPathConnection) {
	hash := pathtest.GetHash()
	diagnostic := conn.TimeoutOrRTO || conn.Retransmits > 0
	if diagnostic {
		s.healthy.remove(hash)
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
	selected := make([]common.Pathtest, 0, baselineSelectionsPerWindow)
	for _, candidate := range s.diagnostic.sorted() {
		selected = append(selected, candidate.pathtest)
		if len(selected) == baselineSelectionsPerWindow {
			return selected
		}
	}
	for _, candidate := range s.healthy.sorted() {
		selected = append(selected, candidate.pathtest)
		if len(selected) == baselineSelectionsPerWindow {
			break
		}
	}
	return selected
}

// Reset removes all retained candidates.
func (s *Selector) Reset() {
	clear(s.diagnostic.items)
	clear(s.healthy.items)
}
