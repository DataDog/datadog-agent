// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides types and functions for kernel telemetry collected by eBPF programs.
package telemetry

import "github.com/prometheus/client_golang/prometheus"

// NoopEbpfErrorsCollector implements the prometheus Collector interface but does nothing
type NoopEbpfErrorsCollector struct{}

// Describe returns all descriptions of the collector
func (e *NoopEbpfErrorsCollector) Describe(chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (e *NoopEbpfErrorsCollector) Collect(chan<- prometheus.Metric) {}
