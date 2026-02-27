// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metricsstore

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MetricsStore stores structured metrics for any cluster-agent resource
// and sends them periodically via a Datadog Sender
type MetricsStore struct {
	metrics             sync.Map // map[string]StructuredMetrics
	generateMetricsFunc func(interface{}) StructuredMetrics
	sender              sender.Sender
	isLeader            func() bool
	globalTagsFunc      func() []string
}

// NewMetricsStore creates a new metrics store.
// globalTagsFunc, if non-nil, is called on every WriteAll to retrieve tags that
// are appended to every metric (e.g. orch_cluster_id from the tagger).
func NewMetricsStore(generateFunc func(interface{}) StructuredMetrics, senderInstance sender.Sender, isLeaderFunc func() bool, globalTagsFunc func() []string) *MetricsStore {
	return &MetricsStore{
		generateMetricsFunc: generateFunc,
		sender:              senderInstance,
		isLeader:            isLeaderFunc,
		globalTagsFunc:      globalTagsFunc,
	}
}

// Add adds or updates metrics for an object.
func (m *MetricsStore) Add(key string, obj interface{}) {
	log.Tracef("Adding/updating metrics for key: %s", key)
	m.metrics.Store(key, m.generateMetricsFunc(obj))
}

// Delete removes metrics for an object.
func (m *MetricsStore) Delete(key string) {
	log.Tracef("Deleting metrics for key: %s", key)
	m.metrics.Delete(key)
}

// WriteAll sends all metrics in the store via Sender
func (m *MetricsStore) WriteAll() error {
	if !m.isLeader() {
		return nil
	}

	// Fetch global tags once per flush (e.g. orch_cluster_id from the tagger)
	var globalTags []string
	if m.globalTagsFunc != nil {
		globalTags = m.globalTagsFunc()
	}

	m.metrics.Range(func(key, value interface{}) bool {
		metrics, ok := value.(StructuredMetrics)
		if !ok {
			log.Warnf("Invalid metrics type in store for key %v", key)
			return true
		}

		log.Tracef("Submitting %d metrics for key: %v", len(metrics), key)
		for _, metric := range metrics {
			tags := metric.Tags
			if len(globalTags) > 0 {
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
		return true
	})

	m.sender.Commit()
	return nil
}

// WriteAllPeriodically runs WriteAll on a ticker at the specified interval
func (m *MetricsStore) WriteAllPeriodically(ctx context.Context, interval time.Duration) {
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
