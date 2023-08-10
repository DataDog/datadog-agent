// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/DataDog/datadog-agent/pkg/serializer"
)

const (
	// TypeStr defines the serializer exporter type string.
	TypeStr   = "serializer"
	stability = component.StabilityLevelStable
)

type factory struct {
	s serializer.MetricSerializer
}

// NewFactory creates a new serializer exporter factory.
func NewFactory(s serializer.MetricSerializer) exp.Factory {
	f := &factory{s}

	return exp.NewFactory(
		TypeStr,
		newDefaultConfig,
		exp.WithMetrics(f.createMetricExporter, stability),
	)
}

func (f *factory) createMetricExporter(ctx context.Context, params exp.CreateSettings, c component.Config) (exp.Metrics, error) {
	cfg := c.(*exporterConfig)

	newExp, err := newExporter(params.Logger, f.s, cfg)
	if err != nil {
		return nil, err
	}

	exporter, err := exporterhelper.NewMetricsExporter(ctx, params, cfg, newExp.ConsumeMetrics,
		exporterhelper.WithQueue(cfg.QueueSettings),
		exporterhelper.WithTimeout(cfg.TimeoutSettings),
		// the metrics remapping code mutates data
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
	if err != nil {
		return nil, err
	}

	return resourcetotelemetry.WrapMetricsExporter(
		resourcetotelemetry.Settings{Enabled: cfg.Metrics.ExporterConfig.ResourceAttributesAsTags}, exporter), nil
}
