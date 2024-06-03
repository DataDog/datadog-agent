// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type traceNameTest struct {
	name                  string
	inTraces              ptrace.Traces
	outResourceAttributes []map[string]any
}

type traceWithResource struct {
	traceNames         []string
	resourceAttributes map[string]any
}

var (
	inTraceNames = []string{
		"full_name_match",
	}

	standardTraceTests = []traceNameTest{
		{
			name: "one tag with global",
			inTraces: testResourceTraces([]traceWithResource{{
				traceNames: inTraceNames,
				resourceAttributes: map[string]any{
					"container.id": "test",
				},
			}}),
			outResourceAttributes: []map[string]any{{
				"global":       "tag",
				"container.id": "test",
				"container":    "id",
			}},
		},
		{
			name: "two tags with global",
			inTraces: testResourceTraces([]traceWithResource{{
				traceNames: inTraceNames,
				resourceAttributes: map[string]any{
					"container.id":        "test",
					"k8s.namespace.name":  "namespace",
					"k8s.deployment.name": "deployment",
				},
			}}),
			outResourceAttributes: []map[string]any{{
				"global":              "tag",
				"container.id":        "test",
				"k8s.namespace.name":  "namespace",
				"k8s.deployment.name": "deployment",
				"container":           "id",
				"deployment":          "name",
			}},
		},
		{
			name: "two resource traces, two tags with global",
			inTraces: testResourceTraces([]traceWithResource{
				{
					traceNames: inTraceNames,
					resourceAttributes: map[string]any{
						"container.id": "test",
					},
				},
				{
					traceNames: inTraceNames,
					resourceAttributes: map[string]any{
						"k8s.namespace.name":  "namespace",
						"k8s.deployment.name": "deployment",
					},
				}}),
			outResourceAttributes: []map[string]any{
				{
					"global":       "tag",
					"container.id": "test",
					"container":    "id",
				},
				{
					"global":              "tag",
					"k8s.namespace.name":  "namespace",
					"k8s.deployment.name": "deployment",
					"deployment":          "name",
				},
			},
		},
	}
)

func testResourceTraces(twrs []traceWithResource) ptrace.Traces {
	td := ptrace.NewTraces()

	for _, twr := range twrs {
		rs := td.ResourceSpans().AppendEmpty()
		//nolint:errcheck
		rs.Resource().Attributes().FromRaw(twr.resourceAttributes)
		ts := rs.ScopeSpans().AppendEmpty().Spans()
		for _, name := range twr.traceNames {
			ts.AppendEmpty().SetName(name)
		}
	}
	return td
}

func TestInfraAttributesTraceProcessor(t *testing.T) {
	for _, test := range standardTraceTests {
		t.Run(test.name, func(t *testing.T) {
			next := new(consumertest.TracesSink)
			cfg := &Config{
				Traces:      TraceInfraAttributes{},
				Cardinality: types.LowCardinality,
			}
			fakeTagger := taggerimpl.SetupFakeTagger(t)
			defer fakeTagger.ResetTagger()
			fakeTagger.SetTags("container_id://test", "test", []string{"container:id"}, nil, nil, nil)
			fakeTagger.SetTags("deployment://namespace/deployment", "test", []string{"deployment:name"}, nil, nil, nil)
			fakeTagger.SetTags(collectors.GlobalEntityID, "test", []string{"global:tag"}, nil, nil, nil)
			factory := NewFactory(fakeTagger)
			fmp, err := factory.CreateTracesProcessor(
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

			cErr := fmp.ConsumeTraces(context.Background(), test.inTraces)
			assert.Nil(t, cErr)
			assert.NoError(t, fmp.Shutdown(ctx))

			assert.Len(t, next.AllTraces(), 1)
			for i, out := range test.outResourceAttributes {
				trs := next.AllTraces()[0].ResourceSpans().At(i)
				assert.NotNil(t, trs)
				assert.EqualValues(t, out, trs.Resource().Attributes().AsRaw())
			}
		})
	}
}
