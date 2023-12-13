// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package retry

import "github.com/DataDog/datadog-agent/pkg/telemetry"

func NewPointCountTelemetryMock() *PointCountTelemetry {
	provider := telemetry.NewStatsTelemetryProvider(StatsTelemetrySenderMock{})
	return NewPointCountTelemetry("domain", provider)
}

type StatsTelemetrySenderMock struct{}

func (m StatsTelemetrySenderMock) Count(metric string, value float64, hostname string, tags []string) {
}
func (m StatsTelemetrySenderMock) Gauge(metric string, value float64, hostname string, tags []string) {
}
func (m StatsTelemetrySenderMock) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
}
