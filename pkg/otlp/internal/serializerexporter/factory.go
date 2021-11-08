// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	// TypeStr defines the serializer exporter type string.
	TypeStr = "serializer"
)

type factory struct {
	s serializer.MetricSerializer
}

// NewFactory creates a new serializer exporter factory.
func NewFactory(s serializer.MetricSerializer) component.ExporterFactory {
	f := &factory{s}

	return exporterhelper.NewFactory(
		TypeStr,
		newDefaultConfig,
		exporterhelper.WithMetrics(f.createMetricExporter),
	)
}

func (f *factory) createMetricExporter(_ context.Context, params component.ExporterCreateSettings, c config.Exporter) (component.MetricsExporter, error) {
	cfg := c.(*exporterConfig)

	exp, err := newExporter(params.Logger, f.s, cfg)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewMetricsExporter(cfg, params, exp.ConsumeMetrics,
		exporterhelper.WithQueue(cfg.QueueSettings),
		exporterhelper.WithTimeout(cfg.TimeoutSettings),
	)
}
