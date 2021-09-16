package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
)

// CheckMetrics stores metrics for the check sampler.
//
// This is similar to ContextMetrics, but provides additional facility to remove metrics.
type CheckMetrics struct {
	// additional time to keep stateful metrics in memory, after the context key has expired
	statefulTimeout float64
	metrics         ContextMetrics
	deadlines       map[ckey.ContextKey]float64
}

// NewCheckMetrics returns new CheckMetrics instance.
func NewCheckMetrics(statefulTimeout time.Duration) CheckMetrics {
	return CheckMetrics{
		statefulTimeout: statefulTimeout.Seconds(),
		metrics:         MakeContextMetrics(),
		deadlines:       nil,
	}
}

// AddSample adds a new sample to the metric with contextKey, initializing a new metric as necessary.
//
// If contextKey is scheduled for removal (see Remove), it will be unscheduled.
//
// See also ContextMetrics.AddSample().
func (cm *CheckMetrics) AddSample(contextKey ckey.ContextKey, sample *MetricSample, timestamp float64, interval int64) error {
	if cm.deadlines != nil {
		delete(cm.deadlines, contextKey)
	}
	return cm.metrics.AddSample(contextKey, sample, timestamp, interval)
}

// Remove deletes metrics associated with the given contextKeys.
//
// Some metrics can not be removed as soon as we stop receiving them (see
// Metric.isStateful()). Stateful metrics will be kept around additional `cm.statefulTimeout`
// time after this call, before ultimately removed. Call to AddSample will cancel delayed
// removal.
func (cm *CheckMetrics) Remove(contextKeys []ckey.ContextKey, timestamp float64) {
	for _, key := range contextKeys {
		if m, ok := cm.metrics[key]; ok {
			if m.isStateful() {
				if cm.deadlines == nil {
					cm.deadlines = make(map[ckey.ContextKey]float64)
				}
				cm.deadlines[key] = timestamp + cm.statefulTimeout
			} else {
				delete(cm.metrics, key)
			}
		}
	}
}

// Flush flushes every metrics in the CheckMetrics (see ContextMetrics.Flush)
func (cm *CheckMetrics) Flush(timestamp float64) ([]*Serie, map[ckey.ContextKey]error) {
	return cm.metrics.Flush(timestamp)
}

// Cleanup removes expired stateful metrics.
func (cm *CheckMetrics) Cleanup(timestamp float64) {
	for key, deadline := range cm.deadlines {
		if deadline < timestamp {
			delete(cm.metrics, key)
			delete(cm.deadlines, key)
		}
	}
}
