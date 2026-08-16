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
	selector.Add(baselineTestPath("retrans-low"), npmodel.NetworkPathConnection{Retransmits: 1})
	selector.Add(baselineTestPath("retrans-high"), npmodel.NetworkPathConnection{Retransmits: 5})
	selector.Add(baselineTestPath("timeout"), npmodel.NetworkPathConnection{TimeoutOrRTO: true})

	assert.Equal(t, []string{"timeout", "retrans-high", "retrans-low"}, selectedHosts(selector))
}

func TestBaselineSelectorFillsFromHealthyByVolume(t *testing.T) {
	selector := New()
	selector.Add(baselineTestPath("diagnostic"), npmodel.NetworkPathConnection{TimeoutOrRTO: true})
	selector.Add(baselineTestPath("small"), npmodel.NetworkPathConnection{Bytes: 60})
	selector.Add(baselineTestPath("small"), npmodel.NetworkPathConnection{Bytes: 60})
	selector.Add(baselineTestPath("large"), npmodel.NetworkPathConnection{Bytes: 100})

	assert.Equal(t, []string{"diagnostic", "small", "large"}, selectedHosts(selector))
}

func TestBaselineSelectorAggregatesAndPromotes(t *testing.T) {
	selector := New()
	path := baselineTestPath("promoted")
	selector.Add(path, npmodel.NetworkPathConnection{Bytes: math.MaxUint64})
	selector.Add(path, npmodel.NetworkPathConnection{Retransmits: 2})
	selector.Add(path, npmodel.NetworkPathConnection{Retransmits: 3})
	selector.Add(baselineTestPath("diagnostic"), npmodel.NetworkPathConnection{Retransmits: 4})
	selector.Add(baselineTestPath("healthy"), npmodel.NetworkPathConnection{Bytes: 1})

	assert.Equal(t, []string{"promoted", "diagnostic", "healthy"}, selectedHosts(selector))
}

func TestBaselineSelectorRetainsBoundedPoolsAndProtectsTimeouts(t *testing.T) {
	selector := New()
	for i := 0; i <= baselineCandidatesPerPool; i++ {
		selector.Add(baselineTestPath(fmt.Sprintf("healthy-%03d", i)), npmodel.NetworkPathConnection{Bytes: uint64(i + 1)})
	}
	for i := 0; i < baselineCandidatesPerPool; i++ {
		selector.Add(baselineTestPath(fmt.Sprintf("timeout-%03d", i)), npmodel.NetworkPathConnection{TimeoutOrRTO: true})
	}
	selector.Add(baselineTestPath("retrans-only"), npmodel.NetworkPathConnection{Retransmits: math.MaxUint64})

	assert.Len(t, selector.healthy.items, baselineCandidatesPerPool)
	assert.Len(t, selector.diagnostic.items, baselineCandidatesPerPool)
	assert.NotContains(t, selectedHosts(selector), "retrans-only")
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
			events = append(events, event{path: path, conn: npmodel.NetworkPathConnection{Retransmits: uint64(i + 1)}})
		}
	}
	for _, host := range []string{"tie-a", "tie-b", "tie-c"} {
		events = append(events, event{path: baselineTestPath(host), conn: npmodel.NetworkPathConnection{Retransmits: 1_000}})
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

func TestBaselineSelectorRecoversHeavyHittersFromLargeCardinalityStream(t *testing.T) {
	tests := map[string]func(uint64) npmodel.NetworkPathConnection{
		"healthy": func(weight uint64) npmodel.NetworkPathConnection { return npmodel.NetworkPathConnection{Bytes: weight} },
		"diagnostic": func(weight uint64) npmodel.NetworkPathConnection {
			return npmodel.NetworkPathConnection{Retransmits: weight}
		},
	}
	for name, connection := range tests {
		t.Run(name, func(t *testing.T) {
			selector := New()
			for i := 0; i < 10_000; i++ {
				selector.Add(baselineTestPath(fmt.Sprintf("background-%05d", i)), connection(1))
			}
			selector.Add(baselineTestPath("top-1"), connection(50_000))
			selector.Add(baselineTestPath("top-2"), connection(40_000))
			selector.Add(baselineTestPath("top-3"), connection(30_000))

			assert.Equal(t, []string{"top-1", "top-2", "top-3"}, selectedHosts(selector))
		})
	}
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
		if len(selector.healthy.items) != baselineCandidatesPerPool {
			b.Fatal("selector exceeded its configured bound")
		}
	}
	b.ReportMetric(baselineCandidatesPerPool, "retained_candidates")
}
