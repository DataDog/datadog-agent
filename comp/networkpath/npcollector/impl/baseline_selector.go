// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"encoding/binary"
	"sort"

	"github.com/cespare/xxhash/v2"

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
	rttVar   uint64
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

// baselinePathtestHash is deliberately local to the bounded selector. The
// selector hashes every eligible connection, so use the Agent's existing
// allocation-efficient xxhash dependency instead of the store's general hash.
func baselinePathtestHash(digest *xxhash.Digest, p common.Pathtest) uint64 {
	digest.Reset()
	writeBaselineHashString(digest, string(p.Origin))
	writeBaselineHashString(digest, p.Namespace)
	writeBaselineHashString(digest, p.Hostname)
	var port [2]byte
	binary.LittleEndian.PutUint16(port[:], p.Port)
	_, _ = digest.Write(port[:])
	writeBaselineHashString(digest, string(p.Protocol))
	writeBaselineHashString(digest, p.SourceContainerID)
	return digest.Sum64()
}

func writeBaselineHashString(digest *xxhash.Digest, value string) {
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.WriteString(value)
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

func (p *baselinePool) add(hash uint64, pathtest common.Pathtest, timeout bool, weight, rttVar uint64) (replaced, discarded bool) {
	if candidate, found := p.items[hash]; found {
		candidate.timeout = candidate.timeout || timeout
		candidate.count += weight
		candidate.rttVar = max(candidate.rttVar, rttVar)
		return false, false
	}

	if len(p.items) < p.capacity {
		p.items[hash] = &baselineCandidate{pathtest: pathtest, hash: hash, timeout: timeout, count: weight, rttVar: rttVar}
		return false, false
	}

	weakest := p.weakest()
	incoming := baselineCandidate{pathtest: pathtest, hash: hash, timeout: timeout, count: weight, rttVar: rttVar}
	if !p.policy.canReplace(weakest, &incoming) {
		return false, true
	}
	delete(p.items, weakest.hash)
	// Reuse the evicted entry. High-cardinality snapshots should not allocate a
	// candidate object for every connection that passes through a bounded pool.
	incoming.count = weakest.count + weight
	*weakest = incoming
	p.items[hash] = weakest
	return true, false
}

func diagnosticCandidateBetter(a, b *baselineCandidate) bool {
	if a.timeout != b.timeout {
		return a.timeout
	}
	if a.count != b.count {
		return a.count > b.count
	}
	if a.rttVar != b.rttVar {
		return a.rttVar > b.rttVar
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

type baselineSelector struct {
	diagnostic baselinePool
	healthy    baselinePool
	hashDigest xxhash.Digest
}

func newBaselineSelector() *baselineSelector {
	return &baselineSelector{
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

type baselineAdmission struct {
	replaced  bool
	discarded bool
}

func (s *baselineSelector) add(pathtest common.Pathtest, conn npmodel.NetworkPathConnection) baselineAdmission {
	hash := baselinePathtestHash(&s.hashDigest, pathtest)
	diagnostic := conn.TCPTimeout || conn.TCPRTO || conn.Retransmits > 0
	if diagnostic {
		s.healthy.remove(hash)
		replaced, discarded := s.diagnostic.add(hash, pathtest, conn.TCPTimeout || conn.TCPRTO, conn.Retransmits, conn.RTTVar)
		return baselineAdmission{replaced: replaced, discarded: discarded}
	}
	if _, found := s.diagnostic.items[hash]; found {
		s.diagnostic.add(hash, pathtest, false, 0, conn.RTTVar)
		return baselineAdmission{}
	}
	replaced, discarded := s.healthy.add(hash, pathtest, false, conn.Bytes, 0)
	return baselineAdmission{replaced: replaced, discarded: discarded}
}

func (s *baselineSelector) selectPathtests() []common.Pathtest {
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

func (s *baselineSelector) reset() {
	clear(s.diagnostic.items)
	clear(s.healthy.items)
}
