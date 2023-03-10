// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

// Metric represents a named piece of telemetry
type Metric struct {
	name  string
	tags  []string
	opts  []string
	value *atomic.Int64

	// metrics of type OptMonotonic use this value
	// when Delta() is called
	prevValue *atomic.Int64
}

// NewMetric returns a new `Metric` instance
func NewMetric(name string, tagsAndOptions ...string) *Metric {
	tags, opts := splitTagsAndOptions(tagsAndOptions)
	m := &Metric{
		name:  name,
		value: atomic.NewInt64(0),
		tags:  tags,
		opts:  opts,
	}

	if contains(OptMonotonic, m.opts) {
		m.prevValue = atomic.NewInt64(0)
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
	return strings.Join(append([]string{m.name}, m.tags...), ",")
}

// Set value atomically
func (m *Metric) Set(v int64) {
	m.value.Store(v)
}

// Add value atomically
func (m *Metric) Add(v int64) {
	m.value.Add(v)
}

// Get value atomically
func (m *Metric) Get() int64 {
	return m.value.Load()
}

// Swap value atomically
func (m *Metric) Swap(v int64) int64 {
	return m.value.Swap(v)
}

// Delta returns the difference between the current value and the previous one
func (m *Metric) Delta() int64 {
	if m.prevValue == nil {
		// indicates misuse of the library
		log.Errorf("metric %s was not instantiated with telemetry.OptMonotonic", m.name)
		return 0
	}

	current := m.value.Load()
	previous := m.prevValue.Swap(current)
	return current - previous
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
		Tags: m.tags,
		Opts: m.opts,
	})
}

func (m *Metric) isEqual(other *Metric) bool {
	if m.name != other.name || len(m.tags) != len(other.tags) {
		return false
	}

	// Tags are always sorted
	for i := range m.tags {
		if m.tags[i] != other.tags[i] {
			return false
		}
	}

	return true
}
