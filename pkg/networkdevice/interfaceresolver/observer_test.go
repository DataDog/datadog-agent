// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interfaceresolver

import (
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeObserver captures every event for assertion in tests. Thread-
// safe — the resolver is documented as safe to share across goroutines
// and so is its Observer.
type fakeObserver struct {
	mu          sync.Mutex
	resolutions []resolutionEvent
	typeDist    []typeDistEvent
}

type resolutionEvent struct {
	Vendor string
	Result Result
}

type typeDistEvent struct {
	Vendor string
	IfType IfType
	Count  int
}

func (f *fakeObserver) ObserveResolution(vendor string, res Result) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolutions = append(f.resolutions, resolutionEvent{vendor, res})
}

func (f *fakeObserver) ObserveIfTypeDistribution(vendor string, t IfType, count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typeDist = append(f.typeDist, typeDistEvent{vendor, t, count})
}

func TestCandidateCountBucket(t *testing.T) {
	cases := map[int]string{
		-1: "0", 0: "0",
		1:  "1",
		2:  "2",
		3:  "3-5", 4: "3-5", 5: "3-5",
		6: "6-10", 10: "6-10",
		11: "11+", 1000: "11+",
	}
	for n, want := range cases {
		assert.Equal(t, want, CandidateCountBucket(n))
	}
}

func TestObserver_ResolutionsAreReported(t *testing.T) {
	obs := &fakeObserver{}
	r := NewResolver(vlanColocatedCandidates(),
		WithVendor("cisco"),
		WithObserver(obs),
	)

	r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{ExpectedType: IfTypeEthernetCsmacd})
	r.Resolve("00:00:00:00:00:00", ContextHint{}) // unmatched
	r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{}) // unresolved ambiguous

	assert.Len(t, obs.resolutions, 3)
	assert.Equal(t, "cisco", obs.resolutions[0].Vendor)
	assert.Equal(t, OutcomeMatchedAmbiguous, obs.resolutions[0].Result.Outcome)
	assert.Equal(t, RulePreferPhysical, obs.resolutions[0].Result.TiebreakerRule)
	assert.Equal(t, OutcomeUnmatched, obs.resolutions[1].Result.Outcome)
	assert.Equal(t, OutcomeUnresolvedAmbiguous, obs.resolutions[2].Result.Outcome)
}

func TestObserver_IfTypeDistributionAtConstruction(t *testing.T) {
	obs := &fakeObserver{}
	_ = NewResolver(vlanColocatedCandidates(),
		WithVendor("cisco"),
		WithObserver(obs),
	)

	// 1 ethernetCsmacd + 2 l2vlan candidates → two histogram buckets.
	assert.Len(t, obs.typeDist, 2)
	// Sort by IfType for stable assertion.
	sort.Slice(obs.typeDist, func(i, j int) bool {
		return obs.typeDist[i].IfType < obs.typeDist[j].IfType
	})
	assert.Equal(t, IfTypeEthernetCsmacd, obs.typeDist[0].IfType)
	assert.Equal(t, 1, obs.typeDist[0].Count)
	assert.Equal(t, IfTypeL2Vlan, obs.typeDist[1].IfType)
	assert.Equal(t, 2, obs.typeDist[1].Count)
	assert.Equal(t, "cisco", obs.typeDist[0].Vendor)
}

func TestObserver_DefaultsToNoop(t *testing.T) {
	// No panic when no observer is supplied.
	r := NewResolver(vlanColocatedCandidates(), WithVendor("cisco"))
	res := r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{})
	assert.Equal(t, OutcomeUnresolvedAmbiguous, res.Outcome)
}

func TestObserver_NilOptionIsAccepted(t *testing.T) {
	// WithObserver(nil) must not blow up at Resolve time.
	r := NewResolver(vlanColocatedCandidates(), WithObserver(nil))
	res := r.Resolve("aa:bb:cc:dd:ee:01", ContextHint{ExpectedType: IfTypeEthernetCsmacd})
	assert.Equal(t, OutcomeMatchedAmbiguous, res.Outcome)
}
