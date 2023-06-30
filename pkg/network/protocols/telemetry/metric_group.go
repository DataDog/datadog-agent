// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"
	"strings"
	"sync"
	"time"
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

	// used for the purposes of building the Summary() string
	deltas deltaCalculator
	then   time.Time
}

// NewMetricGroup returns a new `MetricGroup`
func NewMetricGroup(namespace string, commonTags ...string) *MetricGroup {
	return &MetricGroup{
		namespace:  namespace,
		commonTags: commonTags,
		then:       time.Now(),
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

func (mg *MetricGroup) Summary() string {
	mg.mux.Lock()
	defer mg.mux.Unlock()

	var (
		now       = time.Now()
		timeDelta = now.Sub(mg.then).Seconds()
	)

	// safeguard against division by zero
	if timeDelta == 0 {
		timeDelta = 1
	}

	valueDeltas := mg.deltas.GetState("")
	var b strings.Builder
	for _, m := range mg.metrics {
		v := valueDeltas.ValueFor(m)
		b.WriteString(fmt.Sprintf("%s=%d", m.Name(), v))

		// If the metric is counter we also calculate the rate
		if m.metricType == typeCounter {
			b.WriteString(fmt.Sprintf("(%.2f/s)", float64(v)/timeDelta))
		}
		b.WriteByte(' ')
	}
	mg.then = now
	return b.String()
}

// Clear all metrics belonging to this `MetricGroup`
func (mg *MetricGroup) Clear() {
	mg.mux.Lock()
	defer mg.mux.Unlock()

	for _, m := range mg.metrics {
		m.Set(0)
	}
}
