// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package attributes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// deltaSelector sets delta aggregation temporality for monotonic counters and histograms.
func deltaSelector(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	switch kind {
	case sdkmetric.InstrumentKindCounter,
		sdkmetric.InstrumentKindHistogram,
		sdkmetric.InstrumentKindObservableGauge,
		sdkmetric.InstrumentKindObservableCounter:
		return metricdata.DeltaTemporality
	case sdkmetric.InstrumentKindUpDownCounter,
		sdkmetric.InstrumentKindObservableUpDownCounter:
		return metricdata.CumulativeTemporality
	}
	panic("unknown instrument kind")
}

// AssertHasSumMetric asserts that an OTLP metrics payload has
// a single sum metric with a single datapoint and with the given name and value.
func AssertHasSumMetric[N int64 | float64](t *testing.T, rm *metricdata.ResourceMetrics, name string, value int64) {
	var found bool
	for _, scopeMetric := range rm.ScopeMetrics {
		for _, metric := range scopeMetric.Metrics {
			if metric.Name == name {
				if !found {
					assert.Len(t, metric.Data.(metricdata.Sum[N]).DataPoints, 1)
					assert.Equal(t, value, metric.Data.(metricdata.Sum[N]).DataPoints[0].Value)
					found = true
				} else {
					assert.Fail(t, "metric %s found more than once", name)
				}
			}
		}
	}

	assert.True(t, found, "metric %s not found", name)
}

func TestInternalTelemetryMetrics(t *testing.T) {
	set := componenttest.NewNopTelemetrySettings()
	reader := sdkmetric.NewManualReader(sdkmetric.WithTemporalitySelector(deltaSelector))
	set.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	translator, err := NewTranslator(set)
	require.NoError(t, err)

	resWithHostname := pcommon.NewResource()
	resWithHostname.Attributes().FromRaw(map[string]any{
		"datadog.host.name": "testhost",
	})

	resWithoutHostname := pcommon.NewResource()

	attributeSet := attribute.NewSet(attribute.String("signal", "test"))

	_, _ = translator.ResourceToSource(context.Background(), resWithHostname, attributeSet, nil)
	_, _ = translator.ResourceToSource(context.Background(), resWithoutHostname, attributeSet, nil)
	_, _ = translator.ResourceToSource(context.Background(), resWithoutHostname, attributeSet, nil)

	rm := &metricdata.ResourceMetrics{}
	assert.NoError(t, reader.Collect(context.Background(), rm))
	AssertHasSumMetric[int64](t, rm, missingSourceMetricName, 2)
}
