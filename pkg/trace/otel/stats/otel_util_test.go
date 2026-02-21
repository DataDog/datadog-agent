// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
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
	corestats "github.com/DataDog/datadog-agent/pkg/trace/stats"
)

var (
	testTraceID = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	testSpanID1 = [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	testSpanID2 = [8]byte{2, 2, 3, 4, 5, 6, 7, 8}
)

func TestProcessOTLPTraces(t *testing.T) {
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
		name                             string
		traceID                          *pcommon.TraceID
		spanID                           *pcommon.SpanID
		parentSpanID                     *pcommon.SpanID
		spanName                         string
		rattrs                           map[string]string
		sattrs                           map[string]any
		spanKind                         ptrace.SpanKind
		libname                          string
		spanNameAsResourceName           bool
		spanNameRemappings               map[string]string
		ignoreRes                        []string
		peerTagsAggr                     bool
		legacyTopLevel                   bool
		ctagKeys                         []string
		expected                         *pb.StatsPayload
		enableObfuscation                bool
		enableReceiveResourceSpansV2     bool
		enableOperationAndResourceNameV2 bool
		enableContainerTagsV2            bool
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
			expected: createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.unspecified", "custom", "unspecified", "", agentHost, agentEnv, "", "", nil, nil, true, false),
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
			expected:     createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.internal", "custom", "internal", "", agentHost, agentEnv, "", "", nil, nil, false, true),
		},
		{
			name:         "non root span with kind client gets stats with new top level rules",
			parentSpanID: &parentID,
			spanKind:     ptrace.SpanKindClient,
			expected:     createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.client", "http", "client", "", agentHost, agentEnv, "", "", nil, nil, false, true),
		},
		{
			name:           "non root span with kind internal does get stats with legacy top level rules",
			parentSpanID:   &parentID,
			spanKind:       ptrace.SpanKindInternal,
			legacyTopLevel: true,
			expected:       createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "opentelemetry.internal", "custom", "internal", "", agentHost, agentEnv, "", "", nil, nil, false, false),
		},
		{
			name:     "span with name, service, instrumentation scope and span kind",
			spanName: "spanname",
			rattrs:   map[string]string{"service.name": "svc"},
			spanKind: ptrace.SpanKindServer,
			libname:  "spring",
			expected: createStatsPayload(agentEnv, agentHost, "svc", "spring.server", "web", "server", "spanname", agentHost, agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:     "span with operation name, resource name and env attributes",
			spanName: "spanname2",
			rattrs:   map[string]string{"service.name": "svc", string(semconv.DeploymentEnvironmentKey): "tracer-env"},
			sattrs:   map[string]any{"operation.name": "op", "resource.name": "res"},
			spanKind: ptrace.SpanKindClient,
			libname:  "spring",
			expected: createStatsPayload(agentEnv, agentHost, "svc", "op", "http", "client", "res", agentHost, "tracer-env", "", "", nil, nil, true, false),
		},
		{
			name:     "new env convention",
			spanName: "spanname2",
			rattrs:   map[string]string{"service.name": "svc", "deployment.environment.name": "new-env"},
			sattrs:   map[string]any{"operation.name": "op", "resource.name": "res"},
			spanKind: ptrace.SpanKindClient,
			libname:  "spring",
			expected: createStatsPayload(agentEnv, agentHost, "svc", "op", "http", "client", "res", agentHost, "new-env", "", "", nil, nil, true, false),
		},
		{
			name:                   "span operation name from span name with db attribute, peerTagsAggr not enabled",
			spanName:               "spanname3",
			rattrs:                 map[string]string{"service.name": "svc", "host.name": "test-host", "db.system": "redis"},
			spanKind:               ptrace.SpanKindClient,
			spanNameAsResourceName: true,
			expected:               createStatsPayload(agentEnv, agentHost, "svc", "spanname3", "cache", "client", "spanname3", "test-host", agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:     "with container tags",
			spanName: "spanname4",
			rattrs:   map[string]string{"service.name": "svc", "db.system": "spanner", string(semconv.ContainerIDKey): "test_cid"},
			ctagKeys: []string{string(semconv.ContainerIDKey)},
			spanKind: ptrace.SpanKindClient,
			expected: createStatsPayload(agentEnv, agentHost, "svc", "opentelemetry.client", "db", "client", "spanname4", agentHost, agentEnv, "test_cid", "", []string{"container_id:test_cid", "env:test_env"}, nil, true, false),
		},
		{
			name:               "operation name remapping and resource from http",
			spanName:           "spanname5",
			spanKind:           ptrace.SpanKindInternal,
			sattrs:             map[string]any{string(semconv.HTTPMethodKey): "GET", string(semconv.HTTPRouteKey): "/home"},
			spanNameRemappings: map[string]string{"opentelemetry.internal": "internal_op"},
			expected:           createStatsPayload(agentEnv, agentHost, "otlpresourcenoservicename", "internal_op", "custom", "internal", "GET /home", agentHost, agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:         "with peer tags and peerTagsAggr enabled",
			spanName:     "spanname6",
			spanKind:     ptrace.SpanKindClient,
			peerTagsAggr: true,
			rattrs:       map[string]string{"service.name": "svc", string(semconv.DeploymentEnvironmentKey): "tracer-env", "datadog.host.name": "dd-host"},
			sattrs:       map[string]any{"operation.name": "op", string(semconv.RPCMethodKey): "call", string(semconv.RPCServiceKey): "rpc_service"},
			expected:     createStatsPayload(agentEnv, agentHost, "svc", "op", "http", "client", "call rpc_service", "dd-host", "tracer-env", "", "", nil, []string{"rpc.service:rpc_service"}, true, false),
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
			name:                             "obfuscate sql span",
			spanName:                         "spanname8",
			spanKind:                         ptrace.SpanKindClient,
			rattrs:                           map[string]string{"service.name": "svc", string(semconv.DBSystemKey): semconv.DBSystemMSSQL.Value.AsString(), string(semconv.DBStatementKey): "SELECT username FROM users WHERE id = 12345"},
			enableObfuscation:                true,
			enableReceiveResourceSpansV2:     true,
			enableOperationAndResourceNameV2: true,
			expected:                         createStatsPayload(agentEnv, agentHost, "svc", "mssql.query", "sql", "client", "SELECT username FROM users WHERE id = ?", agentHost, agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:                             "obfuscated redis span",
			spanName:                         "spanname9",
			rattrs:                           map[string]string{"service.name": "svc", "host.name": "test-host", "db.system": "redis", "db.statement": "SET key value"},
			spanKind:                         ptrace.SpanKindClient,
			enableObfuscation:                true,
			enableReceiveResourceSpansV2:     true,
			enableOperationAndResourceNameV2: true,
			expected:                         createStatsPayload(agentEnv, agentHost, "svc", "redis.query", "redis", "client", "SET", "test-host", agentEnv, "", "", nil, nil, true, false),
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
			spanKind:                         ptrace.SpanKindServer,
			libname:                          "ddtracer",
			enableOperationAndResourceNameV2: true,
			enableReceiveResourceSpansV2:     true,
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
			),
		},
		{
			name: "k8s.pod.uid used as a fallback for container.id by default",
			rattrs: map[string]string{
				"k8s.pod.uid": "test_pod_uid",
			},
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"otlpresourcenoservicename",
				"opentelemetry.unspecified",
				"custom",
				"unspecified",
				"",
				agentHost,
				agentEnv,
				"test_pod_uid",
				"",
				nil,
				nil,
				true,
				false,
			),
		},
		{
			name: "k8s.pod.uid not used as a fallback for container.id with container tags v2",
			rattrs: map[string]string{
				"k8s.pod.uid": "test_pod_uid",
			},
			enableContainerTagsV2: true,
			expected: createStatsPayload(
				agentEnv,
				agentHost,
				"otlpresourcenoservicename",
				"opentelemetry.unspecified",
				"custom",
				"unspecified",
				"",
				agentHost,
				agentEnv,
				"",
				"",
				nil,
				nil,
				true,
				false,
			),
		},
		{
			name:     "gRPC status code is included in trace metrics",
			spanName: "grpc_span",
			rattrs:   map[string]string{"service.name": "grpc-svc"},
			sattrs:   map[string]any{"rpc.grpc.status_code": int64(2), "rpc.system.name": "grpc", "rpc.method": "GetUser", "rpc.service": "UserService"},
			spanKind: ptrace.SpanKindServer,
			libname:  "otelgrpc",
			expected: &pb.StatsPayload{
				AgentEnv:      agentEnv,
				AgentHostname: agentHost,
				Stats: []*pb.ClientStatsPayload{{
					Hostname: agentHost,
					Env:      agentEnv,
					Stats: []*pb.ClientStatsBucket{{
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "grpc-svc",
								Name:           "otelgrpc.server",
								Resource:       "GetUser UserService",
								Type:           "web",
								Hits:           1,
								TopLevelHits:   1,
								SpanKind:       "server",
								IsTraceRoot:    pb.Trilean_TRUE,
								GRPCStatusCode: "2",
							},
						},
					}}}},
			},
		},
		{
			name:     "grpc system can interpret rpc.response.status_code",
			spanName: "grpc_response_span",
			rattrs:   map[string]string{"service.name": "grpc-svc"},
			sattrs:   map[string]any{"rpc.response.status_code": int64(5), "rpc.system.name": "grpc", "rpc.method": "FindUser", "rpc.service": "UserService"},
			spanKind: ptrace.SpanKindServer,
			libname:  "otelgrpc",
			expected: &pb.StatsPayload{
				AgentEnv:      agentEnv,
				AgentHostname: agentHost,
				Stats: []*pb.ClientStatsPayload{{
					Hostname: agentHost,
					Env:      agentEnv,
					Stats: []*pb.ClientStatsBucket{{
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "grpc-svc",
								Name:           "otelgrpc.server",
								Resource:       "FindUser UserService",
								Type:           "web",
								Hits:           1,
								TopLevelHits:   1,
								SpanKind:       "server",
								IsTraceRoot:    pb.Trilean_TRUE,
								GRPCStatusCode: "5",
							},
						},
					}}}},
			},
		},
		{
			name:     "jsonrpc system cannot interpret rpc.response.status_code",
			spanName: "jsonrpc_response_span",
			rattrs:   map[string]string{"service.name": "jsonrpc-svc"},
			sattrs:   map[string]any{"rpc.response.status_code": int64(5), "rpc.system.name": "jsonrpc", "rpc.method": "CallMethod", "rpc.service": "JsonService"},
			spanKind: ptrace.SpanKindServer,
			libname:  "oteljsonrpc",
			expected: &pb.StatsPayload{
				AgentEnv:      agentEnv,
				AgentHostname: agentHost,
				Stats: []*pb.ClientStatsPayload{{
					Hostname: agentHost,
					Env:      agentEnv,
					Stats: []*pb.ClientStatsBucket{{
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "jsonrpc-svc",
								Name:           "oteljsonrpc.server",
								Resource:       "CallMethod JsonService",
								Type:           "web",
								Hits:           1,
								TopLevelHits:   1,
								SpanKind:       "server",
								IsTraceRoot:    pb.Trilean_TRUE,
								GRPCStatusCode: "",
							},
						},
					}}}},
			},
		},
		{
			name:     "jsonrpc system can still interpret rpc.grpc.status_code",
			spanName: "jsonrpc_span",
			rattrs:   map[string]string{"service.name": "jsonrpc-svc"},
			sattrs:   map[string]any{"rpc.grpc.status_code": int64(3), "rpc.system": "jsonrpc", "rpc.method": "CallMethod", "rpc.service": "JsonService"},
			spanKind: ptrace.SpanKindServer,
			libname:  "oteljsonrpc",
			expected: &pb.StatsPayload{
				AgentEnv:      agentEnv,
				AgentHostname: agentHost,
				Stats: []*pb.ClientStatsPayload{{
					Hostname: agentHost,
					Env:      agentEnv,
					Stats: []*pb.ClientStatsBucket{{
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "jsonrpc-svc",
								Name:           "oteljsonrpc.server",
								Resource:       "CallMethod JsonService",
								Type:           "web",
								Hits:           1,
								TopLevelHits:   1,
								SpanKind:       "server",
								IsTraceRoot:    pb.Trilean_TRUE,
								GRPCStatusCode: "3",
							},
						},
					}}}},
			},
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
			if !tt.enableReceiveResourceSpansV2 {
				conf.Features["disable_receive_resource_spans_v2"] = struct{}{}
			}
			conf.PeerTagsAggregation = tt.peerTagsAggr
			conf.OTLPReceiver.AttributesTranslator = attributesTranslator
			conf.OTLPReceiver.SpanNameAsResourceName = tt.spanNameAsResourceName
			if !tt.enableOperationAndResourceNameV2 {
				conf.Features["disable_operation_and_resource_name_logic_v2"] = struct{}{}
			}
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
			if tt.enableContainerTagsV2 {
				conf.Features["enable_otlp_container_tags_v2"] = struct{}{}
			}

			concentrator := newTestConcentratorWithCfg(time.Now(), conf)
			var obfuscator *obfuscate.Obfuscator
			var inputs []corestats.Input
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

func TestProcessOTLPTraces_MutliSpanInOneResAndOp(t *testing.T) {
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

	concentrator := newTestConcentratorWithCfg(time.Now(), conf)
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

func createStatsPayload(
	agentEnv, agentHost, svc, operation, typ, kind, resource, tracerHost, env, cid, version string,
	ctags, peerTags []string,
	root, nonTopLevel bool,
) *pb.StatsPayload {
	traceroot := pb.Trilean_TRUE
	if !root {
		traceroot = pb.Trilean_FALSE
	}
	var topLevelHits uint64
	if !nonTopLevel {
		topLevelHits = 1
	}
	return &pb.StatsPayload{
		AgentEnv:      agentEnv,
		AgentHostname: agentHost,
		Stats: []*pb.ClientStatsPayload{{
			Hostname:    tracerHost,
			Version:     version,
			Env:         env,
			ContainerID: cid,
			Tags:        ctags,
			Stats: []*pb.ClientStatsBucket{{
				Stats: []*pb.ClientGroupedStats{
					{
						Service:      svc,
						Name:         operation,
						Resource:     resource,
						Type:         typ,
						Hits:         1,
						TopLevelHits: topLevelHits,
						SpanKind:     kind,
						PeerTags:     peerTags,
						IsTraceRoot:  traceroot,
					},
				},
			}}}},
	}
}

type noopStatsWriter struct{}

func (noopStatsWriter) Write(*pb.StatsPayload) {}

func newTestConcentratorWithCfg(now time.Time, cfg *config.AgentConfig) *corestats.Concentrator {
	return corestats.NewConcentrator(cfg, noopStatsWriter{}, now, &statsd.NoOpClient{})
}

// TestProcessOTLPTraces_HTTPRequestMethodAndEnvName tests stats processing with
// http.request.method (semconv 1.23+) and deployment.environment.name (semconv 1.27+) attributes.
func TestProcessOTLPTraces_HTTPRequestMethodAndEnvName(t *testing.T) {
	start := time.Now().Add(-1 * time.Second)
	end := time.Now()
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(t, err)

	agentEnv := "agent_env"
	agentHost := "agent_host"

	for _, tt := range []struct {
		name                             string
		spanName                         string
		rattrs                           map[string]string
		sattrs                           map[string]any
		spanKind                         ptrace.SpanKind
		enableOperationAndResourceNameV2 bool
		enableReceiveResourceSpansV2     bool
		expected                         *pb.StatsPayload
	}{
		{
			name:     "http.request.method (semconv 1.23+) V2 enabled",
			spanName: "/api/users",
			rattrs:   map[string]string{"service.name": "http-svc"},
			sattrs: map[string]any{
				"http.request.method": "GET",
				"http.route":          "/api/users",
			},
			spanKind:                         ptrace.SpanKindServer,
			enableOperationAndResourceNameV2: true,
			enableReceiveResourceSpansV2:     true,
			expected:                         createStatsPayload(agentEnv, agentHost, "http-svc", "http.server.request", "web", "server", "GET /api/users", agentHost, agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:     "http.request.method (semconv 1.23+) V1 enabled",
			spanName: "/api/users",
			rattrs:   map[string]string{"service.name": "http-svc"},
			sattrs: map[string]any{
				"http.request.method": "GET",
				"http.route":          "/api/users",
			},
			spanKind:                         ptrace.SpanKindServer,
			enableOperationAndResourceNameV2: false,
			enableReceiveResourceSpansV2:     false,
			expected:                         createStatsPayload(agentEnv, agentHost, "http-svc", "opentelemetry.server", "web", "server", "GET /api/users", agentHost, agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:     "deployment.environment.name (semconv 1.27+)",
			spanName: "test-span",
			rattrs: map[string]string{
				"service.name":                "env-svc",
				"deployment.environment.name": "production-127",
			},
			sattrs:                           map[string]any{},
			spanKind:                         ptrace.SpanKindServer,
			enableOperationAndResourceNameV2: true,
			enableReceiveResourceSpansV2:     true,
			expected:                         createStatsPayload(agentEnv, agentHost, "env-svc", "server.request", "web", "server", "test-span", agentHost, "production-127", "", "", nil, nil, true, false),
		},
		{
			name:     "both deployment.environment.name (1.27) and deployment.environment (1.17) - newer takes precedence",
			spanName: "test-span",
			rattrs: map[string]string{
				"service.name":                           "env-svc",
				"deployment.environment.name":            "prod-127",
				string(semconv.DeploymentEnvironmentKey): "prod-117",
			},
			sattrs:                           map[string]any{},
			spanKind:                         ptrace.SpanKindServer,
			enableOperationAndResourceNameV2: true,
			enableReceiveResourceSpansV2:     true,
			expected:                         createStatsPayload(agentEnv, agentHost, "env-svc", "server.request", "web", "server", "test-span", agentHost, "prod-127", "", "", nil, nil, true, false),
		},
		{
			name:     "db.system with V2 - operation name from db.system",
			spanName: "SELECT users",
			rattrs: map[string]string{
				"service.name": "db-svc",
				"db.system":    "postgresql",
			},
			sattrs: map[string]any{
				"db.statement": "SELECT * FROM users",
			},
			spanKind:                         ptrace.SpanKindClient,
			enableOperationAndResourceNameV2: true,
			enableReceiveResourceSpansV2:     true,
			expected:                         createStatsPayload(agentEnv, agentHost, "db-svc", "postgresql.query", "sql", "client", "SELECT * FROM users", agentHost, agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:     "db.query.text (semconv 1.26+) with V2",
			spanName: "SELECT orders",
			rattrs: map[string]string{
				"service.name": "db-svc",
				"db.system":    "postgresql",
			},
			sattrs: map[string]any{
				"db.query.text": "SELECT * FROM orders WHERE status = 'active'",
			},
			spanKind:                         ptrace.SpanKindClient,
			enableOperationAndResourceNameV2: true,
			enableReceiveResourceSpansV2:     true,
			expected:                         createStatsPayload(agentEnv, agentHost, "db-svc", "postgresql.query", "sql", "client", "SELECT * FROM orders WHERE status = 'active'", agentHost, agentEnv, "", "", nil, nil, true, false),
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
			span := sspan.Spans().AppendEmpty()
			span.SetTraceID(testTraceID)
			span.SetSpanID(testSpanID1)
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
			if !tt.enableReceiveResourceSpansV2 {
				conf.Features["disable_receive_resource_spans_v2"] = struct{}{}
			}
			if !tt.enableOperationAndResourceNameV2 {
				conf.Features["disable_operation_and_resource_name_logic_v2"] = struct{}{}
			}
			conf.OTLPReceiver.AttributesTranslator = attributesTranslator

			concentrator := newTestConcentratorWithCfg(time.Now(), conf)
			inputs := OTLPTracesToConcentratorInputs(traces, conf, nil, nil)
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

// TestProcessOTLPTraces_SemconvVersionMixing tests stats processing when spans use different semconv versions.
func TestProcessOTLPTraces_SemconvVersionMixing(t *testing.T) {
	start := time.Now().Add(-1 * time.Second)
	end := time.Now()
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(t, err)

	agentEnv := "agent_env"
	agentHost := "agent_host"

	for _, tt := range []struct {
		name                             string
		spanName                         string
		rattrs                           map[string]string
		sattrs                           map[string]any
		spanKind                         ptrace.SpanKind
		enableOperationAndResourceNameV2 bool
		expected                         *pb.StatsPayload
	}{
		{
			name:     "mixed HTTP conventions - old http.method and new http.request.method",
			spanName: "/api/mixed",
			rattrs:   map[string]string{"service.name": "mixed-svc"},
			sattrs: map[string]any{
				"http.method":         "GET",
				"http.request.method": "POST", // newer convention should take precedence in V2
				"http.route":          "/api/mixed",
			},
			spanKind:                         ptrace.SpanKindServer,
			enableOperationAndResourceNameV2: true,
			expected:                         createStatsPayload(agentEnv, agentHost, "mixed-svc", "http.server.request", "web", "server", "POST /api/mixed", agentHost, agentEnv, "", "", nil, nil, true, false),
		},
		{
			name:     "mixed env conventions - deployment.environment and deployment.environment.name",
			spanName: "test-span",
			rattrs: map[string]string{
				"service.name":                           "mixed-env-svc",
				string(semconv.DeploymentEnvironmentKey): "env-117",
				"deployment.environment.name":            "env-127",
			},
			sattrs:                           map[string]any{},
			spanKind:                         ptrace.SpanKindServer,
			enableOperationAndResourceNameV2: true,
			expected:                         createStatsPayload(agentEnv, agentHost, "mixed-env-svc", "server.request", "web", "server", "test-span", agentHost, "env-127", "", "", nil, nil, true, false),
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
			span := sspan.Spans().AppendEmpty()
			span.SetTraceID(testTraceID)
			span.SetSpanID(testSpanID1)
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
			conf.OTLPReceiver.AttributesTranslator = attributesTranslator
			if !tt.enableOperationAndResourceNameV2 {
				conf.Features["disable_operation_and_resource_name_logic_v2"] = struct{}{}
			}

			concentrator := newTestConcentratorWithCfg(time.Now(), conf)
			inputs := OTLPTracesToConcentratorInputs(traces, conf, nil, nil)
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
