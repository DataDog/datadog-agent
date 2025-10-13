// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processortest"

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
				},
			}),
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
		{
			name: "detect container.id from PID",
			inLogs: testResourceLogs([]logWithResource{
				{
					logNames: inLogNames,
					resourceAttributes: map[string]any{
						"process.pid": int64(12345),
					},
				},
			}),
			outResourceAttributes: []map[string]any{
				{
					"global":       "tag",
					"process.pid":  int64(12345),
					"container.id": "test",
					"container":    "id",
				},
			},
		},
		{
			name: "detect container.id from cgroup inode",
			inLogs: testResourceLogs([]logWithResource{
				{
					logNames: inLogNames,
					resourceAttributes: map[string]any{
						"datadog.container.cgroup_inode": int64(12345),
					},
				},
			}),
			outResourceAttributes: []map[string]any{
				{
					"global":                         "tag",
					"datadog.container.cgroup_inode": int64(12345),
					"container.id":                   "test",
					"container":                      "id",
				},
			},
		},
		{
			name: "detect container.id from pod UID + container name",
			inLogs: testResourceLogs([]logWithResource{
				{
					logNames: inLogNames,
					resourceAttributes: map[string]any{
						"k8s.pod.uid":               "01234567-89ab-cdef-0123-456789abcdef",
						"k8s.container.name":        "mycontainer",
						"datadog.container.is_init": true,
					},
				},
			}),
			outResourceAttributes: []map[string]any{
				{
					"global":                    "tag",
					"k8s.pod.uid":               "01234567-89ab-cdef-0123-456789abcdef",
					"k8s.container.name":        "mycontainer",
					"datadog.container.is_init": true,
					"container.id":              "test",
					"container":                 "id",
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
				Cardinality: types.LowCardinality,
			}
			tc := testutil.NewTestTaggerClient()
			tc.TagMap["container_id://test"] = []string{"container:id"}
			tc.TagMap["deployment://namespace/deployment"] = []string{"deployment:name"}
			tc.TagMap[types.NewEntityID("internal", "global-entity-id").String()] = []string{"global:tag"}
			tc.ContainerIDMap["pid:12345"] = "test"
			tc.ContainerIDMap["inode:12345"] = "test"
			tc.ContainerIDMap["pod:01234567-89ab-cdef-0123-456789abcdef,name:mycontainer,init:true"] = "test"

			factory := NewFactoryForAgent(tc, func(_ context.Context) (string, error) {
				return "test-host", nil
			})
			flp, err := factory.CreateLogs(
				context.Background(),
				processortest.NewNopSettings(Type),
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
