// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"slices"
	"strings"
	"sync"
)

// fakeSender implements spot.SpotSender for testing.
// It records the latest gauge value and accumulates counter values per metric+tags key.
type fakeSender struct {
	mu       sync.Mutex
	gauges   map[string]int // latest value per metricKey
	counters map[string]int // accumulated sum per metricKey
}

func newFakeSender() *fakeSender {
	return &fakeSender{
		gauges:   make(map[string]int),
		counters: make(map[string]int),
	}
}

func (f *fakeSender) Gauge(metric string, value float64, _ string, tags []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gauges[metricKey(metric, tags)] = int(value)
}

func (f *fakeSender) Count(metric string, value float64, _ string, tags []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counters[metricKey(metric, tags)] += int(value)
}

func (f *fakeSender) MonotonicCount(_ string, _ float64, _ string, _ []string) {}
func (f *fakeSender) Histogram(_ string, _ float64, _ string, _ []string)      {}
func (f *fakeSender) Commit()                                                  {}

// getGauge returns the latest gauge value recorded for the given metric and tags (any order).
func (f *fakeSender) getGauge(metric string, tags ...string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gauges[metricKey(metric, tags)]
}

// getCounter returns the accumulated counter value recorded for the given metric and tags (any order).
func (f *fakeSender) getCounter(metric string, tags ...string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counters[metricKey(metric, tags)]
}

// metricKey builds a stable lookup key independent of tag order.
func metricKey(metric string, tags []string) string {
	sorted := slices.Sorted(slices.Values(tags))
	return metric + "\x00" + strings.Join(sorted, "\x00")
}
