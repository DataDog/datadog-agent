// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import "github.com/prometheus/client_golang/prometheus"

// NoopDebugFsStatCollector implements the prometheus Collector interface but does nothing
type NoopDebugFsStatCollector struct{}

// Describe returns all descriptions of the collector
func (c *NoopDebugFsStatCollector) Describe(chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (c *NoopDebugFsStatCollector) Collect(chan<- prometheus.Metric) {}
