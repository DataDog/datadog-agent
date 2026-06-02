// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksmaggregation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func cpuPartial(value float64) NodePartial {
	return NodePartial{Metrics: map[string][]AggValue{
		"kube_pod_container_resource_with_owner_tag_requests": {
			{Labels: map[string]string{
				"namespace": "kube-system", "container": "kindnet-cni",
				"owner_kind": "DaemonSet", "owner_name": "kindnet", "resource": "cpu",
			}, Value: value},
		},
	}}
}

func newStore() *PartialStore { return &PartialStore{entries: make(map[string]*nodeEntry)} }

func TestGetCombined_SumsAcrossNodes(t *testing.T) {
	s := newStore()
	s.Upsert("node-a", cpuPartial(0.1))
	s.Upsert("node-b", cpuPartial(0.1))
	s.Upsert("node-c", cpuPartial(0.1))

	combined := s.GetCombined(time.Minute)
	fam := combined["kube_pod_container_resource_with_owner_tag_requests"]
	if assert.Len(t, fam, 1) && assert.Len(t, fam[0].ListMetrics, 1) {
		assert.InDelta(t, 0.3, fam[0].ListMetrics[0].Val, 1e-9, "three nodes × 0.1 should sum to 0.3")
	}
	assert.Equal(t, 3, s.ReportingNodes(time.Minute))
}

func TestUpsert_LastValueWinsPerNode(t *testing.T) {
	s := newStore()
	s.Upsert("node-a", cpuPartial(0.1))
	s.Upsert("node-a", cpuPartial(0.5)) // retry / new value replaces, not adds
	s.Upsert("node-b", cpuPartial(0.1))

	fam := s.GetCombined(time.Minute)["kube_pod_container_resource_with_owner_tag_requests"]
	assert.InDelta(t, 0.6, fam[0].ListMetrics[0].Val, 1e-9, "node-a replaced (0.5) + node-b (0.1)")
}

func TestGetCombined_ExcludesStalePartials(t *testing.T) {
	s := newStore()
	s.Upsert("node-a", cpuPartial(0.1))
	// force node-a's entry to be stale
	s.entries["node-a"].updatedAt = time.Now().Add(-10 * time.Minute)
	s.Upsert("node-b", cpuPartial(0.1))

	fam := s.GetCombined(time.Minute)["kube_pod_container_resource_with_owner_tag_requests"]
	assert.InDelta(t, 0.1, fam[0].ListMetrics[0].Val, 1e-9, "stale node-a excluded; only node-b counts")
	assert.Equal(t, 1, s.ReportingNodes(time.Minute))
	// stale node-a must be pruned from the store, not just skipped (no memory leak on churn)
	s.mu.RLock()
	_, stillThere := s.entries["node-a"]
	s.mu.RUnlock()
	assert.False(t, stillThere, "stale node-a should be deleted from the store by GetCombined")
}

func TestGetCombined_KeepsDistinctSeriesSeparate(t *testing.T) {
	s := newStore()
	s.Upsert("node-a", NodePartial{Metrics: map[string][]AggValue{
		"kube_pod_container_resource_with_owner_tag_requests": {
			{Labels: map[string]string{"namespace": "ns1", "container": "c", "owner_kind": "Deployment", "owner_name": "a", "resource": "cpu"}, Value: 0.2},
			{Labels: map[string]string{"namespace": "ns2", "container": "c", "owner_kind": "Deployment", "owner_name": "b", "resource": "cpu"}, Value: 0.3},
		},
	}})
	fam := s.GetCombined(time.Minute)["kube_pod_container_resource_with_owner_tag_requests"]
	assert.Len(t, fam[0].ListMetrics, 2, "different owners must remain distinct series")
}

func TestEmitterHeartbeat(t *testing.T) {
	// fresh process: no emit recorded yet
	emitterMu.Lock()
	emitterActiveUntil = time.Time{}
	emitterMu.Unlock()
	assert.False(t, EmitterActive(), "no emit yet → inactive")

	MarkEmitterRun(time.Minute) // active window = 1 min
	assert.True(t, EmitterActive(), "just emitted → active within window")

	// simulate the active window having elapsed (emitter stalled)
	emitterMu.Lock()
	emitterActiveUntil = time.Now().Add(-time.Second)
	emitterMu.Unlock()
	assert.False(t, EmitterActive(), "window elapsed → inactive")
}
