// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp

package serializerexporter

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pmetric"

	otlpmetrics "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
)

func (c *serializerConsumer) ConsumeExplicitBoundHistogram(_ context.Context, _ *otlpmetrics.Dimensions, _ uint64, _ int64, _ pmetric.HistogramDataPoint, _ bool) {
}

func (c *serializerConsumer) ConsumeExponentialHistogram(_ context.Context, _ *otlpmetrics.Dimensions, _ uint64, _ int64, _ pmetric.ExponentialHistogramDataPoint) {
}
