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
	metrics    []metric

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

// NewCounter returns a new `Counter` using the provided namespace and common
// tags and associates it with the current metric group
func (mg *MetricGroup) NewCounter(name string, tags ...string) *Counter {
	if mg.namespace != "" {
		name = fmt.Sprintf("%s.%s", mg.namespace, name)
	}

	m := NewCounter(
		name,
		append(mg.commonTags, tags...)...,
	)

	mg.mux.Lock()
	mg.metrics = append(mg.metrics, metric(m))
	mg.mux.Unlock()

	return m
}

// NewGauge returns a new `Gauge` using the provided namespace and common
// tags and associates it with the current metric group
func (mg *MetricGroup) NewGauge(name string, tags ...string) *Gauge {
	if mg.namespace != "" {
		name = fmt.Sprintf("%s.%s", mg.namespace, name)
	}

	m := NewGauge(
		name,
		append(mg.commonTags, tags...)...,
	)

	mg.mux.Lock()
	mg.metrics = append(mg.metrics, metric(m))
	mg.mux.Unlock()

	return m
}

// Summary builds and returns a summary string all metrics beloging to the
// current `MetricGroup`.
// The string looks like:
// m1=100(50/s) m2=0(0.00/s)
// Where the values are calculated based on the deltas between calls of this method.
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
	for _, metric := range mg.metrics {
		m := metric.base()
		v := valueDeltas.ValueFor(m)
		b.WriteString(fmt.Sprintf("%s=%d", m.Name(), v))

		// If the metric is counter we also calculate the rate
		if _, ok := metric.(*Counter); ok {
			b.WriteString(fmt.Sprintf("(%.2f/s)", float64(v)/timeDelta))
		}
		b.WriteByte(' ')
	}
	mg.then = now
	return b.String()
}
