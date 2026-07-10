// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

func newTranslatorWithOptions(t *testing.T, opts ...TranslatorOption) *defaultTranslator {
	set := componenttest.NewNopTelemetrySettings()
	set.Logger = zap.NewNop()

	attributesTranslator, err := attributes.NewTranslator(set)
	require.NoError(t, err)

	options := append([]TranslatorOption{WithFallbackSourceProvider(testProvider(fallbackHostname))}, opts...)
	tr, err := NewDefaultTranslator(set, attributesTranslator, options...)
	require.NoError(t, err)
	return tr.(*defaultTranslator)
}

// gaugeMetricsWithUnit builds a single-point gauge metric carrying the given OTLP unit.
func gaugeMetricsWithUnit(name, unit string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(name)
	met.SetUnit(unit)
	met.SetEmptyGauge()
	dp := met.Gauge().DataPoints().AppendEmpty()
	dp.SetDoubleValue(1)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Unix(1, 0)))
	return md
}

func TestUnits(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		wantUnit string
	}{
		{"byte", "By", "byte"},
		{"rate", "KiBy/s", "kibibyte/second"},
		{"annotation", "{cpu}", "cpu"},
		{"dimensionless", "1", ""},
		{"unknown", "furlong", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newTranslatorWithOptions(t, WithUnits())
			consumer := newTestConsumer()
			_, err := tr.MapMetrics(context.Background(), gaugeMetricsWithUnit("test.metric", tt.unit), &consumer, nil)
			require.NoError(t, err)
			require.Len(t, consumer.data.Metrics.TimeSeries, 1)
			assert.Equal(t, tt.wantUnit, consumer.data.Metrics.TimeSeries[0].Unit)
		})
	}
}

func TestNoUnits(t *testing.T) {
	tr := newTranslatorWithOptions(t)
	consumer := newTestConsumer()
	_, err := tr.MapMetrics(context.Background(), gaugeMetricsWithUnit("test.metric", "By"), &consumer, nil)
	require.NoError(t, err)
	require.Len(t, consumer.data.Metrics.TimeSeries, 1)
	assert.Empty(t, consumer.data.Metrics.TimeSeries[0].Unit)
}
