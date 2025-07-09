// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics holds metrics related files
package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// NewITCounter creates a new telemetry.Counter with the given Datadog metric name, tags, and help text.
func NewITCounter(metric ITMetric, tags []string, help string) telemetry.Counter {
	return telemetry.NewCounter(metric.Subsystem, metric.Name, tags, help)
}

// NewITGauge creates a new telemetry.Gauge with the given Datadog metric name, tags, and help text.
func NewITGauge(metric ITMetric, tags []string, help string) telemetry.Gauge {
	return telemetry.NewGauge(metric.Subsystem, metric.Name, tags, help)
}
