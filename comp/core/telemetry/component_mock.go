// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	sdk "go.opentelemetry.io/otel/sdk/metric"
)

// Mock implements mock-specific methods.
// Mock implements mock-specific methods.
type Mock interface {
	Component

	GetRegistry() *prometheus.Registry
	GetMeterProvider() *sdk.MeterProvider
}
