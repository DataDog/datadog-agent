// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serializerexporter contains the impleemntation of an exporter which is able
// to serialize OTLP Metrics to an agent demultiplexer.
package serializerexporter

import (
	"context"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

var _ component.Config = (*exporterConfig)(nil)

func newDefaultConfig() component.Config {
	panic("not called")
}

var _ source.Provider = (*sourceProviderFunc)(nil)

// sourceProviderFunc is an adapter to allow the use of a function as a metrics.HostnameProvider.
type sourceProviderFunc func(context.Context) (string, error)

// Source calls f and wraps in a source struct.
func (f sourceProviderFunc) Source(ctx context.Context) (source.Source, error) {
	panic("not called")
}

// exporter translate OTLP metrics into the Datadog format and sends
// them to the agent serializer.
type exporter struct {
	tr          *metrics.Translator
	s           serializer.MetricSerializer
	hostname    string
	extraTags   []string
	cardinality collectors.TagCardinality
}

func translatorFromConfig(set component.TelemetrySettings, attributesTranslator *attributes.Translator, cfg *exporterConfig) (*metrics.Translator, error) {
	panic("not called")
}

func newExporter(set component.TelemetrySettings, attributesTranslator *attributes.Translator, s serializer.MetricSerializer, cfg *exporterConfig) (*exporter, error) {
	panic("not called")
}

func (e *exporter) ConsumeMetrics(ctx context.Context, ld pmetric.Metrics) error {
	panic("not called")
}
