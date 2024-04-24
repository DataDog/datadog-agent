// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package tagenrichmentprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor/processortest"
)

type metricNameTest struct {
	name      string
	inMetrics pmetric.Metrics
}

type metricWithResource struct {
	metricNames        []string
	resourceAttributes map[string]any
}

var (
	inMetricNames = []string{
		"full_name_match",
	}

	standardTests = []metricNameTest{
		{
			name:      "includeFilter",
			inMetrics: testResourceMetrics([]metricWithResource{{metricNames: inMetricNames}}),
		},
	}
)

func TestTagEnrichmentMetricProcessor(t *testing.T) {
	for _, test := range standardTests {
		t.Run(test.name, func(t *testing.T) {
			next := new(consumertest.MetricsSink)
			cfg := &Config{
				Metrics: MetricTagEnrichment{
				},
			}
			factory := NewFactory()
			fmp, err := factory.CreateMetricsProcessor(
				context.Background(),
				processortest.NewNopCreateSettings(),
				cfg,
				next,
			)
			assert.NotNil(t, fmp)
			assert.NoError(t, err)

			caps := fmp.Capabilities()
			assert.True(t, caps.MutatesData)
			ctx := context.Background()
			assert.NoError(t, fmp.Start(ctx, nil))

			cErr := fmp.ConsumeMetrics(context.Background(), test.inMetrics)
			assert.Nil(t, cErr)
			assert.NoError(t, fmp.Shutdown(ctx))
		})
	}
}

func testResourceMetrics(mwrs []metricWithResource) pmetric.Metrics {
	md := pmetric.NewMetrics()

	for _, mwr := range mwrs {
		rm := md.ResourceMetrics().AppendEmpty()
		//nolint:errcheck
		rm.Resource().Attributes().FromRaw(mwr.resourceAttributes)
		ms := rm.ScopeMetrics().AppendEmpty().Metrics()
		for _, name := range mwr.metricNames {
			m := ms.AppendEmpty()
			m.SetName(name)
		}
	}
	return md
}
