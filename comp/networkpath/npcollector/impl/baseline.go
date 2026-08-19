// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"math"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

const (
	baselineSelectionsPerWindow = 3
	baselineCandidateLimit      = 32
	baselineBootstrapWindow     = 5 * time.Minute
	baselineMinimumInterval     = 10 * time.Minute
)

type baselineCandidate struct {
	path  common.Pathtest
	hash  uint64
	bytes uint64
}

func (candidate baselineCandidate) betterThan(other baselineCandidate) bool {
	if candidate.bytes != other.bytes {
		return candidate.bytes > other.bytes
	}
	return candidate.hash < other.hash
}

// baselineSelector approximates the highest-volume paths in each window using
// weighted Space-Saving. Its memory is bounded independently of path count.
type baselineSelector struct {
	mu         sync.Mutex
	interval   time.Duration
	deadline   time.Time
	candidates map[uint64]baselineCandidate
}

func newBaselineSelector(interval time.Duration) *baselineSelector {
	return &baselineSelector{
		interval:   interval,
		candidates: make(map[uint64]baselineCandidate, baselineCandidateLimit),
	}
}

func (selector *baselineSelector) start(now time.Time) {
	selector.mu.Lock()
	defer selector.mu.Unlock()
	selector.startLocked(now)
}

func (selector *baselineSelector) startLocked(now time.Time) {
	if selector.deadline.IsZero() {
		selector.deadline = now.Add(baselineBootstrapWindow)
	}
}

func (selector *baselineSelector) add(path common.Pathtest, bytes uint64, now time.Time) {
	selector.mu.Lock()
	defer selector.mu.Unlock()

	selector.startLocked(now)
	if bytes == 0 {
		return
	}
	hash := path.GetHash()
	if candidate, found := selector.candidates[hash]; found {
		candidate.bytes = saturatingAdd(candidate.bytes, bytes)
		candidate.path = clonePathtest(path)
		selector.candidates[hash] = candidate
		return
	}

	candidate := baselineCandidate{path: clonePathtest(path), hash: hash, bytes: bytes}
	if len(selector.candidates) < baselineCandidateLimit {
		selector.candidates[hash] = candidate
		return
	}

	worst := selector.worstCandidateLocked()
	delete(selector.candidates, worst.hash)
	candidate.bytes = saturatingAdd(worst.bytes, bytes)
	selector.candidates[hash] = candidate
}

func (selector *baselineSelector) worstCandidateLocked() baselineCandidate {
	var worst baselineCandidate
	first := true
	for _, candidate := range selector.candidates {
		if first || worst.betterThan(candidate) {
			worst = candidate
			first = false
		}
	}
	return worst
}

// flush returns the current winners only after the active window has closed.
func (selector *baselineSelector) flush(now time.Time) []common.Pathtest {
	selector.mu.Lock()
	defer selector.mu.Unlock()

	selector.startLocked(now)
	if now.Before(selector.deadline) {
		return nil
	}

	candidates := make([]baselineCandidate, 0, len(selector.candidates))
	for _, candidate := range selector.candidates {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].betterThan(candidates[j]) })
	if len(candidates) > baselineSelectionsPerWindow {
		candidates = candidates[:baselineSelectionsPerWindow]
	}

	paths := make([]common.Pathtest, len(candidates))
	for i, candidate := range candidates {
		paths[i] = candidate.path
		paths[i].DynamicTestProfile = payload.DynamicTestProfileBaseline
		paths[i].RunOnce = true
	}

	clear(selector.candidates)
	selector.deadline = now.Add(selector.interval)
	return paths
}

func clonePathtest(path common.Pathtest) common.Pathtest {
	path.Tags = slices.Clone(path.Tags)
	return path
}

func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}
