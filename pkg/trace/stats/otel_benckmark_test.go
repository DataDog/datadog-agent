// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.17.0"
	"go.opentelemetry.io/otel/metric/noop"
)

func BenchmarkOTelContainerTags(b *testing.B) {
	start := time.Now().Add(-1 * time.Second)
	end := time.Now()
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(b, err)

	traces := ptrace.NewTraces()
	rspan := traces.ResourceSpans().AppendEmpty()
	res := rspan.Resource()
	for k, v := range map[string]string{
		semconv.AttributeServiceName:           "svc",
		semconv.AttributeDeploymentEnvironment: "tracer_env",
		semconv.AttributeContainerID:           "test_cid",
		semconv.AttributeK8SClusterName:        "test_cluster",
		"az":                                   "my-az",
	} {
		res.Attributes().PutStr(k, v)
	}
	sspan := rspan.ScopeSpans().AppendEmpty()
	span := sspan.Spans().AppendEmpty()
	span.SetTraceID(testTraceID)
	span.SetSpanID(testSpanID1)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
	span.SetName("span_name")
	span.SetKind(ptrace.SpanKindClient)

	conf := config.New()
	conf.Hostname = "agent_host"
	conf.DefaultEnv = "agent_env"
	conf.Features["enable_cid_stats"] = struct{}{}
	conf.OTLPReceiver.AttributesTranslator = attributesTranslator

	concentrator := NewTestConcentratorWithCfg(time.Now(), conf)
	containerTagKeys := []string{"az", semconv.AttributeContainerID, semconv.AttributeK8SClusterName}
	expected := []string{"az:my-az", "container_id:test_cid", "kube_cluster_name:test_cluster"}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		inputs := OTLPTracesToConcentratorInputs(traces, conf, containerTagKeys)
		assert.Len(b, inputs, 1)
		input := inputs[0]
		concentrator.Add(input)
		stats := concentrator.Flush(true)
		assert.Len(b, stats.Stats, 1)
		assert.Equal(b, stats.Stats[0].Tags, expected)
	}
}
