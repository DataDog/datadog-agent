// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"sync"
)

var globalRegistry *registry

type registry struct {
	sync.Mutex
	metrics []*Metric
}

// GetMetrics returns all metrics matching a certain set of tags
func GetMetrics(tags ...string) []*Metric {
	filterIndex := make(map[string]struct{}, len(tags))
	for _, f := range tags {
		filterIndex[f] = struct{}{}
	}

	globalRegistry.Lock()
	defer globalRegistry.Unlock()

	result := make([]*Metric, 0, len(globalRegistry.metrics))
	if len(filterIndex) == 0 {
		// if no filters were provided we return all metrics
		result = append(result, globalRegistry.metrics...)
		return result
	}

	for _, m := range globalRegistry.metrics {
		if matches(filterIndex, m) {
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

func matches(filters map[string]struct{}, metric *Metric) bool {
	var totalMatches int

	for _, tag := range metric.opts {
		if _, ok := filters[tag]; ok {
			totalMatches++
			if totalMatches == len(filters) {
				return true
			}

		}
	}

	return false
}

func init() {
	globalRegistry = new(registry)
}
