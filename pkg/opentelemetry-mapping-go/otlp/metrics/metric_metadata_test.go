// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

type metricMetadata struct {
	name, description, unit string
}

type metadataConsumer struct {
	testConsumer
	metadata []metricMetadata
}

func (c *metadataConsumer) ConsumeMetricMetadata(name, description, unit string) {
	c.metadata = append(c.metadata, metricMetadata{name, description, unit})
}

// metricWithDescription builds a delta metric without data points: metadata does not depend on them.
func metricWithDescription(name, description, unit string, typ pmetric.MetricType) pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(name)
	met.SetDescription(description)
	met.SetUnit(unit)
	switch typ {
	case pmetric.MetricTypeGauge:
		met.SetEmptyGauge()
	case pmetric.MetricTypeSum:
		met.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	case pmetric.MetricTypeHistogram:
		met.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	case pmetric.MetricTypeExponentialHistogram:
		met.SetEmptyExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	case pmetric.MetricTypeSummary:
		met.SetEmptySummary()
	}
	return md
}

func TestMetricMetadata(t *testing.T) {
	tests := []struct {
		name    string
		metrics pmetric.Metrics
		options []TranslatorOption
		want    []metricMetadata
	}{
		{
			name:    "gauge",
			metrics: metricWithDescription("test.metric", "A gauge.", "By", pmetric.MetricTypeGauge),
			options: []TranslatorOption{WithMetricMetadata()},
			want:    []metricMetadata{{"test.metric", "A gauge.", "byte"}},
		},
		{
			name:    "rate unit",
			metrics: metricWithDescription("test.metric", "A sum.", "KiBy/s", pmetric.MetricTypeSum),
			options: []TranslatorOption{WithMetricMetadata()},
			want:    []metricMetadata{{"test.metric", "A sum.", "kibibyte/second"}},
		},
		{
			name:    "unknown unit",
			metrics: metricWithDescription("test.metric", "A gauge.", "furlong", pmetric.MetricTypeGauge),
			options: []TranslatorOption{WithMetricMetadata()},
			want:    []metricMetadata{{"test.metric", "A gauge.", ""}},
		},
		{
			name:    "no description",
			metrics: metricWithDescription("test.metric", "", "By", pmetric.MetricTypeGauge),
			options: []TranslatorOption{WithMetricMetadata()},
		},
		{
			name:    "disabled",
			metrics: metricWithDescription("test.metric", "A gauge.", "By", pmetric.MetricTypeGauge),
		},
		{
			name:    "exponential histogram",
			metrics: metricWithDescription("test.metric", "A histogram.", "ms", pmetric.MetricTypeExponentialHistogram),
			options: []TranslatorOption{WithMetricMetadata()},
			want:    []metricMetadata{{"test.metric", "A histogram.", "millisecond"}},
		},
		{
			name:    "histogram as distribution",
			metrics: metricWithDescription("test.metric", "A histogram.", "ms", pmetric.MetricTypeHistogram),
			options: []TranslatorOption{WithMetricMetadata(), WithHistogramMode(HistogramModeDistributions)},
			want:    []metricMetadata{{"test.metric", "A histogram.", "millisecond"}},
		},
		{
			name:    "histogram as counters",
			metrics: metricWithDescription("test.metric", "A histogram.", "ms", pmetric.MetricTypeHistogram),
			options: []TranslatorOption{WithMetricMetadata(), WithHistogramMode(HistogramModeCounters)},
		},
		{
			name:    "summary",
			metrics: metricWithDescription("test.metric", "A summary.", "ms", pmetric.MetricTypeSummary),
			options: []TranslatorOption{WithMetricMetadata()},
		},
		{
			name:    "remapped",
			metrics: metricWithDescription("system.cpu.load_average.1m", "Load average.", "{thread}", pmetric.MetricTypeGauge),
			options: []TranslatorOption{WithMetricMetadata(), WithRemapping()},
			want:    []metricMetadata{{"otel.system.cpu.load_average.1m", "Load average.", "thread"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newTranslatorWithOptions(t, tt.options...)
			consumer := metadataConsumer{testConsumer: newTestConsumer()}
			_, err := tr.MapMetrics(context.Background(), tt.metrics, &consumer, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, consumer.metadata)
		})
	}
}

func TestMetricMetadataMinimalTranslator(t *testing.T) {
	tests := []struct {
		name    string
		metrics pmetric.Metrics
		want    []metricMetadata
	}{
		{
			name:    "histogram",
			metrics: metricWithDescription("test.metric", "A histogram.", "ms", pmetric.MetricTypeHistogram),
			want:    []metricMetadata{{"test.metric", "A histogram.", "millisecond"}},
		},
		{
			name:    "summary",
			metrics: metricWithDescription("test.metric", "A summary.", "ms", pmetric.MetricTypeSummary),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
			require.NoError(t, err)
			tr, err := NewMinimalTranslator(zap.NewNop(), attributesTranslator,
				WithFallbackSourceProvider(testProvider(fallbackHostname)), WithMetricMetadata())
			require.NoError(t, err)
			consumer := metadataConsumer{testConsumer: newTestConsumer()}
			_, err = tr.MapMetrics(context.Background(), tt.metrics, &consumer, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, consumer.metadata)
		})
	}
}
