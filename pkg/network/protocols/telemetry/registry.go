// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
)

var globalRegistry *registry

type registry struct {
	sync.Mutex
	metrics map[string]metric
}

func (r *registry) FindOrCreate(m metric) metric {
	r.Lock()
	defer r.Unlock()

	if r.metrics == nil {
		r.metrics = make(map[string]metric)
	}

	name := m.base().Name()
	if v, ok := r.metrics[name]; ok {
		return v
	}

	r.metrics[name] = m
	return m
}

// GetMetrics returns all metrics matching a certain criteria
func (r *registry) GetMetrics(params ...string) []metric {
	filters := sets.New[string]()
	filters.Insert(params...)

	r.Lock()
	defer r.Unlock()

	result := make([]metric, 0, len(globalRegistry.metrics))
	for _, metricInterface := range r.metrics {
		m := metricInterface.base()
		if filters.Len() == 0 {
			// if no filters were provided we return all metrics
			result = append(result, metricInterface)
			continue
		}

		if m.opts.IsSuperset(filters) {
			result = append(result, metricInterface)
		}
	}

	return result
}

// Clear metrics
// WARNING: Only intended for tests
func Clear() {
	globalRegistry.Lock()
	globalRegistry.metrics = nil
	globalRegistry.Unlock()

	telemetryDelta.mux.Lock()
	telemetryDelta.stateByClientID = make(map[string]*clientState)
	telemetryDelta.mux.Unlock()
}

func init() {
	globalRegistry = new(registry)
}
