// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv126 "go.opentelemetry.io/otel/semconv/v1.26.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
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
			expected: attributes.SpanTypeRedis,
		},
		{
			name:     "memcached span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): "memcached"},
			expected: attributes.SpanTypeMemcached,
		},
		{
			name:     "sql db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemPostgreSQL.Value.AsString()},
			expected: attributes.SpanTypeSQL,
		},
		{
			name:     "elastic db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemElasticsearch.Value.AsString()},
			expected: attributes.SpanTypeElasticsearch,
		},
		{
			name:     "opensearch db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv117.DBSystemOpensearch.Value.AsString()},
			expected: attributes.SpanTypeOpenSearch,
		},
		{
			name:     "cassandra db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemCassandra.Value.AsString()},
			expected: attributes.SpanTypeCassandra,
		},
		{
			name:     "mongodb db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): "mongodb"},
			expected: attributes.SpanTypeMongoDB,
		},
		{
			name:     "other db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{string(semconv.DBSystemKey): semconv.DBSystemCouchDB.Value.AsString()},
			expected: attributes.SpanTypeDB,
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
			expected: attributes.SpanTypeMemcached,
		},
		{
			name:     "db.system only in span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{"db.system": "redis"},
			expected: attributes.SpanTypeRedis,
		},
		{
			name:     "db.system only in resource",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{"db.system": "redis"},
			expected: attributes.SpanTypeRedis,
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

// TestGetOTelOperationNameV2_HTTPRequestMethodFallback tests operation name derivation with
// http.request.method (semconv 1.23+) and http.method (semconv 1.6.1) attributes.
func TestGetOTelOperationNameV2_HTTPRequestMethodFallback(t *testing.T) {
	for _, tt := range []struct {
		name     string
		rattrs   map[string]string
		sattrs   map[string]string
		spanKind ptrace.SpanKind
		expected string
	}{
		{
			name:     "http.request.method (semconv 1.23+) server",
			sattrs:   map[string]string{"http.request.method": "GET"},
			spanKind: ptrace.SpanKindServer,
			expected: "http.server.request",
		},
		{
			name:     "http.request.method (semconv 1.23+) client",
			sattrs:   map[string]string{"http.request.method": "POST"},
			spanKind: ptrace.SpanKindClient,
			expected: "http.client.request",
		},
		{
			name:     "http.method (semconv 1.6.1) server",
			sattrs:   map[string]string{string(semconv.HTTPMethodKey): "GET"},
			spanKind: ptrace.SpanKindServer,
			expected: "http.server.request",
		},
		{
			name:     "http.method (semconv 1.6.1) client",
			sattrs:   map[string]string{string(semconv.HTTPMethodKey): "POST"},
			spanKind: ptrace.SpanKindClient,
			expected: "http.client.request",
		},
		{
			name: "both http.request.method and http.method - http.request.method takes precedence",
			sattrs: map[string]string{
				"http.request.method":         "DELETE",
				string(semconv.HTTPMethodKey): "GET",
			},
			spanKind: ptrace.SpanKindServer,
			expected: "http.server.request",
		},
		{
			name: "http.request.method _OTHER normalized to HTTP",
			sattrs: map[string]string{
				"http.request.method": "_OTHER",
			},
			spanKind: ptrace.SpanKindServer,
			expected: "http.server.request",
		},
		{
			name:     "span attrs take precedence over resource attrs",
			rattrs:   map[string]string{"http.request.method": "GET"},
			sattrs:   map[string]string{"http.request.method": "POST"},
			spanKind: ptrace.SpanKindServer,
			expected: "http.server.request",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(tt.spanKind)
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			actual := GetOTelOperationNameV2(span, res)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// TestGetOTelOperationNameV2_DBSystemOperationName tests operation name derivation from db.system attribute.
func TestGetOTelOperationNameV2_DBSystemOperationName(t *testing.T) {
	for _, tt := range []struct {
		name     string
		rattrs   map[string]string
		sattrs   map[string]string
		spanKind ptrace.SpanKind
		expected string
	}{
		{
			name:     "db.system postgresql client",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "postgresql"},
			spanKind: ptrace.SpanKindClient,
			expected: "postgresql.query",
		},
		{
			name:     "db.system mysql client",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "mysql"},
			spanKind: ptrace.SpanKindClient,
			expected: "mysql.query",
		},
		{
			name:     "db.system redis client",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "redis"},
			spanKind: ptrace.SpanKindClient,
			expected: "redis.query",
		},
		{
			name:     "db.system mongodb client",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "mongodb"},
			spanKind: ptrace.SpanKindClient,
			expected: "mongodb.query",
		},
		{
			name:     "db.system in resource, client span",
			rattrs:   map[string]string{string(semconv.DBSystemKey): "postgresql"},
			spanKind: ptrace.SpanKindClient,
			expected: "postgresql.query",
		},
		{
			name:     "db.system not used for non-client spans",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "postgresql"},
			spanKind: ptrace.SpanKindServer,
			expected: "server.request",
		},
		{
			name: "span db.system takes precedence over resource",
			rattrs: map[string]string{
				string(semconv.DBSystemKey): "mysql",
			},
			sattrs: map[string]string{
				string(semconv.DBSystemKey): "postgresql",
			},
			spanKind: ptrace.SpanKindClient,
			expected: "postgresql.query",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			span.SetKind(tt.spanKind)
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			actual := GetOTelOperationNameV2(span, res)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// TestGetOTelResourceV2_HTTPRequestMethodResource tests resource name derivation with
// http.request.method (semconv 1.23+) and http.method (semconv 1.6.1) attributes.
func TestGetOTelResourceV2_HTTPRequestMethodResource(t *testing.T) {
	for _, tt := range []struct {
		name     string
		rattrs   map[string]string
		sattrs   map[string]string
		spanKind ptrace.SpanKind
		expected string
	}{
		{
			name:     "http.request.method only (semconv 1.23+)",
			sattrs:   map[string]string{"http.request.method": "GET"},
			spanKind: ptrace.SpanKindClient,
			expected: "GET",
		},
		{
			name:     "http.request.method with route for server",
			sattrs:   map[string]string{"http.request.method": "POST", string(semconv.HTTPRouteKey): "/api/users"},
			spanKind: ptrace.SpanKindServer,
			expected: "POST /api/users",
		},
		{
			name:     "http.request.method with route for client - no route appended",
			sattrs:   map[string]string{"http.request.method": "POST", string(semconv.HTTPRouteKey): "/api/users"},
			spanKind: ptrace.SpanKindClient,
			expected: "POST",
		},
		{
			name:     "http.method (semconv 1.6.1) only",
			sattrs:   map[string]string{string(semconv.HTTPMethodKey): "DELETE"},
			spanKind: ptrace.SpanKindClient,
			expected: "DELETE",
		},
		{
			name: "http.request.method takes precedence over http.method",
			sattrs: map[string]string{
				"http.request.method":         "PUT",
				string(semconv.HTTPMethodKey): "GET",
			},
			spanKind: ptrace.SpanKindClient,
			expected: "PUT",
		},
		{
			name:     "http.request.method _OTHER normalized to HTTP",
			sattrs:   map[string]string{"http.request.method": "_OTHER"},
			spanKind: ptrace.SpanKindClient,
			expected: "HTTP",
		},
		{
			name:     "resource.name takes precedence over http.request.method",
			sattrs:   map[string]string{"resource.name": "custom-resource", "http.request.method": "GET"},
			spanKind: ptrace.SpanKindServer,
			expected: "custom-resource",
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
			actual := GetOTelResourceV2(span, res)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// TestGetOTelResourceV2_DBQueryTextFallback tests resource name derivation with
// db.statement (semconv 1.6.1) and db.query.text (semconv 1.26+) attributes.
func TestGetOTelResourceV2_DBQueryTextFallback(t *testing.T) {
	for _, tt := range []struct {
		name     string
		rattrs   map[string]string
		sattrs   map[string]string
		expected string
	}{
		{
			name:     "db.statement (semconv 1.6.1)",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "mysql", string(semconv.DBStatementKey): "SELECT * FROM users WHERE id = 1"},
			expected: "SELECT * FROM users WHERE id = 1",
		},
		{
			name:     "db.query.text (semconv 1.26+)",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "postgresql", string(semconv126.DBQueryTextKey): "SELECT * FROM orders"},
			expected: "SELECT * FROM orders",
		},
		{
			name: "db.statement takes precedence over db.query.text",
			sattrs: map[string]string{
				string(semconv.DBSystemKey):       "mysql",
				string(semconv.DBStatementKey):    "SELECT FROM users",
				string(semconv126.DBQueryTextKey): "SELECT FROM orders",
			},
			expected: "SELECT FROM users",
		},
		{
			name:     "db.system without statement falls back to span name",
			sattrs:   map[string]string{string(semconv.DBSystemKey): "postgresql"},
			expected: "span_name",
		},
		{
			name:     "resource.name takes precedence over db.statement",
			sattrs:   map[string]string{"resource.name": "custom-db-resource", string(semconv.DBSystemKey): "mysql", string(semconv.DBStatementKey): "SELECT 1"},
			expected: "custom-db-resource",
		},
		{
			name:     "db.statement in resource attrs",
			rattrs:   map[string]string{string(semconv.DBSystemKey): "mysql", string(semconv.DBStatementKey): "SELECT FROM res"},
			expected: "SELECT FROM res",
		},
		{
			name:     "span db.statement takes precedence over resource",
			rattrs:   map[string]string{string(semconv.DBSystemKey): "mysql", string(semconv.DBStatementKey): "SELECT FROM resource"},
			sattrs:   map[string]string{string(semconv.DBSystemKey): "mysql", string(semconv.DBStatementKey): "SELECT FROM span"},
			expected: "SELECT FROM span",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("span_name")
			span.SetKind(ptrace.SpanKindClient)
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			actual := GetOTelResourceV2(span, res)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// TestGetOTelAttrFromEitherMap_Precedence tests attribute lookup precedence between span and resource.
func TestGetOTelAttrFromEitherMap_Precedence(t *testing.T) {
	for _, tt := range []struct {
		name      string
		map1Attrs map[string]string
		map2Attrs map[string]string
		keys      []string
		normalize bool
		expected  string
	}{
		{
			name:      "key in map1 only",
			map1Attrs: map[string]string{"key1": "value1"},
			keys:      []string{"key1"},
			expected:  "value1",
		},
		{
			name:      "key in map2 only",
			map2Attrs: map[string]string{"key1": "value1"},
			keys:      []string{"key1"},
			expected:  "value1",
		},
		{
			name:      "key in both maps - map1 takes precedence",
			map1Attrs: map[string]string{"key1": "map1-value"},
			map2Attrs: map[string]string{"key1": "map2-value"},
			keys:      []string{"key1"},
			expected:  "map1-value",
		},
		{
			// Note: GetOTelAttrFromEitherMap searches map1 first, then map2.
			// For each map, it iterates through keys in order.
			// So map1[key2] is found before map2[key1] since map1 is searched first.
			name:      "map1 searched before map2 for any key",
			map1Attrs: map[string]string{"key2": "value2"},
			map2Attrs: map[string]string{"key1": "value1"},
			keys:      []string{"key1", "key2"},
			expected:  "value2", // map1[key2] found before map2[key1]
		},
		{
			// This test documents current behavior: map1 takes precedence over map2,
			// regardless of key order in the keys list.
			name:      "semconv version - map1 precedence over map2",
			map1Attrs: map[string]string{string(semconv117.DeploymentEnvironmentKey): "env-117"},
			map2Attrs: map[string]string{"deployment.environment.name": "env-127"},
			keys:      []string{"deployment.environment.name", string(semconv117.DeploymentEnvironmentKey)},
			expected:  "env-117", // map1 searched first, finds deployment.environment
		},
		{
			name:      "normalization applied",
			map1Attrs: map[string]string{"key1": "  VALUE  "},
			keys:      []string{"key1"},
			normalize: true,
			expected:  "_value",
		},
		{
			name:     "no matching keys returns empty",
			keys:     []string{"nonexistent"},
			expected: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			map1 := pcommon.NewMap()
			for k, v := range tt.map1Attrs {
				map1.PutStr(k, v)
			}
			map2 := pcommon.NewMap()
			for k, v := range tt.map2Attrs {
				map2.PutStr(k, v)
			}
			actual := GetOTelAttrFromEitherMap(map1, map2, tt.normalize, tt.keys...)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// =============================================================================
// FALLBACK INCONSISTENCY TESTS
// These tests document the CURRENT behavior of hardcoded fallbacks.
// Some of these behaviors may be inconsistent and are candidates for cleanup
// when the new semantics library is introduced.
// =============================================================================

// TestFallbackInconsistency_ResourceVsSpanPrecedence documents that two different
// helper functions have OPPOSITE precedence rules for span vs resource attributes.
func TestFallbackInconsistency_ResourceVsSpanPrecedence(t *testing.T) {
	// Create test span and resource with same key, different values
	span := ptrace.NewSpan()
	span.Attributes().PutStr("test.key", "span-value")
	span.Attributes().PutStr("db.system", "span-db")

	res := pcommon.NewResource()
	res.Attributes().PutStr("test.key", "resource-value")
	res.Attributes().PutStr("db.system", "resource-db")

	t.Run("GetOTelAttrFromEitherMap - span (map1) takes precedence", func(t *testing.T) {
		// When called with (span.Attributes(), res.Attributes())
		result := GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), false, "test.key")
		assert.Equal(t, "span-value", result, "GetOTelAttrFromEitherMap: span attrs (map1) take precedence")
	})

	t.Run("GetOTelAttrValInResAndSpanAttrs - RESOURCE takes precedence (INCONSISTENT)", func(t *testing.T) {
		// This function has resource-first precedence, which is OPPOSITE of GetOTelAttrFromEitherMap
		result := GetOTelAttrValInResAndSpanAttrs(span, res, false, "test.key")
		assert.Equal(t, "resource-value", result, "GetOTelAttrValInResAndSpanAttrs: RESOURCE takes precedence - INCONSISTENT!")
	})

	t.Run("SpanKind2Type uses GetOTelAttrValInResAndSpanAttrs - resource db.system wins", func(t *testing.T) {
		span.SetKind(ptrace.SpanKindClient)
		// SpanKind2Type calls GetOTelAttrValInResAndSpanAttrs for db.system
		// So resource value should win
		result := GetOTelAttrValInResAndSpanAttrs(span, res, true, string(semconv.DBSystemKey))
		assert.Equal(t, "resource-db", result, "SpanKind2Type: resource db.system takes precedence")
	})

	t.Run("GetOTelSpanType uses GetOTelAttrFromEitherMap - span db.system wins", func(t *testing.T) {
		span.SetKind(ptrace.SpanKindClient)
		// GetOTelSpanType calls GetOTelAttrFromEitherMap for db.system
		// So span value should win
		result := GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, string(semconv.DBSystemKey))
		assert.Equal(t, "span-db", result, "GetOTelSpanType: span db.system takes precedence")
	})
}

// TestFallbackInconsistency_MessagingDestinationPrecedence documents that messaging.destination (old)
// takes precedence over messaging.destination.name (new) in GetOTelResourceV1/V2.
func TestFallbackInconsistency_MessagingDestinationPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		sattrs     map[string]string
		expectedV1 string
		expectedV2 string
		note       string
	}{
		{
			name: "CURRENT: messaging.destination (old) checked before messaging.destination.name (new)",
			sattrs: map[string]string{
				"messaging.operation":                          "send",
				"messaging.destination":                        "old-destination",
				string(semconv117.MessagingDestinationNameKey): "new-destination",
			},
			expectedV1: "send old-destination",
			expectedV2: "send old-destination",
			note:       "Old semconv key is checked first in the fallback list.",
		},
		{
			name: "messaging.destination.name used when messaging.destination not present",
			sattrs: map[string]string{
				"messaging.operation":                          "receive",
				string(semconv117.MessagingDestinationNameKey): "new-destination",
			},
			expectedV1: "receive new-destination",
			expectedV2: "receive new-destination",
			note:       "Falls back to newer key when old key not present.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("span_name")
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			assert.Equal(t, tt.expectedV1, GetOTelResourceV1(span, res), "V1: %s", tt.note)
			assert.Equal(t, tt.expectedV2, GetOTelResourceV2(span, res), "V2: %s", tt.note)
		})
	}
}

// TestFallbackInconsistency_DBStatementVsQueryText documents that db.statement (old)
// takes precedence over db.query.text (new) in GetOTelResourceV2.
func TestFallbackInconsistency_DBStatementVsQueryText(t *testing.T) {
	tests := []struct {
		name     string
		sattrs   map[string]string
		expected string
		note     string
	}{
		{
			name: "CURRENT: db.statement (old) checked before db.query.text (new)",
			sattrs: map[string]string{
				"db.system":                       "postgresql",
				"db.statement":                    "SELECT * FROM old_table",
				string(semconv126.DBQueryTextKey): "SELECT * FROM new_table",
			},
			expected: "SELECT * FROM old_table",
			note:     "Old db.statement is checked first, even though db.query.text is newer (semconv 1.26+).",
		},
		{
			name: "db.query.text used when db.statement not present",
			sattrs: map[string]string{
				"db.system":                       "postgresql",
				string(semconv126.DBQueryTextKey): "SELECT * FROM new_table",
			},
			expected: "SELECT * FROM new_table",
			note:     "Falls back to db.query.text when db.statement not present.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("span_name")
			span.SetKind(ptrace.SpanKindClient)
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			assert.Equal(t, tt.expected, GetOTelResourceV2(span, res), "Note: %s", tt.note)
		})
	}
}

// TestFallbackInconsistency_HTTPMethodPrecedence documents the http.request.method vs http.method precedence.
func TestFallbackInconsistency_HTTPMethodPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		sattrs     map[string]string
		spanKind   ptrace.SpanKind
		expectedV1 string
		expectedV2 string
		note       string
	}{
		{
			name: "http.request.method (new) checked BEFORE http.method (old) - CORRECT precedence",
			sattrs: map[string]string{
				"http.request.method":         "POST",
				string(semconv.HTTPMethodKey): "GET",
			},
			spanKind:   ptrace.SpanKindServer,
			expectedV1: "POST",
			expectedV2: "POST",
			note:       "http.request.method (1.23+) takes precedence - this is CORRECT newer-first behavior.",
		},
		{
			name: "http.method used when http.request.method not present",
			sattrs: map[string]string{
				string(semconv.HTTPMethodKey): "DELETE",
			},
			spanKind:   ptrace.SpanKindServer,
			expectedV1: "DELETE",
			expectedV2: "DELETE",
			note:       "Falls back to http.method when newer key not present.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("span_name")
			span.SetKind(tt.spanKind)
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}
			res := pcommon.NewResource()
			assert.Equal(t, tt.expectedV1, GetOTelResourceV1(span, res), "V1: %s", tt.note)
			assert.Equal(t, tt.expectedV2, GetOTelResourceV2(span, res), "V2: %s", tt.note)
		})
	}
}

// makeSpanAndResource creates a test OTel span and resource with the given attributes.
func makeSpanAndResource(kind ptrace.SpanKind, spanAttrs, resAttrs map[string]string) (ptrace.Span, pcommon.Resource) {
	span := ptrace.NewSpan()
	span.SetName("test-span")
	span.SetKind(kind)
	span.SetTraceID(testTraceID)
	span.SetSpanID(testSpanID1)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-time.Second)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	for k, v := range spanAttrs {
		span.Attributes().PutStr(k, v)
	}
	res := pcommon.NewResource()
	for k, v := range resAttrs {
		res.Attributes().PutStr(k, v)
	}
	return span, res
}

// BenchmarkSemanticLookups measures the allocation and CPU cost of per-span semantic
// attribute lookups. Each sub-benchmark covers a different span profile (HTTP, DB, etc.)
// to exercise different code paths and lookup depths.
func BenchmarkSemanticLookups(b *testing.B) {
	benchCases := []struct {
		name      string
		kind      ptrace.SpanKind
		spanAttrs map[string]string
		resAttrs  map[string]string
	}{
		{
			name: "http_server",
			kind: ptrace.SpanKindServer,
			spanAttrs: map[string]string{
				"http.request.method":       "GET",
				"http.route":                "/api/v1/users",
				"http.response.status_code": "200",
				"network.protocol.name":     "http",
				"network.protocol.version":  "1.1",
			},
			resAttrs: map[string]string{
				"service.name":           "user-service",
				"service.version":        "1.2.3",
				"deployment.environment": "production",
			},
		},
		{
			name: "db_client",
			kind: ptrace.SpanKindClient,
			spanAttrs: map[string]string{
				"db.system":    "postgresql",
				"db.statement": "SELECT * FROM users WHERE id = ?",
				"db.name":      "mydb",
			},
			resAttrs: map[string]string{
				"service.name":           "user-service",
				"deployment.environment": "production",
			},
		},
		{
			name: "messaging_producer",
			kind: ptrace.SpanKindProducer,
			spanAttrs: map[string]string{
				"messaging.system":      "kafka",
				"messaging.operation":   "publish",
				"messaging.destination": "user-events",
			},
			resAttrs: map[string]string{
				"service.name":           "event-publisher",
				"deployment.environment": "production",
			},
		},
		{
			name: "rpc_client",
			kind: ptrace.SpanKindClient,
			spanAttrs: map[string]string{
				"rpc.system":  "grpc",
				"rpc.service": "UserService",
				"rpc.method":  "GetUser",
			},
			resAttrs: map[string]string{
				"service.name":           "api-gateway",
				"deployment.environment": "production",
			},
		},
		{
			name: "internal_no_match",
			kind: ptrace.SpanKindInternal,
			spanAttrs: map[string]string{
				"custom.attr": "value",
			},
			resAttrs: map[string]string{
				"service.name": "my-service",
			},
		},
	}

	for _, bc := range benchCases {
		span, res := makeSpanAndResource(bc.kind, bc.spanAttrs, bc.resAttrs)
		b.Run(fmt.Sprintf("OperationNameV2/%s", bc.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = GetOTelOperationNameV2(span, res)
			}
		})
		b.Run(fmt.Sprintf("ResourceV2/%s", bc.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = GetOTelResourceV2(span, res)
			}
		})
		b.Run(fmt.Sprintf("SpanType/%s", bc.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = GetOTelSpanType(span, res)
			}
		})
		b.Run(fmt.Sprintf("Service/%s", bc.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = GetOTelService(span, res, true)
			}
		})
	}

	// Combined: simulate full per-span V2 processing (each function creates its own accessor)
	for _, bc := range benchCases {
		span, res := makeSpanAndResource(bc.kind, bc.spanAttrs, bc.resAttrs)
		b.Run(fmt.Sprintf("AllV2Functions/%s", bc.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = GetOTelService(span, res, true)
				_ = GetOTelSpanType(span, res)
				_ = GetOTelOperationNameV2(span, res)
				_ = GetOTelResourceV2(span, res)
			}
		})
	}

	// Combined with shared accessor: simulate full per-span V2 processing
	// with a single accessor reused across all functions via WithAccessor variants.
	for _, bc := range benchCases {
		span, res := makeSpanAndResource(bc.kind, bc.spanAttrs, bc.resAttrs)
		b.Run(fmt.Sprintf("AllV2SharedAccessor/%s", bc.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				accessor := semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes())
				_ = GetOTelServiceWithAccessor(accessor, true)
				_ = GetOTelSpanTypeWithAccessor(span, accessor)
				_ = GetOTelOperationNameV2WithAccessor(span, accessor)
				_ = GetOTelResourceV2WithAccessor(span, accessor)
			}
		})
	}
}
