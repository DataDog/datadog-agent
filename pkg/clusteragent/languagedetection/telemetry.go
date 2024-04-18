// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const subsystem = "language_detection_patcher"

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

var (
	// Patches is the number of patch requests sent by the patcher to the kubernetes api server
	Patches = telemetry.NewCounterWithOpts(
		subsystem,
		"patches",
		[]string{"owner_kind", "owner_name", "namespace", "status"},
		"Tracks the number of patch requests sent by the patcher to the kubernetes api server",
		commonOpts,
	)

	queueDepth = telemetry.NewGaugeWithOpts(
		subsystem,
		"queue_depth",
		[]string{},
		"Patcher queue depth",
		commonOpts,
	)

	queueAdds = telemetry.NewCounterWithOpts(
		subsystem,
		"queue_adds",
		[]string{},
		"Patcher queue adds",
		commonOpts,
	)

	queueLatency = telemetry.NewHistogramWithOpts(
		subsystem,
		"queue_latency",
		[]string{},
		"Patcher queue latency in seconds",
		[]float64{1, 15, 60, 120, 600, 1200},
		commonOpts,
	)

	workDuration = telemetry.NewHistogramWithOpts(
		subsystem,
		"queue_work_duration",
		[]string{},
		"Patcher queue latency in seconds",
		prometheus.DefBuckets,
		commonOpts,
	)

	unfinishedWork = telemetry.NewGaugeWithOpts(
		subsystem,
		"queue_unfinished_work",
		[]string{},
		"Patcher queue unfinished work in seconds",
		commonOpts,
	)

	longestRunningProcessor = telemetry.NewGaugeWithOpts(
		subsystem,
		"queue_longest_running_processor",
		[]string{},
		"Patcher queue longest running processor in seconds",
		commonOpts,
	)

	queueRetries = telemetry.NewCounterWithOpts(
		subsystem,
		"queue_retries",
		[]string{},
		"Patcher queue retries",
		commonOpts,
	)
)

// QueueMetricProvider is a workqueue.MetricsProvider that provides metrics for the SBOM queue
type queueMetricProvider struct{}

// Ensure QueueMetricProvider implements the workqueue.MetricsProvider interface
var _ workqueue.MetricsProvider = queueMetricProvider{}

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

// NewDepthMetric creates a new depth metric
func (queueMetricProvider) NewDepthMetric(string) workqueue.GaugeMetric {
	return gaugeWrapper{queueDepth}
}

// NewAddsMetric creates a new adds metric
func (queueMetricProvider) NewAddsMetric(string) workqueue.CounterMetric {
	return counterWrapper{queueAdds}
}

// NewLatencyMetric creates a new latency metric
func (queueMetricProvider) NewLatencyMetric(string) workqueue.HistogramMetric {
	return histgramWrapper{queueLatency}
}

// NewWorkDurationMetric creates a new work duration metric
func (queueMetricProvider) NewWorkDurationMetric(string) workqueue.HistogramMetric {
	return histgramWrapper{workDuration}
}

// NewUnfinishedWorkSecondsMetric creates a new unfinished work seconds metric
func (queueMetricProvider) NewUnfinishedWorkSecondsMetric(string) workqueue.SettableGaugeMetric {
	return gaugeWrapper{unfinishedWork}
}

// NewLongestRunningProcessorSecondsMetric creates a new longest running processor seconds metric
func (queueMetricProvider) NewLongestRunningProcessorSecondsMetric(string) workqueue.SettableGaugeMetric {
	return gaugeWrapper{longestRunningProcessor}
}

// NewRetriesMetric creates a new retries metric
func (queueMetricProvider) NewRetriesMetric(string) workqueue.CounterMetric {
	return counterWrapper{queueRetries}
}
