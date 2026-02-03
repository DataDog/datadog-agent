// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func BenchmarkOTelStatsWithoutObfuscation(b *testing.B) {
	benchmarkOTelObfuscation(b, false)
}

func BenchmarkOTelStatsWithObfuscation(b *testing.B) {
	benchmarkOTelObfuscation(b, true)
}

func benchmarkOTelObfuscation(b *testing.B, enableObfuscation bool) {
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
		string(semconv.ServiceNameKey):           "svc",
		string(semconv.DeploymentEnvironmentKey): "tracer_env",
		string(semconv.DBSystemKey):              "mysql",
		string(semconv.DBStatementKey): `
		SELECT
    	u.id,
			u.name,
			u.email,
			o.order_id,
			o.total_amount,
			p.product_name,
			p.price
		FROM
				users u
		JOIN
				orders o ON u.id = o.user_id
		JOIN
				order_items oi ON o.order_id = oi.order_id
		JOIN
				products p ON oi.product_id = p.product_id
		WHERE
				u.status = 'active'
				AND o.order_date BETWEEN '2023-01-01' AND '2023-12-31'
				AND p.category IN ('electronics', 'books')
		GROUP BY
				u.id,
				u.name,
				u.email,
				o.order_id,
				o.total_amount,
				p.product_name,
				p.price
		ORDER BY
				o.order_date DESC,
				p.price ASC
		LIMIT 100;
		`,
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
	conf.Obfuscation.Redis.Enabled = true
	conf.OTLPReceiver.AttributesTranslator = attributesTranslator

	concentrator := newTestConcentratorWithCfg(time.Now(), conf)

	var obfuscator *obfuscate.Obfuscator
	if enableObfuscation {
		obfuscator = newTestObfuscator(conf)
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		inputs := OTLPTracesToConcentratorInputsWithObfuscation(traces, conf, nil, nil, obfuscator)
		assert.Len(b, inputs, 1)
		input := inputs[0]
		concentrator.Add(input)
		stats := concentrator.Flush(true)
		assert.Len(b, stats.Stats, 1)
	}
}

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
		string(semconv.ServiceNameKey):           "svc",
		string(semconv.DeploymentEnvironmentKey): "tracer_env",
		string(semconv.ContainerIDKey):           "test_cid",
		string(semconv.K8SClusterNameKey):        "test_cluster",
		"az":                                     "my-az",
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
	conf.OTLPReceiver.AttributesTranslator = attributesTranslator

	concentrator := newTestConcentratorWithCfg(time.Now(), conf)
	containerTagKeys := []string{"az", string(semconv.ContainerIDKey), string(semconv.K8SClusterNameKey)}
	expected := []string{"az:my-az", "container_id:test_cid", "kube_cluster_name:test_cluster"}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		inputs := OTLPTracesToConcentratorInputs(traces, conf, containerTagKeys, nil)
		assert.Len(b, inputs, 1)
		input := inputs[0]
		concentrator.Add(input)
		stats := concentrator.Flush(true)
		assert.Len(b, stats.Stats, 1)
		assert.Equal(b, stats.Stats[0].Tags, expected)
	}
}

func BenchmarkOTelPeerTags(b *testing.B) {
	benchmarkOTelPeerTags(b, true)
}

// This simulates the benchmark of OTLPTracesToConcentratorInputs before https://github.com/DataDog/datadog-agent/pull/28908
func BenchmarkOTelPeerTags_IncludeConfiguredPeerTags(b *testing.B) {
	benchmarkOTelPeerTags(b, false)
}

func benchmarkOTelPeerTags(b *testing.B, initOnce bool) {
	start := time.Now().Add(-1 * time.Second)
	end := time.Now()
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(b, err)

	traces := ptrace.NewTraces()
	rspan := traces.ResourceSpans().AppendEmpty()
	sspan := rspan.ScopeSpans().AppendEmpty()
	span := sspan.Spans().AppendEmpty()
	span.SetTraceID(testTraceID)
	span.SetSpanID(testSpanID1)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
	span.SetName("span_name")
	span.SetKind(ptrace.SpanKindClient)
	span.Attributes().PutStr("peer.service", "my_peer_svc")
	span.Attributes().PutStr("rpc.service", "my_rpc_svc")
	span.Attributes().PutStr("net.peer.name", "my_net_peer")

	conf := config.New()
	conf.Hostname = "agent_host"
	conf.DefaultEnv = "agent_env"
	conf.OTLPReceiver.AttributesTranslator = attributesTranslator
	conf.PeerTagsAggregation = true
	var peerTagKeys []string
	if initOnce {
		peerTagKeys = conf.ConfiguredPeerTags()
	}

	concentrator := newTestConcentratorWithCfg(time.Now(), conf)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		if !initOnce {
			peerTagKeys = conf.ConfiguredPeerTags()
		}
		inputs := OTLPTracesToConcentratorInputs(traces, conf, nil, peerTagKeys)
		assert.Len(b, inputs, 1)
		input := inputs[0]
		concentrator.Add(input)
		stats := concentrator.Flush(true)
		assert.Len(b, stats.Stats, 1)
	}
}
