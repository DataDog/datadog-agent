// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmCheckMetricsTotal   = telemetry.NewGauge("check_metrics", "total_metrics", []string{"stateful"}, "Gauge of all check metrics")
	tlmCheckMetricsActive  = telemetry.NewGauge("check_metrics", "active_metrics", []string{"stateful"}, "Gauge of non-expired check metrics (total - waiting)")
	tlmCheckMetricsWaiting = telemetry.NewGauge("check_metrics", "waiting_metrics", []string{"stateful"}, "Gauge of expired check metrics waiting for timeout")
	tlmCheckMetricsAdded   = telemetry.NewCounter("check_metrics", "created_total", []string{"stateful"}, "Count of new check metrics added")
	tlmCheckMetricsExpired = telemetry.NewCounter("check_metrics", "expired_total", []string{"stateful"}, "Count of expired metrics")
	tlmCheckMetricsRemoved = telemetry.NewCounter("check_metrics", "removed_total", []string{"stateful"}, "Count of removed metrics")

	checkMetricsAddSampleTelemetry = &AddSampleTelemetry{
		Total:     tlmCheckMetricsAdded.WithValues("sum"),
		Stateful:  tlmCheckMetricsAdded.WithValues("true"),
		Stateless: tlmCheckMetricsAdded.WithValues("false"),
	}
	tlmCheckMetricsExpiredTotal     = tlmCheckMetricsExpired.WithValues("sum")
	tlmCheckMetricsExpiredStateful  = tlmCheckMetricsExpired.WithValues("true")
	tlmCheckMetricsExpiredStateless = tlmCheckMetricsExpired.WithValues("false")
	tlmCheckMetricsRemovedTotal     = tlmCheckMetricsRemoved.WithValues("sum")
	tlmCheckMetricsRemovedStateful  = tlmCheckMetricsRemoved.WithValues("true")
	tlmCheckMetricsRemovedStateless = tlmCheckMetricsRemoved.WithValues("false")
)

// CheckMetrics stores metrics for the check sampler.
//
// This is similar to ContextMetrics, but provides additional facility to remove metrics.
//
// Metrics can be requested to be removed by calling Expire(). Metrics that keep state
// between flushes (see Metric.isStateful) are kept for additional statefulTimeout seconds
// after expiration, as a precaution against checks that send metrics intermittently.
// Older stateful metrics need to be cleaned up by calling RemoveExpired().
type CheckMetrics struct {
	expireMetrics bool
	// additional time to keep stateful metrics in memory, after the context key has expired
	statefulTimeout float64
	metrics         ContextMetrics
	deadlines       map[ckey.ContextKey]float64
}

// NewCheckMetrics returns new CheckMetrics instance.
func NewCheckMetrics(expireMetrics bool, statefulTimeout time.Duration) CheckMetrics {
	return CheckMetrics{
		expireMetrics:   expireMetrics,
		statefulTimeout: statefulTimeout.Seconds(),
		metrics:         MakeContextMetrics(),
		// many checks do not have stateful metrics, so avoid allocating `deadlines` unless required
		deadlines: nil,
	}
}

// AddSample adds a new sample to the metric with contextKey, initializing a new metric as necessary.
//
// If contextKey is scheduled for removal (see Expire), it will be unscheduled.
//
// See also ContextMetrics.AddSample().
func (cm *CheckMetrics) AddSample(contextKey ckey.ContextKey, sample *MetricSample, timestamp float64, interval int64, config pkgconfigmodel.Config) error {
	if cm.deadlines != nil {
		delete(cm.deadlines, contextKey)
	}
	return cm.metrics.AddSample(contextKey, sample, timestamp, interval, checkMetricsAddSampleTelemetry, config)
}

// Expire enables metric data for given context keys to be removed.
//
// Metrics that do not keep state between flushes, will be removed immediately.
//
// Metrics that do keep state, will be kept around for additional `cm.statefulTimeout`
// time after timestamp, before ultimately removed (See RemoveExpired). Call to AddSample
// will cancel delayed removal.
func (cm *CheckMetrics) Expire(contextKeys []ckey.ContextKey, timestamp float64) {
	if !cm.expireMetrics {
		return
	}

	expiredStateless := 0.0
	expiredStateful := 0.0
	for _, key := range contextKeys {
		if m, ok := cm.metrics[key]; ok {
			if m.isStateful() {
				expiredStateful++
				if cm.deadlines == nil {
					cm.deadlines = make(map[ckey.ContextKey]float64)
				}
				cm.deadlines[key] = timestamp + cm.statefulTimeout
			} else {
				expiredStateless++
				delete(cm.metrics, key)
			}
		}
	}

	tlmCheckMetricsExpiredTotal.Add(expiredStateless + expiredStateful)
	tlmCheckMetricsExpiredStateful.Add(expiredStateful)
	tlmCheckMetricsExpiredStateless.Add(expiredStateless)

	tlmCheckMetricsRemovedTotal.Add(expiredStateless)
	tlmCheckMetricsRemovedStateless.Add(expiredStateless)
}

// Flush flushes every metrics in the CheckMetrics (see ContextMetrics.Flush)
func (cm *CheckMetrics) Flush(timestamp float64) ([]*Serie, map[ckey.ContextKey]error) {
	return cm.metrics.Flush(timestamp)
}

// RemoveExpired removes stateful metrics that have expired before the given timestamp.
func (cm *CheckMetrics) RemoveExpired(timestamp float64) {
	removed := 0.0
	for key, deadline := range cm.deadlines {
		if deadline < timestamp {
			removed++
			delete(cm.metrics, key)
			delete(cm.deadlines, key)
		}
	}
	tlmCheckMetricsRemovedTotal.Add(removed)
	tlmCheckMetricsRemovedStateful.Add(removed)
}

// CheckMetricsTelemetryAccumulator aggregates telemetry collected from multiple
// CheckMetrics instances.
type CheckMetricsTelemetryAccumulator struct {
	statefulTotal, statefulWaiting   float64
	statelessTotal, statelessWaiting float64
}

// VisitCheckMetrics adds metrics from CheckMetrics instance to the accumulator.
func (c *CheckMetricsTelemetryAccumulator) VisitCheckMetrics(cm *CheckMetrics) {
	for k, m := range cm.metrics {
		if m.isStateful() {
			c.statefulTotal++
			if _, ok := cm.deadlines[k]; ok {
				c.statefulWaiting++
			}
		} else {
			c.statelessTotal++
			if _, ok := cm.deadlines[k]; ok {
				c.statelessWaiting++
			}
		}
	}
}

// Flush updates telemetry counters based on aggregated statistics.
func (c *CheckMetricsTelemetryAccumulator) Flush() {
	tlmCheckMetricsTotal.Set(c.statefulTotal, "true")
	tlmCheckMetricsWaiting.Set(c.statefulWaiting, "true")
	tlmCheckMetricsActive.Set(c.statefulTotal-c.statefulWaiting, "true")

	tlmCheckMetricsTotal.Set(c.statelessTotal, "false")
	tlmCheckMetricsWaiting.Set(c.statelessWaiting, "false")
	tlmCheckMetricsActive.Set(c.statelessTotal-c.statelessWaiting, "false")

	total := c.statefulTotal + c.statelessTotal
	waiting := c.statefulWaiting + c.statelessWaiting
	tlmCheckMetricsTotal.Set(total, "sum")
	tlmCheckMetricsWaiting.Set(waiting, "sum")
	tlmCheckMetricsActive.Set(total-waiting, "sum")
}
