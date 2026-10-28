// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

func TestSummaryMetrics(t *testing.T) {
	tests := []struct {
		name     string
		otlpfile string
		ddogfile string
		options  []TranslatorOption
	}{
		{
			name:     "summary",
			otlpfile: "test/otlp/summary/simple.json",
			ddogfile: "test/datadog/summary/simple_summary.json",
			options:  []TranslatorOption{WithFallbackSourceProvider(testProvider("fallbackHostname"))},
		},
		{
			name:     "summary-cumsum-keep",
			otlpfile: "test/otlp/summary/simple.json",
			ddogfile: "test/datadog/summary/simple_summary_cumsum-keep.json",
			options: []TranslatorOption{
				WithFallbackSourceProvider(testProvider("fallbackHostname")),
				WithInitialCumulMonoValueMode(InitialCumulMonoValueModeKeep),
			},
		},
		{
			name:     "summary-with-quantiles",
			otlpfile: "test/otlp/summary/simple.json",
			ddogfile: "test/datadog/summary/simple_summary-quantile.json",
			options: []TranslatorOption{
				WithFallbackSourceProvider(testProvider("fallbackHostname")),
				WithQuantiles(),
			},
		},
		{
			name:     "summary-with-attributes",
			otlpfile: "test/otlp/summary/attributes.json",
			ddogfile: "test/datadog/summary/attributes_summary.json",
			options:  []TranslatorOption{WithFallbackSourceProvider(testProvider("fallbackHostname"))},
		},
		{
			name:     "summary-with-attributes-quantiles",
			otlpfile: "test/otlp/summary/attributes.json",
			ddogfile: "test/datadog/summary/attributes-quantile_summary.json",
			options: []TranslatorOption{
				WithFallbackSourceProvider(testProvider("fallbackHostname")),
				WithQuantiles(),
			},
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			options := append(
				[]TranslatorOption{WithOriginProduct(OriginProductDatadogAgent)},
				testinstance.options...,
			)
			translator := NewTestTranslator(t, options...)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
		// The minimal translator with summary deltas must match the default translator.
		t.Run(testinstance.name+"/minimal", func(t *testing.T) {
			options := append(
				[]TranslatorOption{WithOriginProduct(OriginProductDatadogAgent), WithSummaryCumulativeToDelta()},
				testinstance.options...,
			)
			attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
			require.NoError(t, err)
			translator, err := NewMinimalTranslator(zap.NewNop(), attributesTranslator, options...)
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
	}
}
