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

type baselinePool struct {
	capacity int
	items    map[uint64]*baselineCandidate
}

func newBaselinePool(capacity int) baselinePool {
	return baselinePool{capacity: capacity, items: make(map[uint64]*baselineCandidate, capacity)}
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

func (p *baselinePool) weakest(diagnostic bool) *baselineCandidate {
	var weakest *baselineCandidate
	for _, candidate := range p.items {
		if weakest == nil || baselineBetter(weakest, candidate, diagnostic) {
			weakest = candidate
		}
	}
	return weakest
}

func (p *baselinePool) add(hash uint64, pathtest common.Pathtest, timeout bool, weight, rttVar uint64, diagnostic bool) (replaced, discarded, saturated bool) {
	if candidate, found := p.items[hash]; found {
		candidate.timeout = candidate.timeout || timeout
		candidate.count, saturated = npmodel.SaturatingSum(candidate.count, weight)
		candidate.rttVar = max(candidate.rttVar, rttVar)
		return false, false, saturated
	}

	if len(p.items) < p.capacity {
		p.items[hash] = &baselineCandidate{pathtest: pathtest, hash: hash, timeout: timeout, count: weight, rttVar: rttVar}
		return false, false, false
	}

	weakest := p.weakest(diagnostic)
	// Timeout/RTO is the primary diagnostic class. Do not let a non-timeout
	// candidate evict one when the diagnostic pool contains only timeouts.
	if diagnostic && weakest.timeout && !timeout {
		return false, true, false
	}
	delete(p.items, weakest.hash)
	estimate, overflow := npmodel.SaturatingSum(weakest.count, weight)
	// Reuse the evicted entry. High-cardinality snapshots should not allocate a
	// candidate object for every connection that passes through a bounded pool.
	*weakest = baselineCandidate{
		pathtest: pathtest,
		hash:     hash,
		timeout:  timeout,
		count:    estimate,
		rttVar:   rttVar,
	}
	p.items[hash] = weakest
	return true, false, overflow
}

func baselineBetter(a, b *baselineCandidate, diagnostic bool) bool {
	if diagnostic && a.timeout != b.timeout {
		return a.timeout
	}
	if a.count != b.count {
		return a.count > b.count
	}
	if diagnostic && a.rttVar != b.rttVar {
		return a.rttVar > b.rttVar
	}
	return a.hash < b.hash
}

func (p *baselinePool) sorted(diagnostic bool) []*baselineCandidate {
	result := make([]*baselineCandidate, 0, len(p.items))
	for _, candidate := range p.items {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return baselineBetter(result[i], result[j], diagnostic) })
	return result
}

type baselineSelector struct {
	diagnostic baselinePool
	healthy    baselinePool
	hashDigest xxhash.Digest
}

func newBaselineSelector() *baselineSelector {
	return &baselineSelector{
		diagnostic: newBaselinePool(baselineDiagnosticCandidates),
		healthy:    newBaselinePool(baselineHealthyCandidates),
	}
}

type baselineAdmission struct {
	replaced  bool
	discarded bool
	saturated bool
}

func (s *baselineSelector) add(pathtest common.Pathtest, conn npmodel.NetworkPathConnection) baselineAdmission {
	hash := baselinePathtestHash(&s.hashDigest, pathtest)
	diagnostic := conn.TCPTimeout || conn.TCPRTO || conn.Retransmits > 0
	if diagnostic {
		s.healthy.remove(hash)
		replaced, discarded, saturated := s.diagnostic.add(hash, pathtest, conn.TCPTimeout || conn.TCPRTO, conn.Retransmits, conn.RTTVar, true)
		return baselineAdmission{replaced: replaced, discarded: discarded, saturated: saturated || conn.NumericSaturated}
	}
	if _, found := s.diagnostic.items[hash]; found {
		_, _, saturated := s.diagnostic.add(hash, pathtest, false, 0, conn.RTTVar, true)
		return baselineAdmission{saturated: saturated || conn.NumericSaturated}
	}
	replaced, discarded, saturated := s.healthy.add(hash, pathtest, false, conn.Bytes, 0, false)
	return baselineAdmission{replaced: replaced, discarded: discarded, saturated: saturated || conn.NumericSaturated}
}

func (s *baselineSelector) selectPathtests() []common.Pathtest {
	selected := make([]common.Pathtest, 0, baselineSelectionsPerWindow)
	for _, candidate := range s.diagnostic.sorted(true) {
		selected = append(selected, candidate.pathtest)
		if len(selected) == baselineSelectionsPerWindow {
			return selected
		}
	}
	for _, candidate := range s.healthy.sorted(false) {
		selected = append(selected, candidate.pathtest)
		if len(selected) == baselineSelectionsPerWindow {
			break
		}
	}
	return selected
}

func (s *baselineSelector) reset() {
	s.diagnostic = newBaselinePool(baselineDiagnosticCandidates)
	s.healthy = newBaselinePool(baselineHealthyCandidates)
}
