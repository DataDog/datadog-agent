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
	metrics []*Metric
}

// GetMetrics returns all metrics matching a certain criteria
func GetMetrics(params ...string) []*Metric {
	filters := sets.NewString()
	filters.Insert(params...)

	globalRegistry.Lock()
	defer globalRegistry.Unlock()

	result := make([]*Metric, 0, len(globalRegistry.metrics))
	if filters.Len() == 0 {
		// if no filters were provided we return all metrics
		result = append(result, globalRegistry.metrics...)
		return result
	}

	for _, m := range globalRegistry.metrics {
		if m.opts.IsSuperset(filters) {
			result = append(result, m)
		}
	}

	return result
}

// Clear metrics
// WARNING: Only intended for tests
func Clear() {
	globalRegistry.Lock()
	defer globalRegistry.Unlock()
	globalRegistry.metrics = nil
}

func init() {
	globalRegistry = new(registry)
}
