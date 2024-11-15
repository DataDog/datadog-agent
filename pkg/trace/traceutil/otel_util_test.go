// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.opentelemetry.io/otel/metric/noop"
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
	rattrs.PutStr(semconv.AttributeHostName, "host1")
	rattrs.PutStr(semconv.AttributeServiceName, "svc1")
	rattrs.PutStr(semconv.AttributeDeploymentEnvironment, "env1")

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
	rattrs.PutStr(semconv.AttributeHostName, "host2")
	rattrs.PutStr(semconv.AttributeServiceName, "svc2")
	rattrs.PutStr(semconv.AttributeDeploymentEnvironment, "env2")

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
	rspans.Resource().Attributes().PutStr(semconv.AttributeServiceName, "svc1")
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
	rspans2.Resource().Attributes().PutStr(semconv.AttributeServiceName, "svc2")
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
			rattrs:   map[string]string{semconv.AttributeDBSystem: "redis"},
			expected: "cache",
		},
		{
			name:     "memcached span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{semconv.AttributeDBSystem: "memcached"},
			expected: "cache",
		},
		{
			name:     "other db client span",
			spanKind: ptrace.SpanKindClient,
			rattrs:   map[string]string{semconv.AttributeDBSystem: "postgres"},
			expected: "db",
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetKind(tt.spanKind)
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}
			actual := GetOTelSpanType(span, res)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetOTelService(t *testing.T) {
	for _, tt := range []struct {
		name      string
		rattrs    map[string]string
		normalize bool
		expected  string
	}{
		{
			name:     "service not set",
			expected: "otlpresourcenoservicename",
		},
		{
			name:     "normal service",
			rattrs:   map[string]string{semconv.AttributeServiceName: "svc"},
			expected: "svc",
		},
		{
			name:      "truncate long service",
			rattrs:    map[string]string{semconv.AttributeServiceName: strings.Repeat("a", MaxServiceLen+1)},
			normalize: true,
			expected:  strings.Repeat("a", MaxServiceLen),
		},
		{
			name:      "invalid service",
			rattrs:    map[string]string{semconv.AttributeServiceName: "%$^"},
			normalize: true,
			expected:  DefaultServiceName,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
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
			sattrs:     map[string]string{semconv.AttributeHTTPMethod: "GET", semconv.AttributeHTTPRoute: "/"},
			expectedV1: "GET /",
			expectedV2: "GET",
		},
		{
			name:       "truncate long resource",
			sattrs:     map[string]string{"resource.name": strings.Repeat("a", MaxResourceLen+1)},
			normalize:  true,
			expectedV1: strings.Repeat("a", MaxResourceLen),
			expectedV2: strings.Repeat("a", MaxResourceLen),
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
			expected:           DefaultSpanName,
		},
		{
			name:      "normalize invalid operation name",
			sattrs:    map[string]string{"operation.name": "%$^"},
			normalize: true,
			expected:  DefaultSpanName,
		},
		{
			name:      "truncate long operation name",
			sattrs:    map[string]string{"operation.name": strings.Repeat("a", MaxNameLen+1)},
			normalize: true,
			expected:  strings.Repeat("a", MaxNameLen),
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

func TestGetOTelHostname(t *testing.T) {
	for _, tt := range []struct {
		name         string
		rattrs       map[string]string
		sattrs       map[string]string
		fallbackHost string
		expected     string
	}{
		{
			name:     "datadog.host.name",
			rattrs:   map[string]string{"datadog.host.name": "test-host"},
			expected: "test-host",
		},
		{
			name:     "_dd.hostname",
			rattrs:   map[string]string{"_dd.hostname": "test-host"},
			expected: "test-host",
		},
		{
			name:         "fallback hostname",
			fallbackHost: "test-host",
			expected:     "test-host",
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
			set := componenttest.NewNopTelemetrySettings()
			set.MeterProvider = noop.NewMeterProvider()
			tr, err := attributes.NewTranslator(set)
			assert.NoError(t, err)
			actual := GetOTelHostname(span, res, tr, tt.fallbackHost)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetOTelStatusCode(t *testing.T) {
	span := ptrace.NewSpan()
	span.SetName("span_name")
	assert.Equal(t, uint32(0), GetOTelStatusCode(span))
	span.Attributes().PutInt(semconv.AttributeHTTPStatusCode, 200)
	assert.Equal(t, uint32(200), GetOTelStatusCode(span))
	span.Attributes().Remove(semconv.AttributeHTTPStatusCode)
	span.Attributes().PutInt("http.response.status_code", 404)
	assert.Equal(t, uint32(404), GetOTelStatusCode(span))
}

func TestGetOTelContainerTags(t *testing.T) {
	res := pcommon.NewResource()
	res.Attributes().PutStr(semconv.AttributeContainerID, "cid")
	res.Attributes().PutStr(semconv.AttributeContainerName, "cname")
	res.Attributes().PutStr(semconv.AttributeContainerImageName, "ciname")
	res.Attributes().PutStr(semconv.AttributeContainerImageTag, "citag")
	res.Attributes().PutStr("az", "my-az")
	assert.Contains(t, GetOTelContainerTags(res.Attributes(), []string{"az", semconv.AttributeContainerID, semconv.AttributeContainerName, semconv.AttributeContainerImageName, semconv.AttributeContainerImageTag}), "container_id:cid", "container_name:cname", "image_name:ciname", "image_tag:citag", "az:my-az")
	assert.Contains(t, GetOTelContainerTags(res.Attributes(), []string{"az"}), "az:my-az")
}
