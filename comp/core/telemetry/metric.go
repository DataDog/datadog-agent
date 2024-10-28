// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless

package telemetry

import (
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/metric"
)

// MeterOption is an alias to metric.MeterOption
type MeterOption = metric.MeterOption

// Meter is an alias to metric.Meter
type Meter = metric.Meter

// MetricFamily is an alias to dto.MetricFamily
type MetricFamily = dto.MetricFamily
