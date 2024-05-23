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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor/processortest"
	conventions "go.opentelemetry.io/collector/semconv/v1.21.0"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type metricNameTest struct {
	name                  string
	inMetrics             pmetric.Metrics
	outResourceAttributes []map[string]any
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
			name: "one tag with global",
			inMetrics: testResourceMetrics([]metricWithResource{{
				metricNames: inMetricNames,
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
			inMetrics: testResourceMetrics([]metricWithResource{{
				metricNames: inMetricNames,
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
			inMetrics: testResourceMetrics([]metricWithResource{
				{
					metricNames: inMetricNames,
					resourceAttributes: map[string]any{
						"container.id": "test",
					},
				},
				{
					metricNames: inMetricNames,
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
			fakeTagger.SetTags("container_id://test", "test", []string{"container:id"}, nil, nil, nil)
			fakeTagger.SetTags("deployment://namespace/deployment", "test", []string{"deployment:name"}, nil, nil, nil)
			fakeTagger.SetTags(collectors.GlobalEntityID, "test", []string{"global:tag"}, nil, nil, nil)
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
			for i, out := range test.outResourceAttributes {
				rms := next.AllMetrics()[0].ResourceMetrics().At(i)
				assert.NotNil(t, rms)
				assert.EqualValues(t, out, rms.Resource().Attributes().AsRaw())
			}
		})
	}
}

func TestEntityIDsFromAttributes(t *testing.T) {
	tests := []struct {
		name      string
		attrs     pcommon.Map
		entityIDs []string
	}{
		{
			name:      "none",
			attrs:     pcommon.NewMap(),
			entityIDs: []string{},
		},
		{
			name: "pod UID and container ID",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeContainerID: "container_id_goes_here",
					conventions.AttributeK8SPodUID:   "k8s_pod_uid_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{"container_id://container_id_goes_here", "kubernetes_pod_uid://k8s_pod_uid_goes_here"},
		},
		{
			name: "container image ID",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeContainerImageID: "docker.io/foo@sha256:sha_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{"container_image_metadata://sha256:sha_goes_here"},
		},
		{
			name: "ecs task arn",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeAWSECSTaskARN: "ecs_task_arn_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{"ecs_task://ecs_task_arn_goes_here"},
		},
		{
			name: "only deployment name without namespace",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeK8SDeploymentName: "k8s_deployment_name_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{},
		},
		{
			name: "deployment name and namespace",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeK8SDeploymentName: "k8s_deployment_name_goes_here",
					conventions.AttributeK8SNamespaceName:  "k8s_namespace_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{"deployment://k8s_namespace_goes_here/k8s_deployment_name_goes_here", "namespace://k8s_namespace_goes_here"},
		},
		{
			name: "only namespace name",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeK8SNamespaceName: "k8s_namespace_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{"namespace://k8s_namespace_goes_here"},
		},
		{
			name: "only node UID",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeK8SNodeUID: "k8s_node_uid_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{"kubernetes_node_uid://k8s_node_uid_goes_here"},
		},
		{
			name: "only process pid",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					conventions.AttributeProcessPID: "process_pid_goes_here",
				})
				return attributes
			}(),
			entityIDs: []string{"process://process_pid_goes_here"},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			entityIDs := entityIDsFromAttributes(testInstance.attrs)
			assert.Equal(t, testInstance.entityIDs, entityIDs)
		})
	}
}
