// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package telemetry provides implementation for the workqueue metrics provider
package telemetry

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

type gaugeWrapper struct {
	telemetry.Gauge
}

// Inc implements the workqueue.GaugeMetric interface
func (g gaugeWrapper) Inc() {
	g.Gauge.Inc()
}

// Dec implements the workqueue.GaugeMetric interface
func (g gaugeWrapper) Dec() {
	g.Gauge.Dec()
}

// Set implements the workqueue.GaugeMetric interface
func (g gaugeWrapper) Set(v float64) {
	g.Gauge.Set(v)
}

type counterWrapper struct {
	telemetry.Counter
}

// Inc implements the workqueue.CounterMetric interface
func (c counterWrapper) Inc() {
	c.Counter.Inc()
}

type histgramWrapper struct {
	telemetry.Histogram
}

// Observer implements the workqueue.HistogramMetric interface
func (l histgramWrapper) Observe(value float64) {
	l.Histogram.Observe(value)
}

// QueueMetricsProvider is a workqueue.MetricsProvider that provides metrics for the SBOM queue
type QueueMetricsProvider struct {
	mutex   *sync.Mutex
	metrics map[string]interface{}
}

// NewQueueMetricsProvider returns a new Queue Metrics Provider
func NewQueueMetricsProvider() QueueMetricsProvider {
	return QueueMetricsProvider{
		mutex:   &sync.Mutex{},
		metrics: map[string]interface{}{},
	}
}

// Ensure QueueMetricsProvider implements the workqueue.MetricsProvider interface
var _ workqueue.MetricsProvider = QueueMetricsProvider{}

type initializer[T any] func(subsystem, name, description string) T

func register[T any](q QueueMetricsProvider, metricName, subsystem, description string, init initializer[T]) T {
	if q.mutex == nil {
		panic("Queue Metrics Provider mutex lock should not be nil")
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if metric, found := q.metrics[metricName]; found {
		return metric.(T)
	}

	metric := init(subsystem, metricName, description)
	q.metrics[metricName] = metric
	return metric
}

// NewDepthMetric creates a new depth metric
func (q QueueMetricsProvider) NewDepthMetric(subsystem string) workqueue.GaugeMetric {
	return register(
		q,
		"queue_depth",
		subsystem,
		"Queue depth",
		func(subsystem, name, description string) workqueue.GaugeMetric {
			return gaugeWrapper{telemetry.NewGaugeWithOpts(
				subsystem,
				name,
				[]string{},
				description,
				commonOpts,
			),
			}
		},
	)
}

// NewAddsMetric creates a new adds metric
func (q QueueMetricsProvider) NewAddsMetric(subsystem string) workqueue.CounterMetric {
	return register(
		q,
		"queue_adds",
		subsystem,
		"Queue adds",
		func(subsystem, name, description string) workqueue.CounterMetric {
			return counterWrapper{telemetry.NewCounterWithOpts(
				subsystem,
				name,
				[]string{},
				description,
				commonOpts,
			)}
		},
	)
}

// NewLatencyMetric creates a new latency metric
func (q QueueMetricsProvider) NewLatencyMetric(subsystem string) workqueue.HistogramMetric {
	return register(
		q,
		"queue_latency",
		subsystem,
		"Queue latency in seconds",
		func(subsystem, name, description string) workqueue.HistogramMetric {
			return histgramWrapper{telemetry.NewHistogramWithOpts(
				subsystem,
				name,
				[]string{},
				description,
				[]float64{1, 15, 60, 120, 600, 1200},
				commonOpts,
			)}
		},
	)
}

// NewWorkDurationMetric creates a new work duration metric
func (q QueueMetricsProvider) NewWorkDurationMetric(subsystem string) workqueue.HistogramMetric {
	return register(
		q,
		"queue_work_duration",
		subsystem,
		"Queue work duration",
		func(subsystem, name, description string) workqueue.HistogramMetric {
			return histgramWrapper{telemetry.NewHistogramWithOpts(
				subsystem,
				name,
				[]string{},
				description,
				prometheus.DefBuckets,
				commonOpts,
			)}
		},
	)
}

// NewUnfinishedWorkSecondsMetric creates a new unfinished work seconds metric
func (q QueueMetricsProvider) NewUnfinishedWorkSecondsMetric(subsystem string) workqueue.SettableGaugeMetric {
	return register(
		q,
		"queue_unfinished_work",
		subsystem,
		"Queue unfinished work in seconds",
		func(subsystem, name, description string) workqueue.SettableGaugeMetric {
			return gaugeWrapper{telemetry.NewGaugeWithOpts(
				subsystem,
				name,
				[]string{},
				description,
				commonOpts,
			)}
		},
	)
}

// NewLongestRunningProcessorSecondsMetric creates a new longest running processor seconds metric
func (q QueueMetricsProvider) NewLongestRunningProcessorSecondsMetric(subsystem string) workqueue.SettableGaugeMetric {
	return register(
		q,
		"queue_longest_running_processor",
		subsystem,
		"Queue longest running processor in seconds",
		func(subsystem, name, description string) workqueue.SettableGaugeMetric {
			return gaugeWrapper{telemetry.NewGaugeWithOpts(
				subsystem,
				name,
				[]string{},
				description,
				commonOpts,
			)}
		},
	)
}

// NewRetriesMetric creates a new retries metric
func (q QueueMetricsProvider) NewRetriesMetric(subsystem string) workqueue.CounterMetric {
	return register(
		q,
		"queue_retries",
		subsystem,
		"Queue retries",
		func(subsystem, name, description string) workqueue.CounterMetric {
			return counterWrapper{telemetry.NewCounterWithOpts(
				subsystem,
				name,
				[]string{},
				description,
				commonOpts,
			)}
		},
	)
}
