// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package translator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestDeltaHistogramOptions(t *testing.T) {
	tests := []struct {
		name     string
		otlpfile string
		ddogfile string
		options  []Option
	}{
		{
			name:     "distributions",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_dist-nocs.json",
			options: []Option{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "distributions-count-sum",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_dist-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeDistributions),
				WithCountSumMetrics(),
			},
		},
		{
			name:     "buckets",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_counters-nocs.json",
			options: []Option{
				WithHistogramMode(HistogramModeCounters),
			},
		},
		{
			name:     "buckets-count-sum",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_counters-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeCounters),
				WithCountSumMetrics(),
			},
		},
		{
			name:     "count-sum",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_nobuckets-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeNoBuckets),
				WithCountSumMetrics(),
			},
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			translator, err := New(zap.NewNop(), testinstance.options...)
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
	}
}

func TestCumulativeHistogramOptions(t *testing.T) {
	tests := []struct {
		name     string
		otlpfile string
		ddogfile string
		options  []Option
	}{
		{
			name:     "distributions",
			otlpfile: "testdata/otlpdata/histogram/simple-cumulative.json",
			ddogfile: "testdata/datadogdata/histogram/simple-cumulative_dist-nocs.json",
			options: []Option{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "distributions-count-sum",
			otlpfile: "testdata/otlpdata/histogram/simple-cumulative.json",
			ddogfile: "testdata/datadogdata/histogram/simple-cumulative_dist-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeDistributions),
				WithCountSumMetrics(),
			},
		},
		{
			name:     "buckets",
			otlpfile: "testdata/otlpdata/histogram/simple-cumulative.json",
			ddogfile: "testdata/datadogdata/histogram/simple-cumulative_counters-nocs.json",
			options: []Option{
				WithHistogramMode(HistogramModeCounters),
			},
		},
		{
			name:     "buckets-count-sum",
			otlpfile: "testdata/otlpdata/histogram/simple-cumulative.json",
			ddogfile: "testdata/datadogdata/histogram/simple-cumulative_counters-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeCounters),
				WithCountSumMetrics(),
			},
		},
		{
			name:     "count-sum",
			otlpfile: "testdata/otlpdata/histogram/simple-cumulative.json",
			ddogfile: "testdata/datadogdata/histogram/simple-cumulative_nobuckets-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeNoBuckets),
				WithCountSumMetrics(),
			},
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			translator, err := New(zap.NewNop(), testinstance.options...)
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
	}
}

func TestExponentialHistogramOptions(t *testing.T) {
	tests := []struct {
		name                                      string
		otlpfile                                  string
		ddogfile                                  string
		options                                   []Option
		expectedUnknownMetricType                 int
		expectedUnsupportedAggregationTemporality int
	}{
		{
			name:                      "no-options",
			otlpfile:                  "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile:                  "testdata/datadogdata/histogram/simple-exponential.json",
			expectedUnknownMetricType: 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "resource-attributes-as-tags",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_res-tags.json",
			options: []Option{
				WithResourceAttributesAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "count-sum",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_cs.json",
			options: []Option{
				WithCountSumMetrics(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_ilmd-tags.json",
			options: []Option{
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "instrumentation-scope-metadata-as-tags",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_ismd-tags.json",
			options: []Option{
				WithInstrumentationScopeMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "count-sum-instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_cs-ilmd-tags.json",
			options: []Option{
				WithCountSumMetrics(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "resource-tags-instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_res-ilmd-tags.json",
			options: []Option{
				WithResourceAttributesAsTags(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "count-sum-resource-tags-instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_cs-both-tags.json",
			options: []Option{
				WithCountSumMetrics(),
				WithResourceAttributesAsTags(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "with-all",
			otlpfile: "testdata/otlpdata/histogram/simple-exponential.json",
			ddogfile: "testdata/datadogdata/histogram/simple-exponential_all.json",
			options: []Option{
				WithCountSumMetrics(),
				WithResourceAttributesAsTags(),
				WithInstrumentationLibraryMetadataAsTags(),
				WithInstrumentationScopeMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			testLogger := zap.New(core)
			translator, err := New(testLogger, testinstance.options...)
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
			assert.Equal(t, testinstance.expectedUnknownMetricType, observed.FilterMessage("Unknown or unsupported metric type").Len())
			assert.Equal(t, testinstance.expectedUnsupportedAggregationTemporality, observed.FilterMessage("Unknown or unsupported aggregation temporality").Len())
		})
	}
}
