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

func TestMapMetrics(t *testing.T) {
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
			otlpfile:                  "testdata/otlpdata/mixed/simple.json",
			ddogfile:                  "testdata/datadogdata/mixed/simple.json",
			expectedUnknownMetricType: 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "resource-attributes-as-tags",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_res-tags.json",
			options: []Option{
				WithResourceAttributesAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "count-sum",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_cs.json",
			options: []Option{
				WithCountSumMetrics(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_ilmd-tags.json",
			options: []Option{
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "instrumentation-scope-metadata-as-tags",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_ismd-tags.json",
			options: []Option{
				WithInstrumentationScopeMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "count-sum-instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_cs-ilmd-tags.json",
			options: []Option{
				WithCountSumMetrics(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "resource-tags-instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_res-ilmd-tags.json",
			options: []Option{
				WithResourceAttributesAsTags(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "count-sum-resource-tags-instrumentation-library-metadata-as-tags",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_cs-both-tags.json",
			options: []Option{
				WithCountSumMetrics(),
				WithResourceAttributesAsTags(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
		},
		{
			name:     "with-all",
			otlpfile: "testdata/otlpdata/mixed/simple.json",
			ddogfile: "testdata/datadogdata/mixed/simple_all.json",
			options: []Option{
				WithCountSumMetrics(),
				WithResourceAttributesAsTags(),
				WithInstrumentationLibraryMetadataAsTags(),
				WithInstrumentationScopeMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 2,
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
