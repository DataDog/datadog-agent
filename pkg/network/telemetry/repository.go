package telemetry

import (
	"sync"
)

var r *repository

type repository struct {
	sync.Mutex
	metrics []*Metric
}

// GetMetrics returns all metrics matching a certain set of tags
func GetMetrics(tags ...string) []*Metric {
	filterIndex := make(map[string]struct{}, len(tags))
	for _, f := range tags {
		filterIndex[f] = struct{}{}
	}

	r.Lock()
	defer r.Unlock()

	if len(filterIndex) == 0 {
		// if no filters were provided we return all metrics
		return r.metrics
	}

	result := make([]*Metric, 0, len(r.metrics))
	for _, m := range r.metrics {
		if matches(filterIndex, m) {
			result = append(result, m)
		}
	}

	return result
}

// Clear metrics
// WARNING: Only intended for tests
func Clear() {
	r.Lock()
	defer r.Unlock()
	r.metrics = nil
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
	r = new(repository)
}
