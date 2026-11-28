// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package metricsevaluator evaluates the Agent's internal telemetry metrics against monitors and
// notifies callers when a monitor changes state.
package metricsevaluator

import (
	"time"
)

// team: fleet-remediation

// Aggregation is how the samples of a monitor's window are reduced to a single value.
type Aggregation string

const (
	// AggregationAvg is the average of the samples in the window. Suited for gauges.
	AggregationAvg Aggregation = "avg"
	// AggregationMax is the highest sample in the window. Suited for gauges.
	AggregationMax Aggregation = "max"
	// AggregationMin is the lowest sample in the window. Suited for gauges.
	AggregationMin Aggregation = "min"
	// AggregationDelta is the increase over the window, counting counter resets. Suited for counters,
	// which are cumulative since the Agent started.
	AggregationDelta Aggregation = "delta"
)

// Comparator is how a monitor's aggregated value is compared to its threshold.
type Comparator string

const (
	// ComparatorAbove alerts when the value is strictly greater than the threshold.
	ComparatorAbove Comparator = "above"
	// ComparatorBelow alerts when the value is strictly lower than the threshold.
	ComparatorBelow Comparator = "below"
)

// NoDataPolicy is what a monitor does when its window cannot be evaluated.
type NoDataPolicy string

const (
	// NoDataKeepState keeps the monitor in its previous state, so an alert stays an alert through data gaps.
	// It is the default.
	NoDataKeepState NoDataPolicy = "keep_state"
	// NoDataNotify moves the monitor to StateNoData, which fires the callback.
	NoDataNotify NoDataPolicy = "notify"
)

// State is the evaluation state of a monitor.
type State string

const (
	// StateNoData means the window does not hold enough samples to be evaluated.
	StateNoData State = "no_data"
	// StateOK means the threshold is not breached.
	StateOK State = "ok"
	// StateAlert means the threshold is breached.
	StateAlert State = "alert"
)

// Event describes a monitor state transition.
type Event struct {
	// Monitor is the name of the monitor that changed state.
	Monitor string
	// Value is the aggregated value of the window. It is meaningless when State is StateNoData.
	Value float64
	// State is the new state of the monitor.
	State State
	// Previous is the state of the monitor before this evaluation.
	Previous State
	// Timestamp is the time of the evaluation.
	Timestamp time.Time
}

// Monitor defines a threshold on an internal telemetry metric.
type Monitor struct {
	// Name uniquely identifies the monitor.
	Name string
	// Metric is the Prometheus name of the metric, e.g. "process_resident_memory_bytes" or
	// "<subsystem>__<name>" for metrics created through the telemetry component.
	Metric string
	// DivideBy is an optional second metric. When set, each sample is Metric divided by DivideBy, both filtered
	// by Tags, e.g. a memory usage over a memory limit. A sample has no value when DivideBy is missing or zero.
	DivideBy string
	// Tags restricts the evaluation to the series having all these label values. Matching series are summed.
	Tags map[string]string
	// Window is the duration the aggregation covers. It is rounded up to a multiple of the evaluation interval.
	Window time.Duration
	// Aggregation reduces the window samples to a single value.
	Aggregation Aggregation
	// Comparator compares the aggregated value to Threshold.
	Comparator Comparator
	// Threshold is the value the aggregated value is compared to.
	Threshold float64
	// OnNoData is what the monitor does when its window has no data. Empty means NoDataKeepState.
	OnNoData NoDataPolicy
	// Callback is called, from the evaluator goroutine, every time the monitor changes state.
	// It must not block.
	Callback func(Event)
}

// Component is the component type.
type Component interface {
	// Register adds a monitor. It returns an error if the monitor is invalid or if a monitor with the same
	// name is already registered.
	Register(m Monitor) error
	// Unregister removes the monitor with the given name, if any.
	Unregister(name string)
}
