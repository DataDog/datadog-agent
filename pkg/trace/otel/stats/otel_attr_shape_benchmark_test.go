// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	corestats "github.com/DataDog/datadog-agent/pkg/trace/stats"
)

// attrShape describes the attribute layout of a synthetic OTLP payload used to
// benchmark the OTLP -> concentrator input conversion. The conversion performs
// many per-span attribute lookups (env, hostname, version, container ID, peer
// and primary tags), and pcommon.Map lookups are linear scans, so the number of
// resource/span attributes and configured tag keys dominates its cost.
type attrShape struct {
	name string
	// resourceAttrs is the number of resource attributes (realistic keys first,
	// then synthetic filler keys if more are requested).
	resourceAttrs int
	// peerTags enables peer tag aggregation (the default ~44 precursor keys).
	peerTags bool
	// primaryTagKeys are span-derived primary tag keys passed to the conversion.
	primaryTagKeys []string
}

// realisticResourceAttrs approximates what the k8sattributes / resourcedetection
// processors and the OTel SDK attach to every resource.
var realisticResourceAttrs = []string{
	"service.name", "service.version", "service.namespace", "service.instance.id",
	"deployment.environment.name", "telemetry.sdk.name", "telemetry.sdk.language",
	"telemetry.sdk.version", "telemetry.distro.name", "telemetry.distro.version",
	"host.name", "host.arch", "host.id", "host.type", "host.image.id",
	"os.type", "os.description", "process.pid", "process.executable.name",
	"process.runtime.name", "process.runtime.version", "process.runtime.description",
	"process.command_args", "process.owner", "cloud.provider", "cloud.platform",
	"cloud.region", "cloud.availability_zone", "cloud.account.id",
	"k8s.cluster.name", "k8s.namespace.name", "k8s.pod.name", "k8s.pod.uid",
	"k8s.pod.start_time", "k8s.node.name", "k8s.deployment.name",
	"k8s.replicaset.name", "k8s.replicaset.uid", "k8s.container.name",
	"k8s.container.restart_count", "container.id", "container.image.name",
	"container.image.tag",
}

const (
	attrShapeResources        = 10
	attrShapeSpansPerResource = 50
)

func buildAttrShapeTraces(shape attrShape) ptrace.Traces {
	start := pcommon.NewTimestampFromTime(time.Now().Add(-time.Second))
	end := pcommon.NewTimestampFromTime(time.Now())
	traces := ptrace.NewTraces()
	for r := 0; r < attrShapeResources; r++ {
		rs := traces.ResourceSpans().AppendEmpty()
		rattrs := rs.Resource().Attributes()
		rattrs.EnsureCapacity(shape.resourceAttrs)
		for i := 0; i < shape.resourceAttrs; i++ {
			var key string
			if i < len(realisticResourceAttrs) {
				key = realisticResourceAttrs[i]
			} else {
				key = fmt.Sprintf("k8s.pod.label.app.example.com/filler-%02d", i)
			}
			rattrs.PutStr(key, fmt.Sprintf("value-%d-%d", r, i))
		}
		rattrs.PutStr("service.name", fmt.Sprintf("service-%d", r))

		// One trace per resource: a root span followed by a chain of children.
		// The DD trace ID is the low 8 bytes, so vary those per trace.
		traceID := pcommon.TraceID{0: 0xab, 15: byte(r + 1)}
		spans := rs.ScopeSpans().AppendEmpty().Spans()
		for s := 0; s < attrShapeSpansPerResource; s++ {
			span := spans.AppendEmpty()
			span.SetTraceID(traceID)
			span.SetSpanID(pcommon.SpanID{byte(r), byte(s), 1})
			if s > 0 {
				span.SetParentSpanID(pcommon.SpanID{byte(r), byte(s - 1), 1})
			}
			span.SetStartTimestamp(start)
			span.SetEndTimestamp(end)
			attrs := span.Attributes()
			// Mix of span kinds with semconv-shaped attributes. Most spans carry
			// no peer tag precursor at all; client spans carry one or two.
			switch s % 5 {
			case 0:
				span.SetKind(ptrace.SpanKindServer)
				span.SetName("GET /api/v1/items/{id}")
				attrs.PutStr("http.request.method", "GET")
				attrs.PutStr("http.route", "/api/v1/items/{id}")
				attrs.PutInt("http.response.status_code", 200)
				attrs.PutStr("url.scheme", "https")
				attrs.PutStr("network.protocol.version", "1.1")
				attrs.PutStr("user_agent.original", "Mozilla/5.0")
				attrs.PutStr("client.address", "10.0.0.1")
			case 1:
				span.SetKind(ptrace.SpanKindClient)
				span.SetName("SELECT items")
				attrs.PutStr("db.system.name", "postgresql")
				attrs.PutStr("db.namespace", "inventory")
				attrs.PutStr("db.operation.name", "SELECT")
				attrs.PutStr("db.collection.name", "items")
				attrs.PutStr("server.address", "db.internal")
				attrs.PutInt("server.port", 5432)
			case 2:
				span.SetKind(ptrace.SpanKindClient)
				span.SetName("GET")
				attrs.PutStr("http.request.method", "GET")
				attrs.PutStr("url.full", "https://payments.internal/charge")
				attrs.PutInt("http.response.status_code", 200)
				attrs.PutStr("server.address", "payments.internal")
				attrs.PutInt("server.port", 443)
				attrs.PutStr("peer.service", "payments")
			default:
				span.SetKind(ptrace.SpanKindInternal)
				span.SetName("process-item")
				attrs.PutStr("code.function.name", "processItem")
				attrs.PutStr("code.file.path", "/app/internal/items.go")
				attrs.PutInt("code.line.number", 123)
				attrs.PutStr("thread.name", "worker-1")
			}
			for _, k := range shape.primaryTagKeys {
				if s%2 == 0 {
					attrs.PutStr(k, "primary-value")
				}
			}
		}
	}
	return traces
}

// BenchmarkOTelStatsAttrShapes measures OTLPTracesToConcentratorInputs over
// payloads of 500 spans with varying attribute shapes.
func BenchmarkOTelStatsAttrShapes(b *testing.B) {
	shapes := []attrShape{
		// SDK + k8sattributes/resourcedetection: the common collector deployment.
		{name: "k8s_resource_43/peer_tags", resourceAttrs: 43, peerTags: true},
		// Bare SDK exporting straight to the agent/collector.
		{name: "small_resource_3/peer_tags", resourceAttrs: 3, peerTags: true},
		// Very wide resources (many pod labels promoted to resource attributes).
		{name: "wide_resource_120/peer_tags", resourceAttrs: 120, peerTags: true},
		// Peer tags disabled, few primary tag keys: per-span cost now scales with
		// attribute count instead of key count.
		{name: "k8s_resource_43/primary_only_2", resourceAttrs: 43, primaryTagKeys: []string{"customer.tier", "region.cell"}},
		// Both selections populated: two full passes over attributes.
		{name: "k8s_resource_43/peer_and_primary_2", resourceAttrs: 43, peerTags: true, primaryTagKeys: []string{"customer.tier", "region.cell"}},
		// Nothing configured: should be unaffected.
		{name: "k8s_resource_43/no_tags", resourceAttrs: 43},
	}
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	if err != nil {
		b.Fatal(err)
	}
	for _, shape := range shapes {
		b.Run(shape.name, func(b *testing.B) {
			conf := config.New()
			conf.Hostname = "agent_host"
			conf.DefaultEnv = "agent_env"
			conf.OTLPReceiver.AttributesTranslator = attributesTranslator
			conf.PeerTagsAggregation = shape.peerTags
			peerTagKeys := conf.ConfiguredPeerTags()
			traces := buildAttrShapeTraces(shape)
			validateAttrShapeInputs(b, OTLPTracesToConcentratorInputs(traces, conf, nil, peerTagKeys, shape.primaryTagKeys))
			b.ReportAllocs()
			for b.Loop() {
				// One input per trace: guards against payloads whose trace IDs collide.
				if inputs := OTLPTracesToConcentratorInputs(traces, conf, nil, peerTagKeys, shape.primaryTagKeys); len(inputs) != attrShapeResources {
					b.Fatalf("expected %d concentrator inputs, got %d", attrShapeResources, len(inputs))
				}
			}
		})
	}
}

// validateAttrShapeInputs checks the payload converts into one well-formed
// trace per resource, so the benchmark doesn't measure ID-collision behavior.
func validateAttrShapeInputs(b *testing.B, inputs []corestats.Input) {
	b.Helper()
	if len(inputs) != attrShapeResources {
		b.Fatalf("expected %d concentrator inputs, got %d", attrShapeResources, len(inputs))
	}
	seen := make(map[uint64]struct{}, len(inputs))
	for _, input := range inputs {
		spans := input.Traces[0].TraceChunk.Spans
		if len(spans) != attrShapeSpansPerResource {
			b.Fatalf("expected %d spans per trace, got %d", attrShapeSpansPerResource, len(spans))
		}
		traceID := spans[0].TraceID
		if traceID == 0 {
			b.Fatal("converted trace ID is zero")
		}
		for _, span := range spans {
			if span.TraceID != traceID {
				b.Fatalf("trace mixes IDs %d and %d", traceID, span.TraceID)
			}
		}
		if _, dup := seen[traceID]; dup {
			b.Fatalf("trace ID %d appears in more than one input", traceID)
		}
		seen[traceID] = struct{}{}
	}
}
