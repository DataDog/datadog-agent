// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package ksmaggregation implements the cluster-agent side of the KSM
// node-aggregate collection: it stores per-node partials pushed by node agents
// and combines them into metric families for the cluster_aggregates_only check.
package ksmaggregation

import (
	"sync"
	"time"

	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

// AggValue is a single aggregated value with its source labels, pushed by a node agent.
type AggValue struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
}

// NodePartial is the set of pre-aggregated source metrics sent by one node agent.
// Keys are the 4 cluster-aggregate source metric names; values are per-label sums
// across all pods local to that node.
type NodePartial struct {
	// Metrics maps source metric name → label+value pairs (one per distinct label set)
	Metrics map[string][]AggValue `json:"metrics"`
}

type nodeEntry struct {
	partial   NodePartial
	updatedAt time.Time
}

// PartialStore is a thread-safe, node-keyed store of KSM partial aggregates.
type PartialStore struct {
	mu      sync.RWMutex
	entries map[string]*nodeEntry
}

var (
	globalStore     *PartialStore
	globalStoreOnce sync.Once

	emitterMu          sync.RWMutex
	emitterActiveUntil time.Time // the cluster_aggregates_only check is considered an active emitter until this time
)

// GetStore returns the singleton PartialStore for this process.
func GetStore() *PartialStore {
	globalStoreOnce.Do(func() {
		globalStore = &PartialStore{
			entries: make(map[string]*nodeEntry),
		}
	})
	return globalStore
}

// MarkEmitterRun records that the cluster_aggregates_only check just emitted the combined
// .total family, and that it should be considered an active emitter for activeWindow.
// Called once per check run on the leader; activeWindow is derived from the check's own
// collection interval (a small multiple), so a stalled emitter expires within ~one interval
// — no separate TTL config needed.
func MarkEmitterRun(activeWindow time.Duration) {
	emitterMu.Lock()
	defer emitterMu.Unlock()
	emitterActiveUntil = time.Now().Add(activeWindow)
}

// EmitterActive reports whether the cluster_aggregates_only check has emitted recently
// enough (within the window set by the last MarkEmitterRun). The endpoint only tells node
// agents to suppress their local .total when this is true, so nodes never go silent before
// an authoritative emitter exists, and resume local emission within one interval if it stalls.
func EmitterActive() bool {
	emitterMu.RLock()
	defer emitterMu.RUnlock()
	return !emitterActiveUntil.IsZero() && time.Now().Before(emitterActiveUntil)
}

// Upsert stores or replaces the partial for the given node.
func (s *PartialStore) Upsert(nodeName string, partial NodePartial) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[nodeName] = &nodeEntry{
		partial:   partial,
		updatedAt: time.Now(),
	}
}

// ReportingNodes returns the count of nodes that have pushed a partial within ttl.
func (s *PartialStore) ReportingNodes(ttl time.Duration) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cutoff := time.Now().Add(-ttl)
	n := 0
	for _, e := range s.entries {
		if e.updatedAt.After(cutoff) {
			n++
		}
	}
	return n
}

// GetCombined sums all live node partials (within ttl) into metric families
// shaped for processMetrics in cluster_aggregates_only mode. Entries older than
// ttl are pruned from the store (a node that stopped reporting — e.g. removed by
// autoscaling — would otherwise keep its partial in memory forever). This is the
// store's cleanup path; it runs once per cluster_aggregates_only check interval.
func (s *PartialStore) GetCombined(ttl time.Duration) map[string][]ksmstore.DDMetricsFam {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-ttl)

	// combined maps metric_name → (label_key → summed value)
	// label_key is a canonical string of the labels map for dedup/sum
	type labelKey = [5]string // ns, container, owner_kind, owner_name, resource
	type combinedMap = map[labelKey]*combinedEntry
	perMetric := make(map[string]combinedMap)

	for node, e := range s.entries {
		if !e.updatedAt.After(cutoff) {
			delete(s.entries, node) // prune stale node; safe to delete during range in Go
			continue
		}
		for metricName, values := range e.partial.Metrics {
			cm, ok := perMetric[metricName]
			if !ok {
				cm = make(combinedMap)
				perMetric[metricName] = cm
			}
			for _, v := range values {
				k := labelKey{
					v.Labels["namespace"],
					v.Labels["container"],
					v.Labels["owner_kind"],
					v.Labels["owner_name"],
					v.Labels["resource"],
				}
				if existing, found := cm[k]; found {
					existing.value += v.Value
				} else {
					cm[k] = &combinedEntry{labels: v.Labels, value: v.Value}
				}
			}
		}
	}

	result := make(map[string][]ksmstore.DDMetricsFam, len(perMetric))
	for metricName, cm := range perMetric {
		ddMetrics := make([]ksmstore.DDMetric, 0, len(cm))
		for _, ce := range cm {
			ddMetrics = append(ddMetrics, ksmstore.DDMetric{
				Labels: ce.labels,
				Val:    ce.value,
			})
		}
		result[metricName] = []ksmstore.DDMetricsFam{{
			Name:        metricName,
			ListMetrics: ddMetrics,
		}}
	}
	return result
}

type combinedEntry struct {
	labels map[string]string
	value  float64
}
