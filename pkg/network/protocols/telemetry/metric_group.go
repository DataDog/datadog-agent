// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"
	"strings"
	"sync"
)

// MetricGroup provides a convenient constructor for a group with metrics
// sharing the same namespace and group of tags.
// Note the usage of this API is entirely optional; I'm only adding this here
// to keep compatibility with some common patterns I've seen in the codebase.
type MetricGroup struct {
	mux        sync.Mutex
	namespace  string
	commonTags []string
	metrics    []*Metric
}

// NewMetricGroup returns a new `MetricGroup`
func NewMetricGroup(namespace string, commonTags ...string) *MetricGroup {
	return &MetricGroup{
		namespace:  namespace,
		commonTags: commonTags,
	}
}

// NewMetric returns a new `Metric` using the provided namespace and common tags
func (mg *MetricGroup) NewMetric(name string, tags ...string) *Metric {
	if mg.namespace != "" {
		name = fmt.Sprintf("%s.%s", mg.namespace, name)
	}

	m := NewMetric(
		name,
		append(mg.commonTags, tags...)...,
	)

	mg.mux.Lock()
	mg.metrics = append(mg.metrics, m)
	mg.mux.Unlock()

	return m
}

// Summary returns a map[string]int64 representing
// a summary of all metrics belonging to this MetricGroup
func (mg *MetricGroup) Summary() map[string]int64 {
	mg.mux.Lock()
	defer mg.mux.Unlock()

	prefix := fmt.Sprintf("%s.", mg.namespace)
	summary := make(map[string]int64, len(mg.metrics))
	for _, m := range mg.metrics {
		nameWithoutNS := strings.TrimPrefix(m.Name(), prefix)
		summary[nameWithoutNS] = m.Get()
	}

	return summary
}

// Clear all metrics belonging to this `MetricGroup`
func (mg *MetricGroup) Clear() {
	mg.mux.Lock()
	defer mg.mux.Unlock()

	for _, m := range mg.metrics {
		m.Set(0)
	}
}
