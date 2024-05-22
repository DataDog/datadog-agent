// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type metricNameTest struct {
	name                  string
	inMetrics             pmetric.Metrics
	outResourceAttributes map[string]any
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
			name: "one tag",
			inMetrics: testResourceMetrics([]metricWithResource{{
				metricNames: inMetricNames,
				resourceAttributes: map[string]any{
					"container.id": "test",
				},
			}}),
			outResourceAttributes: map[string]any{
				"container.id": "test",
				"container":    "id",
			},
		},
		{
			name: "two tags",
			inMetrics: testResourceMetrics([]metricWithResource{{
				metricNames: inMetricNames,
				resourceAttributes: map[string]any{
					"container.id":        "test",
					"k8s.namespace.name":  "namespace",
					"k8s.deployment.name": "deployment",
				},
			}}),
			outResourceAttributes: map[string]any{
				"container.id":        "test",
				"k8s.namespace.name":  "namespace",
				"k8s.deployment.name": "deployment",
				"container":           "id",
				"deployment":          "name",
			},
		},
	}
)

func TestInfraAttributesMetricProcessor(t *testing.T) {
	for _, test := range standardTests {
		t.Run(test.name, func(t *testing.T) {
			next := new(consumertest.MetricsSink)
			cfg := &Config{
				Metrics:     MetricInfraAttributes{},
				Cardinality: types.LowCardinality,
			}
			fakeTagger := taggerimpl.SetupFakeTagger(t)
			defer fakeTagger.ResetTagger()
			fakeTagger.SetTags("container_id://test", "foo", []string{"container:id"}, nil, nil, nil)
			fakeTagger.SetTags("deployment://namespace/deployment", "foo", []string{"deployment:name"}, nil, nil, nil)
			factory := NewFactory(fakeTagger)
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

			assert.Len(t, next.AllMetrics(), 1)
			assert.EqualValues(t, test.outResourceAttributes, next.AllMetrics()[0].ResourceMetrics().At(0).Resource().Attributes().AsRaw())
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
