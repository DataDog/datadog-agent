// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"encoding/json"
	"strings"

	"go.uber.org/atomic"
	"k8s.io/apimachinery/pkg/util/sets"
)

type metricType string

const (
	typeCounter = metricType("counter")
	typeGauge   = metricType("gauge")
)

// Metric represents a named piece of telemetry
type Metric struct {
	name       string
	metricType metricType
	tags       sets.String
	opts       sets.String
	value      *atomic.Int64
}

// NewMetric returns a new `Metric` instance
func NewMetric(name string, tagsAndOptions ...string) *Metric {
	tags, opts := splitTagsAndOptions(tagsAndOptions)
	mtype := parseMetricType(opts)

	m := &Metric{
		name:       name,
		metricType: mtype,
		value:      atomic.NewInt64(0),
		tags:       tags,
		opts:       opts,
	}

	globalRegistry.Lock()
	defer globalRegistry.Unlock()
	// Ensure we only have one intance per (name, tags). If there is an existing
	// `Metric` instance matching the params we simply return it. For now we're
	// doing a brute-force search here because calls to `NewMetric` are almost
	// always restriced to program initialization
	for _, other := range globalRegistry.metrics {
		if other.isEqual(m) {
			return other
		}
	}

	globalRegistry.metrics = append(globalRegistry.metrics, m)
	return m
}

// Name of the `Metric` (including tags)
func (m *Metric) Name() string {
	return strings.Join(append([]string{m.name}, m.tags.List()...), ",")
}

// Set value atomically
func (m *Metric) Set(v int64) {
	if m.metricType != typeGauge {
		return
	}

	m.value.Store(v)
}

// Add value atomically
func (m *Metric) Add(v int64) {
	if v < 0 {
		return
	}

	m.value.Add(v)
}

// Get value atomically
func (m *Metric) Get() int64 {
	return m.value.Load()
}

// MarshalJSON returns a json representation of the current `Metric`. We
// implement our own method so we don't need to export the fields.
// This is mostly inteded for serving a list of the existing
// metrics under /network_tracer/debug/telemetry endpoint
func (m *Metric) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name string
		Tags []string `json:",omitempty"`
		Opts []string
	}{
		Name: m.name,
		Tags: m.tags.List(),
		Opts: m.opts.List(),
	})
}

func (m *Metric) isEqual(other *Metric) bool {
	return m.name == other.name && m.tags.Equal(other.tags)
}

func parseMetricType(opts sets.String) metricType {
	defer func() {
		// remove type parameters from the options set
		opts.Delete(OptGauge)
		opts.Delete(OptCounter)
	}()

	if opts.Has(OptGauge) {
		return typeGauge
	}

	return typeCounter
}
