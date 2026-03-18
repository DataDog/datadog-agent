// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metricsstore

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// keyEntry holds the metrics for a single store key, protected by a RWMutex.
// It stores both generated metrics (from Add) and pushed gauges (from PushGauge)
// in the same map, keyed by metric identity.
// keyTags holds per-key tags extracted from the object at Add time (e.g. from
// the ad.datadoghq.com/tags annotation); they are appended to every metric for
// this key when sending.
type keyEntry struct {
	mu      sync.RWMutex
	metrics map[string]StructuredMetric // identity → metric
	keyTags []string
}

func newKeyEntry() *keyEntry {
	return &keyEntry{
		metrics: make(map[string]StructuredMetric),
	}
}

// metricIdentity returns a stable string key for a metric based on its name
// and sorted tags, used as the upsert key in the pushed map.
func metricIdentity(name string, tags []string) string {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)

	size := len(name) + 1 // name + '\x00'
	for _, t := range sorted {
		size += len(t) + 1 // tag + ','
	}
	if len(sorted) > 0 {
		size-- // no trailing comma
	}

	var b strings.Builder
	b.Grow(size)
	b.WriteString(name)
	b.WriteByte('\x00')
	for i, t := range sorted {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(t)
	}
	return b.String()
}

// MetricsStore stores structured metrics for any cluster-agent resource
// and sends them periodically via a Datadog Sender
type MetricsStore[T any] struct {
	metrics             sync.Map // map[string]*keyEntry
	generateMetricsFunc func(T) StructuredMetrics
	keyTagsFunc         func(T) []string
	sender              sender.Sender
	isLeader            func() bool
	globalTagsFunc      func() []string
}

// NewMetricsStore creates a new metrics store.
// keyTagsFunc, if non-nil, is called on every Add to extract per-key tags from
// the object (e.g. from the ad.datadoghq.com/tags annotation). Those tags are
// stored in the keyEntry and appended to every metric for that key at send time.
// globalTagsFunc, if non-nil, is called on every WriteAll to retrieve tags that
// are appended to every metric (e.g. orch_cluster_id from the tagger).
func NewMetricsStore[T any](generateFunc func(T) StructuredMetrics, keyTagsFunc func(T) []string, senderInstance sender.Sender, isLeaderFunc func() bool, globalTagsFunc func() []string) *MetricsStore[T] {
	return &MetricsStore[T]{
		generateMetricsFunc: generateFunc,
		keyTagsFunc:         keyTagsFunc,
		sender:              senderInstance,
		isLeader:            isLeaderFunc,
		globalTagsFunc:      globalTagsFunc,
	}
}

// Add adds or updates the generated metrics for an object.
// It fully replaces all metrics for the key, including any previously pushed gauges.
// If a keyTagsFunc was provided at construction, it is called to extract per-key
// tags from obj (e.g. from the ad.datadoghq.com/tags annotation), which are
// stored on the keyEntry and appended to every metric for this key at send time.
func (m *MetricsStore[T]) Add(key string, obj T) {
	log.Tracef("Adding/updating metrics for key: %s", key)
	generated := m.generateMetricsFunc(obj)

	actual, _ := m.metrics.LoadOrStore(key, newKeyEntry())
	e := actual.(*keyEntry)

	e.mu.Lock()
	defer e.mu.Unlock()

	newMetrics := make(map[string]StructuredMetric, len(generated))
	for _, metric := range generated {
		newMetrics[metricIdentity(metric.Name, metric.Tags)] = metric
	}
	e.metrics = newMetrics

	if m.keyTagsFunc != nil {
		e.keyTags = m.keyTagsFunc(obj)
	}
}

// PushGauge adds or updates a gauge metric for a specific key.
// Multiple gauges with the same metric name but different tags are stored as separate entries.
// Calling PushGauge again with the same name and tags updates the existing value.
func (m *MetricsStore[T]) PushGauge(key, metricName string, value float64, tags []string) {
	actual, _ := m.metrics.LoadOrStore(key, newKeyEntry())
	e := actual.(*keyEntry)

	e.mu.Lock()
	defer e.mu.Unlock()

	e.metrics[metricIdentity(metricName, tags)] = StructuredMetric{
		Name:  metricName,
		Type:  MetricTypeGauge,
		Value: value,
		Tags:  slices.Clone(tags),
	}
}

// Delete removes all metrics for a key, including both generated and pushed gauges.
func (m *MetricsStore[T]) Delete(key string) {
	log.Tracef("Deleting metrics for key: %s", key)
	m.metrics.Delete(key)
}

// WriteAll sends all metrics in the store via Sender
func (m *MetricsStore[T]) WriteAll() error {
	if !m.isLeader() {
		return nil
	}

	// Fetch global tags once per flush (e.g. orch_cluster_id from the tagger)
	var globalTags []string
	if m.globalTagsFunc != nil {
		globalTags = m.globalTagsFunc()
	}

	sendMetric := func(metric StructuredMetric, keyTags []string) {
		tags := metric.Tags
		if len(keyTags) > 0 || len(globalTags) > 0 {
			tags = tags[:len(tags):len(tags)]
			tags = append(tags, keyTags...)
			tags = append(tags, globalTags...)
		}
		switch metric.Type {
		case MetricTypeGauge:
			m.sender.Gauge(metric.Name, metric.Value, "", tags)
		case MetricTypeMonotonicCount:
			m.sender.MonotonicCount(metric.Name, metric.Value, "", tags)
		case MetricTypeCount:
			m.sender.Count(metric.Name, metric.Value, "", tags)
		default:
			log.Warnf("Unknown metric type %v for metric %s", metric.Type, metric.Name)
		}
	}

	m.metrics.Range(func(key, value interface{}) bool {
		e := value.(*keyEntry)

		e.mu.RLock()
		defer e.mu.RUnlock()

		log.Tracef("Submitting %d metrics for key: %v", len(e.metrics), key)
		for _, metric := range e.metrics {
			sendMetric(metric, e.keyTags)
		}
		return true
	})

	m.sender.Commit()
	return nil
}

// WriteAllPeriodically runs WriteAll on a ticker at the specified interval
func (m *MetricsStore[T]) WriteAllPeriodically(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Debugf("Starting periodic metrics writer (interval: %s)", interval)
	// Submit immediately on start
	if err := m.WriteAll(); err != nil {
		log.Errorf("Failed to write metrics: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping periodic metrics writer")
			return
		case <-ticker.C:
			if err := m.WriteAll(); err != nil {
				log.Errorf("Failed to write metrics: %v", err)
			}
		}
	}
}
