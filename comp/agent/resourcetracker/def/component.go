// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package resourcetracker offers a resource (CPU/RSS/...) tracking component.
// It submits resource usage metrics for use in Fleet Automation.
package resourcetracker

import "github.com/DataDog/datadog-agent/pkg/telemetry"

// team: fleet-automation

// Component is the component type.
type Component interface {
}

// Submitter is the interface to submit gauge metrics.
type Submitter interface {
	Gauge(name string, value float64, tags []string)
}

type telemetrySubmitter struct{}

func (t *telemetrySubmitter) Gauge(name string, value float64, tags []string) {
	telemetry.GetStatsTelemetryProvider().Gauge(name, value, tags)
}

// NewTelemetryGaugeSubmitter returns a new Submitter that submits gauge metrics to the telemetry provider.
func NewTelemetryGaugeSubmitter() Submitter {
	return &telemetrySubmitter{}
}
