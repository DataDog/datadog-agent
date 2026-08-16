// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package baselineselector

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func baselineTestPath(name string) common.Pathtest {
	return common.Pathtest{Hostname: name, Protocol: payload.ProtocolUDP, Origin: payload.PathOriginNetworkTraffic}
}

func selectedHosts(selector *Selector) []string {
	paths := selector.Select()
	hosts := make([]string, len(paths))
	for i := range paths {
		hosts[i] = paths[i].Hostname
	}
	return hosts
}

func TestBaselineSelectorRanksDiagnosticThenHealthy(t *testing.T) {
	selector := New()
	selector.Add(baselineTestPath("healthy-large"), npmodel.NetworkPathConnection{Bytes: 10_000})
	selector.Add(baselineTestPath("retrans-low"), npmodel.NetworkPathConnection{Retransmits: 1, RTTVar: 100})
	selector.Add(baselineTestPath("retrans-high"), npmodel.NetworkPathConnection{Retransmits: 5})
	selector.Add(baselineTestPath("timeout"), npmodel.NetworkPathConnection{TCPTimeout: true})

	assert.Equal(t, []string{"timeout", "retrans-high", "retrans-low"}, selectedHosts(selector))
}

func TestBaselineCandidateRankingPolicies(t *testing.T) {
	tests := []struct {
		name   string
		better func(a, b *baselineCandidate) bool
		a      baselineCandidate
		b      baselineCandidate
	}{
		{name: "diagnostic timeout before retransmits", better: diagnosticCandidateBetter, a: baselineCandidate{timeout: true}, b: baselineCandidate{count: math.MaxUint64}},
		{name: "diagnostic retransmits before RTT variance", better: diagnosticCandidateBetter, a: baselineCandidate{count: 2}, b: baselineCandidate{count: 1, rttVar: math.MaxUint64}},
		{name: "diagnostic RTT variance before hash", better: diagnosticCandidateBetter, a: baselineCandidate{count: 1, rttVar: 2, hash: 2}, b: baselineCandidate{count: 1, rttVar: 1, hash: 1}},
		{name: "diagnostic hash tie breaker", better: diagnosticCandidateBetter, a: baselineCandidate{count: 1, rttVar: 1, hash: 1}, b: baselineCandidate{count: 1, rttVar: 1, hash: 2}},
		{name: "healthy bytes before hash", better: healthyCandidateBetter, a: baselineCandidate{count: 2, hash: 2}, b: baselineCandidate{count: 1, hash: 1}},
		{name: "healthy hash tie breaker", better: healthyCandidateBetter, a: baselineCandidate{count: 1, hash: 1}, b: baselineCandidate{count: 1, hash: 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, tt.better(&tt.a, &tt.b))
			assert.False(t, tt.better(&tt.b, &tt.a))
		})
	}
}

func TestBaselineSelectorTreatsTimeoutAndRTOEqually(t *testing.T) {
	selector := New()
	selector.Add(baselineTestPath("timeout"), npmodel.NetworkPathConnection{TCPTimeout: true})
	selector.Add(baselineTestPath("rto"), npmodel.NetworkPathConnection{TCPRTO: true})
	selector.Add(baselineTestPath("retrans"), npmodel.NetworkPathConnection{Retransmits: math.MaxUint64})

	selected := selector.Select()
	require.Len(t, selected, 3)
	timeoutHash := baselineTestPath("timeout").GetHash()
	rtoHash := baselineTestPath("rto").GetHash()
	if timeoutHash < rtoHash {
		assert.Equal(t, []string{"timeout", "rto", "retrans"}, selectedHosts(selector))
	} else {
		assert.Equal(t, []string{"rto", "timeout", "retrans"}, selectedHosts(selector))
	}
}

func TestBaselineSelectorFillsFromHealthyByVolume(t *testing.T) {
	selector := New()
	selector.Add(baselineTestPath("diagnostic"), npmodel.NetworkPathConnection{TCPRTO: true})
	selector.Add(baselineTestPath("small"), npmodel.NetworkPathConnection{Bytes: 10})
	selector.Add(baselineTestPath("large"), npmodel.NetworkPathConnection{Bytes: 100})

	assert.Equal(t, []string{"diagnostic", "large", "small"}, selectedHosts(selector))
}

func TestBaselineSelectorAggregatesAndPromotes(t *testing.T) {
	selector := New()
	path := baselineTestPath("promoted")
	selector.Add(path, npmodel.NetworkPathConnection{Bytes: 100})
	selector.Add(path, npmodel.NetworkPathConnection{Retransmits: 2, RTTVar: 5})
	selector.Add(path, npmodel.NetworkPathConnection{Retransmits: 3, RTTVar: 10})

	hash := path.GetHash()
	require.NotContains(t, selector.healthy.items, hash)
	require.Contains(t, selector.diagnostic.items, hash)
	assert.Equal(t, uint64(5), selector.diagnostic.items[hash].count)
	assert.Equal(t, uint64(10), selector.diagnostic.items[hash].rttVar)
}

func TestBaselineSelectorAggregatesEverySignal(t *testing.T) {
	selector := New()
	diagnosticPath := baselineTestPath("diagnostic")
	healthyPath := baselineTestPath("healthy")

	selector.Add(diagnosticPath, npmodel.NetworkPathConnection{Retransmits: 2, RTTVar: 10})
	selector.Add(diagnosticPath, npmodel.NetworkPathConnection{TCPRTO: true, Retransmits: 3, RTTVar: 5})
	selector.Add(diagnosticPath, npmodel.NetworkPathConnection{RTTVar: 20})
	selector.Add(healthyPath, npmodel.NetworkPathConnection{Bytes: 40})
	selector.Add(healthyPath, npmodel.NetworkPathConnection{Bytes: 2})

	diagnosticHash := diagnosticPath.GetHash()
	diagnostic := selector.diagnostic.items[diagnosticHash]
	require.NotNil(t, diagnostic)
	assert.True(t, diagnostic.timeout)
	assert.Equal(t, uint64(5), diagnostic.count)
	assert.Equal(t, uint64(20), diagnostic.rttVar)

	healthyHash := healthyPath.GetHash()
	healthy := selector.healthy.items[healthyHash]
	require.NotNil(t, healthy)
	assert.Equal(t, uint64(42), healthy.count)
}

func TestBaselineSelectorIsBoundedAndUsesSpaceSavingEstimate(t *testing.T) {
	selector := New()
	for i := 0; i < baselineHealthyCandidates; i++ {
		selector.Add(baselineTestPath(fmt.Sprintf("path-%03d", i)), npmodel.NetworkPathConnection{Bytes: uint64(i + 1)})
	}
	selector.Add(baselineTestPath("replacement"), npmodel.NetworkPathConnection{Bytes: 1})

	assert.Len(t, selector.healthy.items, baselineHealthyCandidates)
	replacement := selector.healthy.items[baselineTestPath("replacement").GetHash()]
	require.NotNil(t, replacement)
	assert.Greater(t, replacement.count, uint64(1), "replacement must inherit the evicted estimate")
}

func TestBaselineSelectorProtectsTimeoutCandidates(t *testing.T) {
	selector := New()
	for i := 0; i < baselineDiagnosticCandidates; i++ {
		selector.Add(baselineTestPath(fmt.Sprintf("timeout-%03d", i)), npmodel.NetworkPathConnection{TCPTimeout: true})
	}
	selector.Add(baselineTestPath("retrans-only"), npmodel.NetworkPathConnection{Retransmits: math.MaxUint64})

	assert.Len(t, selector.diagnostic.items, baselineDiagnosticCandidates)
	assert.NotContains(t, selector.diagnostic.items, baselineTestPath("retrans-only").GetHash())
}

func TestBaselineSelectorDeterministicTies(t *testing.T) {
	selector := New()
	paths := []common.Pathtest{baselineTestPath("c"), baselineTestPath("a"), baselineTestPath("b")}
	for _, path := range paths {
		selector.Add(path, npmodel.NetworkPathConnection{Bytes: 1})
	}
	selected := selector.Select()
	for i := 1; i < len(selected); i++ {
		assert.Less(t, selected[i-1].GetHash(), selected[i].GetHash())
	}
}

func TestBaselineSelectorIsDeterministicBelowCapacity(t *testing.T) {
	type event struct {
		path common.Pathtest
		conn npmodel.NetworkPathConnection
	}
	events := make([]event, 0, 96)
	for i := 0; i < 32; i++ {
		path := baselineTestPath(fmt.Sprintf("path-%02d", i))
		events = append(events,
			event{path: path, conn: npmodel.NetworkPathConnection{Bytes: uint64(i + 1)}},
			event{path: path, conn: npmodel.NetworkPathConnection{Bytes: uint64(2 * i)}},
		)
		if i%3 == 0 {
			events = append(events, event{path: path, conn: npmodel.NetworkPathConnection{Retransmits: uint64(i + 1), RTTVar: uint64(100 - i)}})
		}
	}

	baseline := New()
	for _, event := range events {
		baseline.Add(event.path, event.conn)
	}
	want := selectedHosts(baseline)

	for seed := int64(0); seed < 20; seed++ {
		shuffled := append([]event(nil), events...)
		rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		selector := New()
		for _, event := range shuffled {
			selector.Add(event.path, event.conn)
		}
		assert.Equalf(t, want, selectedHosts(selector), "seed %d", seed)
	}
}

func TestBaselineSelectorRecoversExactTopSetFromLargeCardinalityStream(t *testing.T) {
	selector := New()
	for i := 0; i < 10_000; i++ {
		selector.Add(baselineTestPath(fmt.Sprintf("background-%05d", i)), npmodel.NetworkPathConnection{Bytes: 1})
	}
	for _, item := range []struct {
		host  string
		bytes uint64
	}{
		{host: "top-1", bytes: 50_000},
		{host: "top-2", bytes: 40_000},
		{host: "top-3", bytes: 30_000},
	} {
		selector.Add(baselineTestPath(item.host), npmodel.NetworkPathConnection{Bytes: item.bytes})
	}

	assert.Equal(t, []string{"top-1", "top-2", "top-3"}, selectedHosts(selector),
		"the bounded approximation should recover the exact heavy hitters")
	assert.Len(t, selector.healthy.items, baselineHealthyCandidates)
}

func TestBaselineSelectorRecoversLateDiagnosticHeavyHitters(t *testing.T) {
	selector := New()
	for i := 0; i < 10_000; i++ {
		selector.Add(baselineTestPath(fmt.Sprintf("background-%05d", i)), npmodel.NetworkPathConnection{Retransmits: 1})
	}
	for _, item := range []struct {
		host        string
		retransmits uint64
	}{
		{host: "top-1", retransmits: 50_000},
		{host: "top-2", retransmits: 40_000},
		{host: "top-3", retransmits: 30_000},
	} {
		selector.Add(baselineTestPath(item.host), npmodel.NetworkPathConnection{Retransmits: item.retransmits})
	}

	assert.Equal(t, []string{"top-1", "top-2", "top-3"}, selectedHosts(selector))
	assert.Len(t, selector.diagnostic.items, baselineDiagnosticCandidates)
}

func TestBaselineSelectorResetRetainsPolicies(t *testing.T) {
	selector := New()
	selector.Add(baselineTestPath("before"), npmodel.NetworkPathConnection{Bytes: 1})
	selector.Reset()

	assert.Empty(t, selector.diagnostic.items)
	assert.Empty(t, selector.healthy.items)
	selector.Add(baselineTestPath("healthy"), npmodel.NetworkPathConnection{Bytes: 10})
	selector.Add(baselineTestPath("diagnostic"), npmodel.NetworkPathConnection{Retransmits: 1})
	assert.Equal(t, []string{"diagnostic", "healthy"}, selectedHosts(selector))
}

func BenchmarkBaselineSelectorBoundedMemory(b *testing.B) {
	paths := make([]common.Pathtest, 65_536)
	for n := range paths {
		paths[n] = baselineTestPath(fmt.Sprintf("service-%d.example.com", n))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector := New()
		for n, path := range paths {
			selector.Add(path, npmodel.NetworkPathConnection{Bytes: uint64(n + 1)})
		}
		if len(selector.healthy.items) != baselineHealthyCandidates {
			b.Fatal("selector exceeded its configured bound")
		}
	}
	b.ReportMetric(baselineHealthyCandidates, "retained_candidates")
}
