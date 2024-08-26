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
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type logNameTest struct {
	name                  string
	inLogs                plog.Logs
	outResourceAttributes []map[string]any
}

type logWithResource struct {
	logNames           []string
	resourceAttributes map[string]any
}

var (
	inLogNames = []string{
		"full_name_match",
	}

	standardLogTests = []logNameTest{
		{
			name: "one tag with global",
			inLogs: testResourceLogs([]logWithResource{{
				logNames: inLogNames,
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
			inLogs: testResourceLogs([]logWithResource{{
				logNames: inLogNames,
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
			name: "two resource metrics, two tags with global",
			inLogs: testResourceLogs([]logWithResource{
				{
					logNames: inLogNames,
					resourceAttributes: map[string]any{
						"container.id": "test",
					},
				},
				{
					logNames: inLogNames,
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

func testResourceLogs(lwrs []logWithResource) plog.Logs {
	ld := plog.NewLogs()
	for _, lwr := range lwrs {
		rl := ld.ResourceLogs().AppendEmpty()
		//nolint:errcheck
		rl.Resource().Attributes().FromRaw(lwr.resourceAttributes)
		lr := rl.ScopeLogs().AppendEmpty()
		for _, name := range lwr.logNames {
			l := lr.Scope()
			l.SetName(name)
		}
	}
	return ld
}

func TestInfraAttributesLogProcessor(t *testing.T) {
	for _, test := range standardLogTests {
		t.Run(test.name, func(t *testing.T) {
			next := new(consumertest.LogsSink)
			cfg := &Config{
				Logs:        LogInfraAttributes{},
				Cardinality: types.LowCardinality,
			}
			fakeTagger := taggerimpl.SetupFakeTagger(t)
			defer fakeTagger.ResetTagger()
			fakeTagger.SetTags("container_id://test", "test", []string{"container:id"}, nil, nil, nil)
			fakeTagger.SetTags("deployment://namespace/deployment", "test", []string{"deployment:name"}, nil, nil, nil)
			fakeTagger.SetTags(collectors.GlobalEntityID, "test", []string{"global:tag"}, nil, nil, nil)
			factory := NewFactory(fakeTagger)
			flp, err := factory.CreateLogsProcessor(
				context.Background(),
				processortest.NewNopSettings(),
				cfg,
				next,
			)
			assert.NotNil(t, flp)
			assert.NoError(t, err)

			caps := flp.Capabilities()
			assert.True(t, caps.MutatesData)
			ctx := context.Background()
			assert.NoError(t, flp.Start(ctx, nil))

			cErr := flp.ConsumeLogs(context.Background(), test.inLogs)
			assert.Nil(t, cErr)
			assert.NoError(t, flp.Shutdown(ctx))

			assert.Len(t, next.AllLogs(), 1)
			for i, out := range test.outResourceAttributes {
				rms := next.AllLogs()[0].ResourceLogs().At(i)
				assert.NotNil(t, rms)
				assert.EqualValues(t, out, rms.Resource().Attributes().AsRaw())
			}
		})
	}
}
