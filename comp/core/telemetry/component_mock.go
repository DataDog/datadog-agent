// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless && test

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
)

// Mock implements mock-specific methods.
type Mock interface {
	Component

	// Meter returns a new OTEL meter
	Meter(name string, opts ...metric.MeterOption) metric.Meter
	GetRegistry() *prometheus.Registry
	GetMeterProvider() *sdk.MeterProvider
}
