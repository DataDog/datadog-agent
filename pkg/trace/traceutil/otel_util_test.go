// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv126 "go.opentelemetry.io/otel/semconv/v1.26.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"

	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

var (
	testTraceID = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	testSpanID1 = [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	testSpanID2 = [8]byte{2, 2, 3, 4, 5, 6, 7, 8}
	testSpanID3 = [8]byte{3, 2, 3, 4, 5, 6, 7, 8}
	testSpanID4 = [8]byte{4, 2, 3, 4, 5, 6, 7, 8}
	testSpanID5 = [8]byte{5, 2, 3, 4, 5, 6, 7, 8}
	testSpanID6 = [8]byte{6, 2, 3, 4, 5, 6, 7, 8}
)

func TestIndexOTelSpans(t *testing.T) {
	traces := ptrace.NewTraces()

	rspan1 := traces.ResourceSpans().AppendEmpty()
	res1 := rspan1.Resource()
	rattrs := res1.Attributes()
	rattrs.PutStr(string(semconv.HostNameKey), "host1")
	rattrs.PutStr(string(semconv.ServiceNameKey), "svc1")
	rattrs.PutStr(string(semconv.DeploymentEnvironmentKey), "env1")

	sspan1 := rspan1.ScopeSpans().AppendEmpty()
	scope1 := sspan1.Scope()
	scope1.SetName("scope")
	scope1.SetVersion("1.0.0")

	span1 := sspan1.Spans().AppendEmpty()
	span1.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span1.SetKind(ptrace.SpanKindClient)
	span1.SetName("span_name1")
	span1.SetTraceID(testTraceID)
	span1.SetSpanID(testSpanID1)

	span2 := sspan1.Spans().AppendEmpty()
	span2.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span2.SetKind(ptrace.SpanKindClient)
	span2.SetName("span_name2")
	span2.SetTraceID(testTraceID)
	span2.SetSpanID(testSpanID2)

	rspan2 := traces.ResourceSpans().AppendEmpty()
	res2 := rspan2.Resource()
	rattrs = res2.Attributes()
	rattrs.PutStr(string(semconv.HostNameKey), "host2")
	rattrs.PutStr(string(semconv.ServiceNameKey), "svc2")
	rattrs.PutStr(string(semconv.DeploymentEnvironmentKey), "env2")

	sspan2 := rspan2.ScopeSpans().AppendEmpty()
	scope2 := sspan2.Scope()
	scope2.SetName("scope2")
	scope2.SetVersion("1.0.0")

	span3 := sspan2.Spans().AppendEmpty()
	span3.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span3.SetKind(ptrace.SpanKindClient)
	span3.SetName("span_name3")
	span3.SetTraceID(testTraceID)
	span3.SetSpanID(testSpanID3)

	// Spans with empty trace ID are discarded
	span4 := sspan2.Spans().AppendEmpty()
	span4.SetTraceID(pcommon.NewTraceIDEmpty())

	// Spans with empty span ID are discarded
	span5 := sspan2.Spans().AppendEmpty()
	span5.SetTraceID(testTraceID)
	span5.SetSpanID(pcommon.NewSpanIDEmpty())

	spanByID, resByID, scopeByID := IndexOTelSpans(traces)
	assert.Equal(t, map[pcommon.SpanID]ptrace.Span{testSpanID1: span1, testSpanID2: span2, testSpanID3: span3}, spanByID)
	assert.Equal(t, map[pcommon.SpanID]pcommon.Resource{testSpanID1: res1, testSpanID2: res1, testSpanID3: res2}, resByID)
	assert.Equal(t, map[pcommon.SpanID]pcommon.InstrumentationScope{testSpanID1: scope1, testSpanID2: scope1, testSpanID3: scope2}, scopeByID)
}

func TestGetTopLevelOTelSpans(t *testing.T) {
	traces := ptrace.NewTraces()
	rspans := traces.ResourceSpans().AppendEmpty()
	rspans.Resource().Attributes().PutStr(string(semconv.ServiceNameKey), "svc1")
	sspans := rspans.ScopeSpans().AppendEmpty()

	// Root span
	// Is top level in both new and old rules
	span1 := sspans.Spans().AppendEmpty()
	span1.SetTraceID(testTraceID)
	span1.SetSpanID(testSpanID1)

	// Span with span kind server
	// Is top-level in new rules, is not in old rules
	span2 := sspans.Spans().AppendEmpty()
	span2.SetTraceID(testTraceID)
	span2.SetSpanID(testSpanID2)
	span2.SetParentSpanID(testSpanID1)
	span2.SetKind(ptrace.SpanKindServer)

	// Span with span kind consumer
	// Is top-level in new rules, is not in old rules
	span3 := sspans.Spans().AppendEmpty()
	span3.SetTraceID(testTraceID)
	span3.SetSpanID(testSpanID3)
	span3.SetParentSpanID(testSpanID1)
	span3.SetKind(ptrace.SpanKindConsumer)

	// Span with span kind client but parent is not in this chunk
	// Is top-level in old rules, is not in new rules
	span4 := sspans.Spans().AppendEmpty()
	span4.SetTraceID(testTraceID)
	span4.SetSpanID(testSpanID4)
	span4.SetParentSpanID(testSpanID6)
	span4.SetKind(ptrace.SpanKindClient)

	// Spans with span kind internal but has a different service than its parent
	// Is top-level in old rules, is not in new rules
	rspans2 := traces.ResourceSpans().AppendEmpty()
	rspans2.Resource().Attributes().PutStr(string(semconv.ServiceNameKey), "svc2")
	span5 := rspans2.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span5.SetTraceID(testTraceID)
	span5.SetSpanID(testSpanID5)
	span5.SetParentSpanID(testSpanID1)
	span5.SetKind(ptrace.SpanKindInternal)

	spanByID, resByID, _ := IndexOTelSpans(traces)
	topLevelSpans := GetTopLevelOTelSpans(spanByID, resByID, true)
	assert.Equal(t, topLevelSpans, map[pcommon.SpanID]struct{}{
		testSpanID1: {},
		testSpanID2: {},
		testSpanID3: {},
	})

	topLevelSpans = GetTopLevelOTelSpans(spanByID, resByID, false)
	assert.Equal(t, topLevelSpans, map[pcommon.SpanID]struct{}{
		testSpanID1: {},
		testSpanID4: {},
		testSpanID5: {},
	})
}

func TestGetOTelSpanType(t *testing.T) {
	for _, tt := range []struct {
		name     string
		spanKind ptrace.SpanKind
		rattrs   map[string]string
		sattrs   map[string]string
		expected string
	}{
		{
			name:     "override with span.type attr",
			spanKind: ptrace.SpanKindInternal,
			rattrs:   map[string]string{"span.type": "my-type"},
			expected: "my-type",
		},
		{
			name:     "web span",
			spanKind: ptrace.SpanKindServer,
			expected: "web",
		},
		{
			name:     "redis span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): "redis"},
			expected: spanTypeRedis,
		},
		{
			name:     "memcached span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): "memcached"},
			expected: spanTypeMemcached,
		},
		{
			name:     "sql db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemPostgreSQL.Value.AsString()},
			expected: spanTypeSQL,
		},
		{
			name:     "elastic db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemElasticsearch.Value.AsString()},
			expected: spanTypeElasticsearch,
		},
		{
			name:     "opensearch db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv117.DBSystemOpensearch.Value.AsString()},
			expected: spanTypeOpenSearch,
		},
		{
			name:     "cassandra db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemCassandra.Value.AsString()},
			expected: spanTypeCassandra,
		},
		{
			name:     "other db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemCouchDB.Value.AsString()},
			expected: spanTypeDB,
		},
		{
			name:     "http client span",
			spanKind: ptrace.SpanKindClient,
			expected: "http",
		},
		{
			name:     "other custom span",
			spanKind: ptrace.SpanKindInternal,
			expected: "custom",
		},
		{
			name:     "span.type in both span and resource (span wins)",
			rattrs:   map[string]string{"span.type": "resource-type"},
			sattrs:   map[string]string{"span.type": "span-type"},
			expected: "span-type",
		},
		{
			name:     "span.type only in span",
			sattrs:   map[string]string{"span.type": "span-type"},
			expected: "span-type",
		},
		{
			name:     "span.type only in resource",
			rattrs:   map[string]string{"span.type": "resource-type"},
			expected: "resource-type",
		},
		{
			name:     "db.system in both span and resource (span wins)",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{"db.system": "redis"},
			sattrs:   map[string]string{"db.system": "memcached"},
			expected: spanTypeMemcached,
		},
		{
			name:     "db.system only in span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{"db.system": "redis"},
			expected: spanTypeRedis,
		},
		{
			name:     "db.system only in resource",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{"db.system": "redis"},
			expected: spanTypeRedis,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetKind(tt.spanKind)
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			actual := GetOTelSpanType(span, res)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestSpanKind2Type(t *testing.T) {
	for _, tt := range []struct {
		kind ptrace.SpanKind
		meta map[string]string
		out  string
	}{
		{
			kind: ptrace.SpanKindServer,
			out:  "web",
		},
		{
			kind: ptrace.SpanKindClient,
			out:  "http",
		},
		{
			kind: ptrace.SpanKindClient,
			meta: map[string]string{"db.system": "redis"},
			out:  "cache",
		},
		{
			kind: ptrace.SpanKindClient,
			meta: map[string]string{"db.system": "memcached"},
			out:  "cache",
		},
		{
			kind: ptrace.SpanKindClient,
			meta: map[string]string{"db.system": "other"},
			out:  "db",
		},
		{
			kind: ptrace.SpanKindProducer,
			out:  "custom",
		},
		{
			kind: ptrace.SpanKindConsumer,
			out:  "custom",
		},
		{
			kind: ptrace.SpanKindInternal,
			out:  "custom",
		},
		{
			kind: ptrace.SpanKindUnspecified,
			out:  "custom",
		},
	} {
		t.Run(tt.out, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetKind(tt.kind)
			res := pcommon.NewResource()
			for k, v := range tt.meta {
				res.Attributes().PutStr(k, v)
			}
			actual := SpanKind2Type(span, res)
			assert.Equal(t, tt.out, actual)
		})
	}
}

func TestGetOTelService(t *testing.T) {
	for _, tt := range []struct {
		name      string
		rattrs    map[string]string
		sattrs    map[string]string
		normalize bool
		expected  string
	}{
		{
			name:     "service not set",
			expected: "otlpresourcenoservicename",
		},
		{
			name:     "normal service in resource",
			rattrs:   map[string]string{string(semconv.ServiceNameKey): "svc"},
			expected: "svc",
		},
		{
			name:     "normal service in span",
			sattrs:   map[string]string{string(semconv.ServiceNameKey): "svc"},
			expected: "svc",
		},
		{
			name:     "service in both, span takes precedence",
			rattrs:   map[string]string{string(semconv.ServiceNameKey): "resource_svc"},
			sattrs:   map[string]string{string(semconv.ServiceNameKey): "span_svc"},
			expected: "span_svc",
		},
		{
			name:      "truncate long service",
			rattrs:    map[string]string{string(semconv.ServiceNameKey): strings.Repeat("a", normalizeutil.MaxServiceLen+1)},
			normalize: true,
			expected:  strings.Repeat("a", normalizeutil.MaxServiceLen),
		},
		{
			name:      "invalid service",
			rattrs:    map[string]string{string(semconv.ServiceNameKey): "%$^"},
			normalize: true,
			expected:  normalizeutil.DefaultServiceName,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			span := ptrace.NewSpan()
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			actual := GetOTelService(span, res, tt.normalize)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetOTelResource(t *testing.T) {
	for _, tt := range []struct {
		name       string
		rattrs     map[string]string
		sattrs     map[string]string
		normalize  bool
		expectedV1 string
		expectedV2 string
	}{
		{
			name:       "resource not set",
			expectedV1: "span_name",
			expectedV2: "span_name",
		},
		{
			name:       "normal resource",
			sattrs:     map[string]string{"resource.name": "res"},
			expectedV1: "res",
			expectedV2: "res",
		},
		{
			name:       "HTTP request method resource",
			sattrs:     map[string]string{"http.request.method": "GET"},
			expectedV1: "GET",
			expectedV2: "GET",
		},
		{
			name:       "HTTP method and route resource",
			sattrs:     map[string]string{string(semconv.HTTPMethodKey): "GET", string(semconv.HTTPRouteKey): "/"},
			expectedV1: "GET /",
			expectedV2: "GET",
		},
		{
			name:       "truncate long resource",
			sattrs:     map[string]string{"resource.name": strings.Repeat("a", normalizeutil.MaxResourceLen+1)},
			normalize:  true,
			expectedV1: strings.Repeat("a", normalizeutil.MaxResourceLen),
			expectedV2: strings.Repeat("a", normalizeutil.MaxResourceLen),
		},
		{
			name:       "GraphQL with no type",
			sattrs:     map[string]string{"graphql.operation.name": "myQuery"},
			normalize:  false,
			expectedV1: "span_name",
			expectedV2: "span_name",
		},
		{
			name:       "GraphQL with only type",
			sattrs:     map[string]string{"graphql.operation.type": "query"},
			normalize:  false,
			expectedV1: "query",
			expectedV2: "query",
		},
		{
			name:       "GraphQL with only type",
			sattrs:     map[string]string{"graphql.operation.type": "query", "graphql.operation.name": "myQuery"},
			normalize:  false,
			expectedV1: "query myQuery",
			expectedV2: "query myQuery",
		},
		{
			name: "SQL statement resource",
			rattrs: map[string]string{
				string(semconv.DBSystemKey):    "mysql",
				string(semconv.DBStatementKey): "SELECT * FROM table WHERE id = 12345",
			},
			sattrs:     map[string]string{"span.name": "span_name"},
			expectedV1: "span_name",
			expectedV2: "SELECT * FROM table WHERE id = 12345",
		},
		{
			name: "Redis command resource",
			rattrs: map[string]string{
				string(semconv.DBSystemKey):       "redis",
				string(semconv126.DBQueryTextKey): "SET key value",
			},
			sattrs:     map[string]string{"span.name": "span_name"},
			expectedV1: "span_name",
			expectedV2: "SET key value",
		},
		{
			name:       "resource.name in both span and resource (span wins)",
			rattrs:     map[string]string{"resource.name": "res_resource"},
			sattrs:     map[string]string{"resource.name": "res_span"},
			expectedV1: "res_resource",
			expectedV2: "res_span",
		},
		{
			name:       "http.request.method in both span and resource (span wins)",
			rattrs:     map[string]string{"http.request.method": "POST"},
			sattrs:     map[string]string{"http.request.method": "GET"},
			expectedV1: "POST",
			expectedV2: "GET",
		},
		{
			name:       "messaging.operation in both span and resource (span wins)",
			rattrs:     map[string]string{"messaging.operation": "receive"},
			sattrs:     map[string]string{"messaging.operation": "process"},
			expectedV1: "receive",
			expectedV2: "process",
		},
		{
			name:       "rpc.method in both span and resource (span wins)",
			rattrs:     map[string]string{"rpc.method": "resource_method", "rpc.service": "resource_service"},
			sattrs:     map[string]string{"rpc.method": "span_method", "rpc.service": "span_service"},
			expectedV1: "resource_method resource_service",
			expectedV2: "span_method span_service",
		},
		{
			name:       "GraphQL type in both span and resource (span wins)",
			rattrs:     map[string]string{"graphql.operation.type": "mutation"},
			sattrs:     map[string]string{"graphql.operation.type": "query", "graphql.operation.name": "myQuery"},
			expectedV1: "mutation myQuery",
			expectedV2: "query myQuery",
		},
		{
			name:       "DB statement in both span and resource (span wins)",
			rattrs:     map[string]string{"db.system": "mysql", "db.statement": "SELECT * FROM resource"},
			sattrs:     map[string]string{"db.system": "mysql", "db.statement": "SELECT * FROM span"},
			expectedV1: "span_name",
			expectedV2: "SELECT * FROM span",
		},
		{
			name:       "fallback to span name if nothing set",
			sattrs:     map[string]string{},
			rattrs:     map[string]string{},
			expectedV1: "span_name",
			expectedV2: "span_name",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("span_name")
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			assert.Equal(t, tt.expectedV1, GetOTelResourceV1(span, res))
			assert.Equal(t, tt.expectedV2, GetOTelResourceV2(span, res))
		})
	}
}

func TestGetOTelOperationName(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		rattrs                 map[string]string
		sattrs                 map[string]string
		normalize              bool
		spanKind               ptrace.SpanKind
		libname                string
		spanNameAsResourceName bool
		spanNameRemappings     map[string]string
		expected               string
	}{
		{
			name:     "operation name from span kind",
			spanKind: ptrace.SpanKindClient,
			expected: "opentelemetry.client",
		},
		{
			name:     "operation name from instrumentation scope and span kind",
			spanKind: ptrace.SpanKindServer,
			libname:  "spring",
			expected: "spring.server",
		},
		{
			name:                   "operation name from span name",
			spanNameAsResourceName: true,
			expected:               "span_name",
		},
		{
			name:               "operation name remapping",
			spanKind:           ptrace.SpanKindInternal,
			spanNameRemappings: map[string]string{"opentelemetry.internal": "internal_op"},
			expected:           "internal_op",
		},
		{
			name:     "operation.name attribute",
			sattrs:   map[string]string{"operation.name": "op"},
			expected: "op",
		},
		{
			name:               "normalize empty operation name",
			sattrs:             map[string]string{"operation.name": "op"},
			spanNameRemappings: map[string]string{"op": ""},
			normalize:          true,
			expected:           normalizeutil.DefaultSpanName,
		},
		{
			name:      "normalize invalid operation name",
			sattrs:    map[string]string{"operation.name": "%$^"},
			normalize: true,
			expected:  normalizeutil.DefaultSpanName,
		},
		{
			name:      "truncate long operation name",
			sattrs:    map[string]string{"operation.name": strings.Repeat("a", normalizeutil.MaxNameLen+1)},
			normalize: true,
			expected:  strings.Repeat("a", normalizeutil.MaxNameLen),
		},
		{
			name:                   "operation name retrieved from span name, then remapped",
			sattrs:                 map[string]string{"operation.name": "op"},
			spanNameRemappings:     map[string]string{"op": "test_result"},
			spanNameAsResourceName: true,
			expected:               "test_result",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("span_name")
			span.SetKind(tt.spanKind)
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			lib := pcommon.NewInstrumentationScope()
			lib.SetName(tt.libname)
			actual := GetOTelOperationNameV1(span, res, lib, tt.spanNameAsResourceName, tt.spanNameRemappings, tt.normalize)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetOTelContainerTags(t *testing.T) {
	res := pcommon.NewResource()
	res.Attributes().PutStr(string(semconv.ContainerIDKey), "cid")
	res.Attributes().PutStr(string(semconv.ContainerNameKey), "cname")
	res.Attributes().PutStr(string(semconv.ContainerImageNameKey), "ciname")
	res.Attributes().PutStr(string(semconv.ContainerImageTagKey), "citag")
	res.Attributes().PutStr("az", "my-az")
	assert.Contains(t, GetOTelContainerTags(res.Attributes(), []string{"az", string(semconv.ContainerIDKey), string(semconv.ContainerNameKey), string(semconv.ContainerImageNameKey), string(semconv.ContainerImageTagKey)}), "container_id:cid", "container_name:cname", "image_name:ciname", "image_tag:citag", "az:my-az")
	assert.Contains(t, GetOTelContainerTags(res.Attributes(), []string{"az"}), "az:my-az")
}
