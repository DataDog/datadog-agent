// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file contains V2 versions of the OTLP stats tests.
// These tests verify the new translation logic when the disable_otlp_translations_v2 feature flag is NOT set.
// The original tests in otel_util_test.go verify backward compatibility with the old translation logic.

package stats

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestProcessOTLPTracesV2_OTelCompliantTranslation(t *testing.T) {
	start := time.Now().Add(-1 * time.Second)
	end := time.Now()
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(t, err)

	agentEnv := "agent_env"
	agentHost := "agent_host"
	traceIDEmpty := pcommon.NewTraceIDEmpty()
	spanIDEmpty := pcommon.NewSpanIDEmpty()
	parentID := pcommon.SpanID(testSpanID2)

	for _, tt := range []struct {
		name                       string
		traceID                    *pcommon.TraceID
		spanID                     *pcommon.SpanID
		parentSpanID               *pcommon.SpanID
		spanName                   string
		rattrs                     map[string]string
		sattrs                     map[string]any
		spanKind                   ptrace.SpanKind
		libname                    string
		spanNameAsResourceName     bool
		spanNameRemappings         map[string]string
		ignoreRes                  []string
		peerTagsAggr               bool
		legacyTopLevel             bool
		ctagKeys                   []string
		expected                   *pb.StatsPayload
		enableObfuscation          bool
		ignoreMissingDatadogFields bool
	}{
		{
			name:     "empty trace id",
			traceID:  &traceIDEmpty,
			expected: &pb.StatsPayload{AgentEnv: agentEnv, AgentHostname: agentHost},
		},
		{
			name:     "empty span id",
			spanID:   &spanIDEmpty,
			expected: &pb.StatsPayload{AgentEnv: agentEnv, AgentHostname: agentHost},
		},
		{
			name:     "span with no attributes, everything uses default",
			expected: createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.unspecified", "custom", "unspecified", "", agentHost, agentEnv, "", "", nil, nil, true, false, 0),
		},
		{
			name:         "non root span with kind internal does not get stats with new top level rules",
			parentSpanID: &parentID,
			spanKind:     ptrace.SpanKindInternal,
			expected:     &pb.StatsPayload{AgentEnv: agentEnv, AgentHostname: agentHost},
		},
		{
			name:         "non root span with kind internal and _dd.measured key gets stats with new top level rules",
			parentSpanID: &parentID,
			spanKind:     ptrace.SpanKindInternal,
			sattrs:       map[string]any{"_dd.measured": int64(1)},
			expected:     createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.internal", "custom", "internal", "", agentHost, agentEnv, "", "", nil, nil, false, true, 0),
		},
		{
			name:         "non root span with kind client gets stats with new top level rules",
			parentSpanID: &parentID,
			spanKind:     ptrace.SpanKindClient,
			expected:     createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.client", "http", "client", "", agentHost, agentEnv, "", "", nil, nil, false, true, 0),
		},
		{
			name:           "non root span with kind internal does get stats with legacy top level rules",
			parentSpanID:   &parentID,
			spanKind:       ptrace.SpanKindInternal,
			legacyTopLevel: true,
			expected:       createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.internal", "custom", "internal", "", agentHost, agentEnv, "", "", nil, nil, false, false, 0),
		},
		{
			name:     "span with name, service, instrumentation scope and span kind",
			spanName: "spanname",
			rattrs:   map[string]string{"service.name": "svc"},
			spanKind: ptrace.SpanKindServer,
			libname:  "spring",
			expected: createStatsPayload(agentEnv, agentHost, "svc", "spring.server", "web", "server", "spanname", agentHost, agentEnv, "", "", nil, nil, true, false, 0),
		},
		{
			name:     "span with operation name, resource name and env attributes",
			spanName: "spanname2",
			rattrs:   map[string]string{"service.name": "svc", string(semconv.DeploymentEnvironmentKey): "tracer-env"},
			sattrs:   map[string]any{"operation.name": "op", "resource.name": "res"},
			spanKind: ptrace.SpanKindClient,
			libname:  "spring",
			expected: createStatsPayload(agentEnv, agentHost, "svc", "op", "http", "client", "res", agentHost, "tracer-env", "", "", nil, nil, true, false, 0),
		},
		{
			name:     "new env convention",
			spanName: "spanname2",
			rattrs:   map[string]string{"service.name": "svc", "deployment.environment.name": "new-env"},
			sattrs:   map[string]any{"operation.name": "op", "resource.name": "res"},
			spanKind: ptrace.SpanKindClient,
			libname:  "spring",
			expected: createStatsPayload(agentEnv, agentHost, "svc", "op", "http", "client", "res", agentHost, "new-env", "", "", nil, nil, true, false, 0),
		},
		{
			name:                   "span operation name from span name with db attribute, peerTagsAggr not enabled",
			spanName:               "spanname3",
			rattrs:                 map[string]string{"service.name": "svc", "host.name": "test-host", "db.system": "redis"},
			spanKind:               ptrace.SpanKindClient,
			spanNameAsResourceName: true,
			expected:               createStatsPayload(agentEnv, agentHost, "svc", "spanname3", "cache", "client", "spanname3", "test-host", agentEnv, "", "", nil, nil, true, false, 0),
		},
		{
			name:     "with container tags",
			spanName: "spanname4",
			rattrs:   map[string]string{"service.name": "svc", "db.system": "spanner", string(semconv.ContainerIDKey): "test_cid"},
			ctagKeys: []string{string(semconv.ContainerIDKey)},
			spanKind: ptrace.SpanKindClient,
			expected: createStatsPayload(agentEnv, agentHost, "svc", "opentelemetry.client", "db", "client", "spanname4", agentHost, agentEnv, "test_cid", "", []string{"container_id:test_cid", "env:test_env"}, nil, true, false, 0),
		},
		{
			name:               "operation name remapping and resource from http",
			spanName:           "spanname5",
			spanKind:           ptrace.SpanKindInternal,
			sattrs:             map[string]any{string(semconv.HTTPMethodKey): "GET", string(semconv.HTTPRouteKey): "/home"},
			spanNameRemappings: map[string]string{"opentelemetry.internal": "internal_op"},
			expected:           createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "internal_op", "custom", "internal", "GET /home", agentHost, agentEnv, "", "", nil, nil, true, false, 0),
		},
		{
			name:         "with peer tags and peerTagsAggr enabled",
			spanName:     "spanname6",
			spanKind:     ptrace.SpanKindClient,
			peerTagsAggr: true,
			rattrs:       map[string]string{"service.name": "svc", string(semconv.DeploymentEnvironmentKey): "tracer-env", "datadog.host.name": "dd-host"},
			sattrs:       map[string]any{"operation.name": "op", string(semconv.RPCMethodKey): "call", string(semconv.RPCServiceKey): "rpc_service"},
			expected:     createStatsPayload(agentEnv, agentHost, "svc", "op", "http", "client", "call rpc_service", "dd-host", "tracer-env", "", "", nil, []string{"rpc.service:rpc_service"}, true, false, 0),
		},

		{
			name:      "ignore resource name",
			spanName:  "spanname7",
			spanKind:  ptrace.SpanKindClient,
			sattrs:    map[string]any{"http.request.method": "GET", string(semconv.HTTPRouteKey): "/home"},
			ignoreRes: []string{"GET /home"},
			expected:  &pb.StatsPayload{AgentEnv: agentEnv, AgentHostname: agentHost},
		},
		{
			name:              "obfuscate sql span",
			spanName:          "spanname8",
			spanKind:          ptrace.SpanKindClient,
			rattrs:            map[string]string{"service.name": "svc", string(semconv.DBSystemKey): semconv.DBSystemMSSQL.Value.AsString(), string(semconv.DBStatementKey): "SELECT username FROM users WHERE id = 12345"},
			enableObfuscation: true,
			expected:          createStatsPayload(agentEnv, agentHost, "svc", "mssql.query", "sql", "client", "SELECT username FROM users WHERE id = ?", agentHost, agentEnv, "", "", nil, nil, true, false, 0),
		},
		{
			name:              "obfuscated redis span",
			spanName:          "spanname9",
			rattrs:            map[string]string{"service.name": "svc", "host.name": "test-host", "db.system": "redis", "db.statement": "SET key value"},
			spanKind:          ptrace.SpanKindClient,
			enableObfuscation: true,
			expected:          createStatsPayload(agentEnv, agentHost, "svc", "redis.query", "redis", "client", "SET", "test-host", agentEnv, "", "", nil, nil, true, false, 0),
		},
		{
			name:     "span vs resource attribute precedence",
			spanName: "/path",
			rattrs: map[string]string{
				"service.name":           "res-service",
				"deployment.environment": "res-env",
				"operation.name":         "res-op",
				"resource.name":          "res-res",
				"span.type":              "res-type",
				"http.status_code":       "res-status",
				"version":                "res-version",
			},
			sattrs: map[string]any{
				"service.name":           "span-service",
				"deployment.environment": "span-env",
				"operation.name":         "span-op",
				"resource.name":          "span-res",
				"span.type":              "span-type",
				"http.status_code":       "span-status",
				"service.version":        "span-service-version",
			},
			spanKind: ptrace.SpanKindServer,
			libname:  "ddtracer",
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"span-service",
				"span-op",
				"span-type",
				"server",
				"span-res",
				agentHost,
				"span-env",
				"",
				"span-service-version",
				nil,
				nil,
				true,
				false,
				0,
			),
		},
		{
			name:     "datadog.* resource attributes take precedence over standard attributes",
			spanName: "/path",
			rattrs: map[string]string{
				"service.name":                             "svc",
				"deployment.environment":                   "env",
				"service.version":                          "v1",
				"host.name":                                "host1",
				"container.id":                             "cid1",
				attributes.APMConventionKeys.Env():         "dd-env",
				attributes.APMConventionKeys.Service():     "dd-svc",
				attributes.APMConventionKeys.Version():     "dd-v2",
				attributes.APMConventionKeys.Host():        "dd-host",
				attributes.APMConventionKeys.ContainerID(): "dd-cid",
			},
			sattrs:   map[string]any{},
			spanKind: ptrace.SpanKindServer,
			libname:  "ddtracer",
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"dd-svc",
				"server.request",
				"web",
				"server",
				"/path",
				"dd-host",
				"dd-env",
				"dd-cid",
				"dd-v2",
				nil,
				nil,
				true,
				false,
				0,
			),
		},
		{
			name:     "datadog.* resource attributes win when span attributes are empty",
			spanName: "test-name",
			rattrs: map[string]string{
				attributes.DDNamespaceKeys.Service():        "test-service",
				attributes.DDNamespaceKeys.OperationName():  "test-name",
				attributes.DDNamespaceKeys.ResourceName():   "test-resource",
				attributes.DDNamespaceKeys.SpanType():       "test-type",
				attributes.DDNamespaceKeys.Env():            "test-env",
				attributes.DDNamespaceKeys.Version():        "test-version",
				attributes.DDNamespaceKeys.SpanKind():       "test-kind",
				attributes.DDNamespaceKeys.HTTPStatusCode(): "404",
			},
			sattrs:   map[string]any{},
			spanKind: ptrace.SpanKindServer,
			libname:  "ddtracer",
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"test-service",
				"test-name",
				"test-type",
				"test-kind",
				"test-resource",
				agentHost,
				"test-env",
				"",
				"test-version",
				nil,
				nil,
				true,
				false,
				404,
			),
		},
		{
			name:     "datadog.* resource attributes win over standard span attributes",
			spanName: "span-name",
			rattrs: map[string]string{
				attributes.APMConventionKeys.Service():        "test-service",
				attributes.APMConventionKeys.OperationName():  "test-name",
				attributes.APMConventionKeys.ResourceName():   "test-resource",
				attributes.APMConventionKeys.SpanType():       "test-type",
				attributes.APMConventionKeys.Env():            "test-env",
				attributes.APMConventionKeys.Version():        "test-version",
				attributes.APMConventionKeys.SpanKind():       "test-kind",
				attributes.APMConventionKeys.HTTPStatusCode(): "404",
			},
			sattrs: map[string]any{
				"service.name":           "span-service",
				"operation.name":         "span-op",
				"resource.name":          "span-resource",
				"span.type":              "span-type",
				"deployment.environment": "span-env",
				"service.version":        "span-version",
				"span.kind":              "server",
				"http.status_code":       "200",
			},
			spanKind: ptrace.SpanKindServer,
			libname:  "ddtracer",
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"test-service",
				"test-name",
				"test-type",
				"test-kind",
				"test-resource",
				agentHost,
				"test-env",
				"",
				"test-version",
				nil,
				nil,
				true,
				false,
				404,
			),
		},
		{
			name:     "datadog.* span attributes win over all others",
			spanName: "dd-span-name",
			rattrs: map[string]string{
				attributes.APMConventionKeys.Service():        "test-service",
				attributes.APMConventionKeys.OperationName():  "test-name",
				attributes.APMConventionKeys.ResourceName():   "test-resource",
				attributes.APMConventionKeys.SpanType():       "test-type",
				attributes.APMConventionKeys.Env():            "test-env",
				attributes.APMConventionKeys.Version():        "test-version",
				attributes.APMConventionKeys.SpanKind():       "test-kind",
				attributes.APMConventionKeys.HTTPStatusCode(): "404",
			},
			sattrs: map[string]any{
				attributes.DDNamespaceKeys.Service():        "dd-span-service",
				attributes.DDNamespaceKeys.OperationName():  "dd-span-name",
				attributes.DDNamespaceKeys.ResourceName():   "dd-span-resource",
				attributes.DDNamespaceKeys.SpanType():       "dd-span-type",
				attributes.DDNamespaceKeys.Env():            "dd-span-env",
				attributes.DDNamespaceKeys.Version():        "dd-span-version",
				attributes.DDNamespaceKeys.SpanKind():       "dd-span-kind",
				attributes.DDNamespaceKeys.HTTPStatusCode(): "500",
			},
			spanKind: ptrace.SpanKindServer,
			libname:  "ddtracer",
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"dd-span-service",
				"dd-span-name",
				"dd-span-type",
				"dd-span-kind",
				"dd-span-resource",
				agentHost,
				"dd-span-env",
				"",
				"dd-span-version",
				nil,
				nil,
				true,
				false,
				500,
			),
		},
		{
			name:     "missing datadog.service in resource, ignoreMissingDatadogFields=false",
			spanName: "test-name",
			rattrs: map[string]string{
				attributes.APMConventionKeys.OperationName(): "test-name",
			},
			sattrs:                     map[string]any{},
			spanKind:                   ptrace.SpanKindServer,
			libname:                    "ddtracer",
			ignoreMissingDatadogFields: false,
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"otlpresourcenoservicename",
				"test-name",
				"web",
				"server",
				"test-name",
				agentHost,
				agentEnv,
				"",
				"",
				nil,
				nil,
				true,
				false,
				0,
			),
		},
		{
			name:     "missing datadog.service in span, ignoreMissingDatadogFields=false",
			spanName: "test-name",
			rattrs:   map[string]string{},
			sattrs: map[string]any{
				attributes.APMConventionKeys.OperationName(): "test-name",
			},
			spanKind:                   ptrace.SpanKindServer,
			libname:                    "ddtracer",
			ignoreMissingDatadogFields: false,
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"otlpresourcenoservicename",
				"test-name",
				"web",
				"server",
				"test-name",
				agentHost,
				agentEnv,
				"",
				"",
				nil,
				nil,
				true,
				false,
				0,
			),
		},
		{
			name:     "ignoreMissingDatadogFields=true, only standard conventions, output defaults",
			spanName: "spanname",
			rattrs: map[string]string{
				"service.name":           "svc",
				"operation.name":         "op",
				"resource.name":          "res",
				"span.type":              "type",
				"deployment.environment": "env",
				"service.version":        "v1",
			},
			sattrs:                     map[string]any{},
			spanKind:                   ptrace.SpanKindServer,
			libname:                    "ddtracer",
			ignoreMissingDatadogFields: true,
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"otlpresourcenoservicename",
				"Server",
				"custom",
				"",
				"spanname",
				agentHost,
				agentEnv,
				"",
				"",
				nil,
				nil,
				true,
				false,
				0,
			),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			rspan := traces.ResourceSpans().AppendEmpty()
			res := rspan.Resource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			sspan := rspan.ScopeSpans().AppendEmpty()
			sspan.Scope().SetName(tt.libname)
			span := sspan.Spans().AppendEmpty()
			if tt.traceID != nil {
				span.SetTraceID(*tt.traceID)
			} else {
				span.SetTraceID(testTraceID)
			}
			if tt.spanID != nil {
				span.SetSpanID(*tt.spanID)
			} else {
				span.SetSpanID(testSpanID1)
			}
			if tt.parentSpanID != nil {
				span.SetParentSpanID(*tt.parentSpanID)
			}
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
			span.SetName(tt.spanName)
			span.SetKind(tt.spanKind)
			for k, v := range tt.sattrs {
				switch typ := v.(type) {
				case int64:
					span.Attributes().PutInt(k, v.(int64))
				case string:
					span.Attributes().PutStr(k, v.(string))
				default:
					t.Fatal("unhandled attribute value type ", typ)
				}
			}

			conf := config.New()
			conf.Hostname = agentHost
			conf.DefaultEnv = agentEnv
			conf.Features = map[string]struct{}{"enable_otel_compliant_translation": {}}
			conf.PeerTagsAggregation = tt.peerTagsAggr
			conf.OTLPReceiver.AttributesTranslator = attributesTranslator
			conf.OTLPReceiver.SpanNameAsResourceName = tt.spanNameAsResourceName
			conf.OTLPReceiver.SpanNameRemappings = tt.spanNameRemappings
			conf.Ignore["resource"] = tt.ignoreRes
			if !tt.legacyTopLevel {
				conf.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}
			}
			conf.ContainerTags = func(cid string) ([]string, error) {
				if cid == "test_cid" {
					return []string{"env:test_env"}, nil
				}
				return nil, nil
			}
			conf.OTLPReceiver.IgnoreMissingDatadogFields = tt.ignoreMissingDatadogFields

			concentrator := NewTestConcentratorWithCfg(time.Now(), conf)
			var obfuscator *obfuscate.Obfuscator
			var inputs []Input
			if tt.enableObfuscation {
				obfuscator = newTestObfuscator(conf)
				inputs = OTLPTracesToConcentratorInputsWithObfuscation(traces, conf, tt.ctagKeys, conf.ConfiguredPeerTags(), obfuscator)
			} else {
				inputs = OTLPTracesToConcentratorInputs(traces, conf, tt.ctagKeys, conf.ConfiguredPeerTags())
			}
			for _, input := range inputs {
				concentrator.Add(input)
			}

			stats := concentrator.Flush(true)
			if diff := cmp.Diff(
				tt.expected,
				stats,
				protocmp.Transform(),
				protocmp.IgnoreFields(&pb.ClientStatsBucket{}, "start", "duration"),
				protocmp.IgnoreFields(&pb.ClientGroupedStats{}, "duration", "okSummary", "errorSummary")); diff != "" {
				t.Errorf("Diff between APM stats -want +got:\n%v", diff)
			}
		})
	}
}

func TestProcessOTLPTraces_MutliSpanInOneResAndOp_OTelCompliantTranslation(t *testing.T) {
	start := time.Now().Add(-1 * time.Second)
	end := time.Now()
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(t, err)

	agentEnv := "agent_env"
	agentHost := "agent_host"

	traces := ptrace.NewTraces()
	rspan := traces.ResourceSpans().AppendEmpty()
	res := rspan.Resource()
	res.Attributes().PutStr("service.name", "svc")
	res.Attributes().PutStr(string(semconv.DeploymentEnvironmentKey), "tracer-env")
	res.Attributes().PutStr("datadog.host.name", "dd-host")

	sspan := rspan.ScopeSpans().AppendEmpty()
	span1 := sspan.Spans().AppendEmpty()
	span1.SetTraceID(testTraceID)
	span1.SetSpanID(testSpanID1)
	span1.SetName("span1")
	span1.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span1.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
	span1.SetKind(ptrace.SpanKindClient)
	span1.Attributes().PutStr(string(semconv.HTTPMethodKey), "GET")
	span1.Attributes().PutStr(string(semconv.HTTPRouteKey), "/home")

	span2 := sspan.Spans().AppendEmpty()
	span2.SetTraceID(testTraceID)
	span2.SetSpanID(testSpanID2)
	span2.SetName("span2")
	span2.Attributes().PutStr("resource.name", "GET /home")
	span2.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span2.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
	span2.SetKind(ptrace.SpanKindClient)

	conf := config.New()
	conf.Hostname = agentHost
	conf.DefaultEnv = agentEnv
	conf.OTLPReceiver.AttributesTranslator = attributesTranslator
	conf.Features["disable_operation_and_resource_name_logic_v2"] = struct{}{}

	concentrator := NewTestConcentratorWithCfg(time.Now(), conf)
	inputs := OTLPTracesToConcentratorInputs(traces, conf, nil, nil)
	for _, input := range inputs {
		concentrator.Add(input)
	}

	stats := concentrator.Flush(true)
	expected := &pb.StatsPayload{
		AgentEnv:      agentEnv,
		AgentHostname: agentHost,
		Stats: []*pb.ClientStatsPayload{{
			Hostname: "dd-host",
			Env:      "tracer-env",
			Stats: []*pb.ClientStatsBucket{{
				Stats: []*pb.ClientGroupedStats{
					{
						Service:      "svc",
						Name:         "opentelemetry.client",
						Resource:     "GET /home",
						Type:         "http",
						Hits:         2,
						TopLevelHits: 2,
						SpanKind:     "client",
						IsTraceRoot:  pb.Trilean_TRUE,
					},
				},
			}}}},
	}
	if diff := cmp.Diff(
		expected,
		stats,
		protocmp.Transform(),
		protocmp.IgnoreFields(&pb.ClientStatsBucket{}, "start", "duration"),
		protocmp.IgnoreFields(&pb.ClientGroupedStats{}, "duration", "okSummary", "errorSummary")); diff != "" {
		t.Errorf("Diff between APM stats -want +got:\n%v", diff)
	}
}
