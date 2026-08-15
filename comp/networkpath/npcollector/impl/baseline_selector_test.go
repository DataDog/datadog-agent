// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"fmt"
	"math"
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

func selectedHosts(selector *baselineSelector) []string {
	paths := selector.selectPathtests()
	hosts := make([]string, len(paths))
	for i := range paths {
		hosts[i] = paths[i].Hostname
	}
	return hosts
}

func TestBaselineSelectorRanksDiagnosticThenHealthy(t *testing.T) {
	selector := newBaselineSelector()
	selector.add(baselineTestPath("healthy-large"), npmodel.NetworkPathConnection{Bytes: 10_000})
	selector.add(baselineTestPath("retrans-low"), npmodel.NetworkPathConnection{Retransmits: 1, RTTVar: 100})
	selector.add(baselineTestPath("retrans-high"), npmodel.NetworkPathConnection{Retransmits: 5})
	selector.add(baselineTestPath("timeout"), npmodel.NetworkPathConnection{TCPTimeout: true})

	assert.Equal(t, []string{"timeout", "retrans-high", "retrans-low"}, selectedHosts(selector))
}

func TestBaselineSelectorFillsFromHealthyByVolume(t *testing.T) {
	selector := newBaselineSelector()
	selector.add(baselineTestPath("diagnostic"), npmodel.NetworkPathConnection{TCPRTO: true})
	selector.add(baselineTestPath("small"), npmodel.NetworkPathConnection{Bytes: 10})
	selector.add(baselineTestPath("large"), npmodel.NetworkPathConnection{Bytes: 100})

	assert.Equal(t, []string{"diagnostic", "large", "small"}, selectedHosts(selector))
}

func TestBaselineSelectorAggregatesAndPromotes(t *testing.T) {
	selector := newBaselineSelector()
	path := baselineTestPath("promoted")
	selector.add(path, npmodel.NetworkPathConnection{Bytes: 100})
	selector.add(path, npmodel.NetworkPathConnection{Retransmits: 2, RTTVar: 5})
	selector.add(path, npmodel.NetworkPathConnection{Retransmits: 3, RTTVar: 10})

	hash := baselinePathtestHash(&selector.hashDigest, path)
	require.NotContains(t, selector.healthy.items, hash)
	require.Contains(t, selector.diagnostic.items, hash)
	assert.Equal(t, uint64(5), selector.diagnostic.items[hash].count)
	assert.Equal(t, uint64(10), selector.diagnostic.items[hash].rttVar)
}

func TestBaselineSelectorIsBoundedAndUsesSpaceSavingEstimate(t *testing.T) {
	selector := newBaselineSelector()
	for i := 0; i < baselineHealthyCandidates; i++ {
		selector.add(baselineTestPath(fmt.Sprintf("path-%03d", i)), npmodel.NetworkPathConnection{Bytes: uint64(i + 1)})
	}
	admission := selector.add(baselineTestPath("replacement"), npmodel.NetworkPathConnection{Bytes: 1})

	assert.True(t, admission.replaced)
	assert.Len(t, selector.healthy.items, baselineHealthyCandidates)
	replacement := selector.healthy.items[baselinePathtestHash(&selector.hashDigest, baselineTestPath("replacement"))]
	require.NotNil(t, replacement)
	assert.Greater(t, replacement.count, uint64(1), "replacement must inherit the evicted estimate")
}

func TestBaselineSelectorProtectsTimeoutCandidates(t *testing.T) {
	selector := newBaselineSelector()
	for i := 0; i < baselineDiagnosticCandidates; i++ {
		selector.add(baselineTestPath(fmt.Sprintf("timeout-%03d", i)), npmodel.NetworkPathConnection{TCPTimeout: true})
	}
	admission := selector.add(baselineTestPath("retrans-only"), npmodel.NetworkPathConnection{Retransmits: math.MaxUint64})

	assert.True(t, admission.discarded)
	assert.Len(t, selector.diagnostic.items, baselineDiagnosticCandidates)
	assert.NotContains(t, selector.diagnostic.items, baselinePathtestHash(&selector.hashDigest, baselineTestPath("retrans-only")))
}

func TestBaselineSelectorReportsSaturation(t *testing.T) {
	selector := newBaselineSelector()
	path := baselineTestPath("saturated")
	selector.add(path, npmodel.NetworkPathConnection{Bytes: math.MaxUint64})
	admission := selector.add(path, npmodel.NetworkPathConnection{Bytes: 1})
	assert.True(t, admission.saturated)
	assert.Equal(t, uint64(math.MaxUint64), selector.healthy.items[baselinePathtestHash(&selector.hashDigest, path)].count)
}

func TestBaselineSelectorDeterministicTies(t *testing.T) {
	selector := newBaselineSelector()
	paths := []common.Pathtest{baselineTestPath("c"), baselineTestPath("a"), baselineTestPath("b")}
	for _, path := range paths {
		selector.add(path, npmodel.NetworkPathConnection{Bytes: 1})
	}
	selected := selector.selectPathtests()
	for i := 1; i < len(selected); i++ {
		assert.Less(t, baselinePathtestHash(&selector.hashDigest, selected[i-1]), baselinePathtestHash(&selector.hashDigest, selected[i]))
	}
}

func TestBaselineSelectorRecoversExactTopSetFromLargeCardinalityStream(t *testing.T) {
	selector := newBaselineSelector()
	for i := 0; i < 10_000; i++ {
		selector.add(baselineTestPath(fmt.Sprintf("background-%05d", i)), npmodel.NetworkPathConnection{Bytes: 1})
	}
	for _, item := range []struct {
		host  string
		bytes uint64
	}{
		{host: "top-1", bytes: 50_000},
		{host: "top-2", bytes: 40_000},
		{host: "top-3", bytes: 30_000},
	} {
		selector.add(baselineTestPath(item.host), npmodel.NetworkPathConnection{Bytes: item.bytes})
	}

	assert.Equal(t, []string{"top-1", "top-2", "top-3"}, selectedHosts(selector),
		"the bounded approximation should recover the exact heavy hitters")
	assert.Len(t, selector.healthy.items, baselineHealthyCandidates)
}

func BenchmarkBaselineSelectorBoundedMemory(b *testing.B) {
	paths := make([]common.Pathtest, 65_536)
	for n := range paths {
		paths[n] = baselineTestPath(fmt.Sprintf("service-%d.example.com", n))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector := newBaselineSelector()
		for n, path := range paths {
			selector.add(path, npmodel.NetworkPathConnection{Bytes: uint64(n + 1)})
		}
		if len(selector.healthy.items) != baselineHealthyCandidates {
			b.Fatal("selector exceeded its configured bound")
		}
	}
	b.ReportMetric(baselineHealthyCandidates, "retained_candidates")
}
