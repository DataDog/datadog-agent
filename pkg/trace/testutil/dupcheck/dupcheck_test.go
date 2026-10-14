// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dupcheck

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

const (
	rawQuery        = "SELECT * FROM users WHERE id = 42"
	obfuscatedQuery = "SELECT * FROM users WHERE id = ?"
)

func sqlSpan(meta map[string]string) *pb.Span {
	return &pb.Span{Service: "dupcheck-service", Name: "dupcheck.operation", Resource: rawQuery, Type: "sql", TraceID: 1, SpanID: 2, Meta: meta}
}

// pairs returns the findings as "change rule location == location" strings.
func pairs(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, fmt.Sprintf("%s %s %s == %s", f.Change, f.Rule, f.Locations[0], f.Locations[1]))
	}
	return out
}

func TestCheckSpanCreatedAttributeCopyingTopLevel(t *testing.T) {
	in := sqlSpan(map[string]string{"db.type": "postgresql"})
	out := sqlSpan(map[string]string{"db.type": "postgresql", "sql.query": obfuscatedQuery})
	out.Resource = obfuscatedQuery

	assert.Equal(t, []string{"created dup span.meta[sql.query] == span.resource"}, pairs(CheckSpan(in, out)))
}

func TestCheckSpanOverwrittenAttribute(t *testing.T) {
	in := sqlSpan(map[string]string{"custom.a": "first-value", "custom.b": "second-value"})
	out := sqlSpan(map[string]string{"custom.a": "first-value", "custom.b": "first-value"})

	fs := CheckSpan(in, out)
	assert.Equal(t, []string{"overwritten dup span.meta[custom.b] == span.meta[custom.a]"}, pairs(fs))
	assert.Equal(t, "sql", fs[0].SpanType)
	assert.EqualValues(t, 2, fs[0].SpanID)
	assert.Equal(t, "first-value", fs[0].Value)
}

func TestCheckSpanBothChanged(t *testing.T) {
	in := sqlSpan(map[string]string{"custom.a": "first-value", "custom.b": "second-value"})
	out := sqlSpan(map[string]string{"custom.a": "third-value", "custom.b": "third-value"})

	assert.Equal(t, []string{"both-changed dup span.meta[custom.a] == span.meta[custom.b]"}, pairs(CheckSpan(in, out)))
}

func TestCheckSpanAllowsSDKDuplicates(t *testing.T) {
	// Both attributes held the raw query in the input and were obfuscated the
	// same way, along with the resource: the SDK sent the duplicate.
	in := sqlSpan(map[string]string{"sql.query": rawQuery, "db.statement": rawQuery})
	out := sqlSpan(map[string]string{"sql.query": obfuscatedQuery, "db.statement": obfuscatedQuery})
	out.Resource = obfuscatedQuery

	assert.Empty(t, CheckSpan(in, out))
}

func TestCheckSpanSkipsUnchangedAttributeVsTopLevel(t *testing.T) {
	// The agent derived the resource from an attribute it left alone.
	in := &pb.Span{Service: "dupcheck-service", Meta: map[string]string{"http.route": "/api/v1/users/{id}"}}
	out := &pb.Span{Service: "dupcheck-service", Resource: "/api/v1/users/{id}", Meta: map[string]string{"http.route": "/api/v1/users/{id}"}}

	assert.Empty(t, CheckSpan(in, out))
}

func TestCheckSpanSkipsTopLevelPairs(t *testing.T) {
	in := &pb.Span{Service: "dupcheck-service", Name: "dupcheck.operation"}
	out := &pb.Span{Service: "dupcheck-service", Name: "dupcheck-service", Resource: "dupcheck-service"}

	assert.Empty(t, CheckSpan(in, out))
}

func TestCheckSpanMinLenAndConcepts(t *testing.T) {
	in := sqlSpan(map[string]string{"http.request.method": "GET"})
	// "GET" is shorter than MinLen, but http.method and http.request.method
	// are synonyms in the semantics registry.
	out := sqlSpan(map[string]string{"http.request.method": "GET", "http.method": "GET", "custom.short": "GET"})

	assert.Equal(t, []string{"created concept-dup span.meta[http.method] == span.meta[http.request.method]"}, pairs(CheckSpan(in, out)))
}

func TestCheckSpanSkipsNonStringValues(t *testing.T) {
	in := sqlSpan(nil)
	out := sqlSpan(nil)
	out.Metrics = map[string]float64{"a": 12345678, "b": 12345678}
	out.MetaStruct = map[string][]byte{"a": []byte(rawQuery)}
	out.SpanEvents = []*pb.SpanEvent{{Name: "event", Attributes: map[string]*pb.AttributeAnyValue{
		"number": {Type: pb.AttributeAnyValue_INT_VALUE, IntValue: 12345678},
		"query":  {Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: rawQuery},
	}}}

	assert.Equal(t, []string{"created dup span.events[0].attributes[query] == span.resource"}, pairs(CheckSpan(in, out)))
}

func TestCheckSpanLinkAttributes(t *testing.T) {
	in := sqlSpan(nil)
	out := sqlSpan(nil)
	out.SpanLinks = []*pb.SpanLink{{Attributes: map[string]string{"link.name": "dupcheck-service"}}}

	assert.Equal(t, []string{"created dup span.links[0].attributes[link.name] == span.service"}, pairs(CheckSpan(in, out)))
}

func newSpanV1(strs *idx.StringTable, attrs map[string]string) *idx.InternalSpan {
	s := idx.NewInternalSpan(strs, &idx.Span{
		ServiceRef:  strs.Add("dupcheck-service"),
		ResourceRef: strs.Add(rawQuery),
		TypeRef:     strs.Add("sql"),
		SpanID:      2,
		Attributes:  make(map[uint32]*idx.AnyValue),
	})
	for k, v := range attrs {
		s.SetStringAttribute(k, v)
	}
	return s
}

func TestCheckSpanV1(t *testing.T) {
	in := newSpanV1(idx.NewStringTable(), map[string]string{"custom.a": "first-value"})
	out := newSpanV1(idx.NewStringTable(), map[string]string{"custom.a": "first-value", "custom.b": "first-value", "custom.c": rawQuery})
	out.SetFloat64Attribute("custom.number", 12345678)
	out.SetFloat64Attribute("custom.other_number", 12345678)
	out.SetVersion("first-value")

	assert.ElementsMatch(t, []string{
		"created dup span.attributes[custom.b] == span.attributes[custom.a]",
		"created dup span.attributes[custom.c] == span.resource",
		"created dup span.attributes[custom.b] == span.version",
	}, pairs(CheckSpanV1(in, out)))
}

func TestCheckTracerPayloadCrossLevel(t *testing.T) {
	newPayload := func(chunkTags map[string]string) *pb.TracerPayload {
		return &pb.TracerPayload{Chunks: []*pb.TraceChunk{{
			Tags:  chunkTags,
			Spans: []*pb.Span{{Service: "dupcheck-service", TraceID: 1, SpanID: 2, Meta: map[string]string{"_dd.p.dm": "-4", "custom.sdk": "-3"}}},
		}}}
	}
	in := newPayload(map[string]string{"custom.sdk": "-3"})
	out := &pb.AgentPayload{TracerPayloads: []*pb.TracerPayload{newPayload(map[string]string{"custom.sdk": "-3", "_dd.p.dm": "-4"})}}

	fs := CheckTracerPayload(in, out)
	assert.Equal(t, []string{"created cross-level chunk.tags[_dd.p.dm] == span.meta[_dd.p.dm]"}, pairs(fs))
	assert.Equal(t, "[v04] created cross-level chunk.tags[_dd.p.dm] == span.meta[_dd.p.dm] type=", Finding{Stage: "v04", Rule: fs[0].Rule, Change: fs[0].Change, Locations: fs[0].Locations}.Fingerprint())
}

func TestCheckTracerPayloadPayloadLevel(t *testing.T) {
	in := &pb.TracerPayload{Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{TraceID: 1, SpanID: 2}}}}}
	outTP := &pb.TracerPayload{
		Hostname: "dupcheck-hostname",
		Tags:     map[string]string{"custom.host": "dupcheck-hostname"},
		Chunks:   []*pb.TraceChunk{{Tags: map[string]string{"custom.chunk_host": "dupcheck-hostname"}, Spans: []*pb.Span{{TraceID: 1, SpanID: 2}}}},
	}
	out := &pb.AgentPayload{HostName: "dupcheck-hostname", TracerPayloads: []*pb.TracerPayload{outTP}}

	assert.ElementsMatch(t, []string{
		"created dup payload.tags[custom.host] == agent.hostname",
		"created dup payload.tags[custom.host] == payload.hostname",
		"created dup chunk.tags[custom.chunk_host] == agent.hostname",
		"created dup chunk.tags[custom.chunk_host] == payload.hostname",
		"created dup chunk.tags[custom.chunk_host] == payload.tags[custom.host]",
	}, pairs(CheckTracerPayload(in, out)))
}

func TestCheckTracerPayloadV1(t *testing.T) {
	newPayload := func(extra bool) *idx.InternalTracerPayload {
		strs := idx.NewStringTable()
		tp := &idx.InternalTracerPayload{Strings: strs, Attributes: make(map[uint32]*idx.AnyValue)}
		chunk := idx.NewInternalTraceChunk(strs, 2, "", make(map[uint32]*idx.AnyValue), []*idx.InternalSpan{newSpanV1(strs, map[string]string{"_dd.p.dm": "-4"})}, false, []byte{1}, 0)
		if extra {
			chunk.SetStringAttribute("_dd.p.dm", "-4")
		}
		tp.Chunks = []*idx.InternalTraceChunk{chunk}
		return tp
	}
	out := &pb.AgentPayload{IdxTracerPayloads: []*idx.TracerPayload{newPayload(true).ToProto()}}

	assert.Equal(t, []string{"created cross-level chunk.attributes[_dd.p.dm] == span.attributes[_dd.p.dm]"}, pairs(CheckTracerPayloadV1(newPayload(false), out)))
}

func TestCheckOTLPConversion(t *testing.T) {
	rs := ptrace.NewResourceSpans()
	rs.Resource().Attributes().PutStr("service.name", "dupcheck-service")
	otlpSpan := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	otlpSpan.SetSpanID(spanID64(2))
	otlpSpan.Attributes().PutStr("http.request.method", "GET")
	otlpSpan.Attributes().PutStr("http.route", "/api/v1/users/{id}")
	otlpSpan.Attributes().PutInt("http.response.status_code", 200)

	out := &pb.TracerPayload{Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{
		Service:  "dupcheck-service",
		Resource: "GET /api/v1/users/{id}",
		SpanID:   2,
		Meta: map[string]string{
			"service.name":        "dupcheck-service",
			"http.request.method": "GET",
			"http.method":         "GET",
			"http.route":          "/api/v1/users/{id}",
			"custom.route":        "/api/v1/users/{id}",
		},
		Metrics: map[string]float64{"http.response.status_code": 200},
	}}}}}

	assert.ElementsMatch(t, []string{
		"created concept-dup span.meta[http.method] == span.meta[http.request.method]",
		"created dup span.meta[custom.route] == span.meta[http.route]",
	}, pairs(CheckOTLPConversion(rs, out)))
}

func TestCheckOTLPConversionScopePrecedence(t *testing.T) {
	// The scope and the resource share a key: the conversion writes scope
	// attributes last, so the span carries the scope value, which the SDK
	// also sent under another span attribute.
	rs := ptrace.NewResourceSpans()
	rs.Resource().Attributes().PutStr("custom.shared", "dupcheck-resource-value")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().Attributes().PutStr("custom.shared", "dupcheck-scope-value")
	otlpSpan := ss.Spans().AppendEmpty()
	otlpSpan.SetSpanID(spanID64(2))
	otlpSpan.Attributes().PutStr("custom.copy", "dupcheck-scope-value")

	out := &pb.TracerPayload{Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{
		SpanID: 2,
		Meta:   map[string]string{"custom.shared": "dupcheck-scope-value", "custom.copy": "dupcheck-scope-value"},
	}}}}}

	assert.Empty(t, CheckOTLPConversion(rs, out))
}

func TestCheckOTLPConversionEmptySpanAttributeFallsBackToResource(t *testing.T) {
	// The span attribute is empty: the conversion fills it from the resource
	// attribute, whose value the SDK also sent under another span attribute.
	rs := ptrace.NewResourceSpans()
	rs.Resource().Attributes().PutStr("custom.shared", "dupcheck-long-value")
	otlpSpan := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	otlpSpan.SetSpanID(spanID64(2))
	otlpSpan.Attributes().PutStr("custom.shared", "")
	otlpSpan.Attributes().PutStr("custom.copy", "dupcheck-long-value")

	out := &pb.TracerPayload{Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{{
		SpanID: 2,
		Meta:   map[string]string{"custom.shared": "dupcheck-long-value", "custom.copy": "dupcheck-long-value"},
	}}}}}

	assert.Empty(t, CheckOTLPConversion(rs, out))
}

func TestFingerprintStripsIndexesAndValues(t *testing.T) {
	f1 := Finding{Stage: "v1", Rule: RuleDup, Change: ChangeCreated, Value: "a", SpanType: "sql", Locations: []Location{"span.events[0].attributes[k]", LocationResource}}
	f2 := f1
	f2.Value, f2.SpanID, f2.Context = "b", 3, "other"
	f2.Locations = []Location{"span.events[12].attributes[k]", LocationResource}

	assert.Equal(t, f1.Fingerprint(), f2.Fingerprint())
	assert.Equal(t, "[v1] created dup span.events[*].attributes[k] == span.resource type=sql", f1.Fingerprint())

	f3 := f1
	f3.Locations = []Location{MetaLocation("k"), MetaLocation(RandomKeyPrefix + "abc"), LocationService}
	assert.Equal(t, "[v1] created dup span.meta[k] == span.meta[dupcheck.random.*] type=sql", f3.Fingerprint())
}

func TestCheckSpanGroupsSDKDuplicatePartners(t *testing.T) {
	// The SDK sent the same value under custom.a, custom.b and the service: the
	// agent copying it once more is a single finding, listing the attributes
	// before the top-level field.
	in := &pb.Span{Service: "dupcheck-service", Meta: map[string]string{"custom.a": "dupcheck-service", "custom.b": "dupcheck-service"}}
	out := &pb.Span{Service: "dupcheck-service", Meta: map[string]string{"custom.a": "dupcheck-service", "custom.b": "dupcheck-service", "custom.c": "dupcheck-service"}}

	fs := CheckSpan(in, out)
	require.Len(t, fs, 1)
	assert.Equal(t, []Location{MetaLocation("custom.c"), MetaLocation("custom.a"), MetaLocation("custom.b"), LocationService}, fs[0].Locations)
	assert.Equal(t, ChangeCreated, fs[0].Change)
	assert.Equal(t, RuleDup, fs[0].Rule)
}

func TestCheckSpanSplitsGroupedPartnersByRule(t *testing.T) {
	// The SDK sent the same long value under http.request.method and
	// custom.other. The agent copying it to http.method, a synonym of
	// http.request.method, is a concept-dup with the synonym and a dup with
	// the other attribute.
	const value = "dupcheck-method"
	in := &pb.Span{Meta: map[string]string{"http.request.method": value, "custom.other": value}}
	out := &pb.Span{Meta: map[string]string{"http.request.method": value, "custom.other": value, "http.method": value}}

	fs := CheckSpan(in, out)
	require.Len(t, fs, 2)
	assert.Equal(t, RuleConceptDup, fs[0].Rule)
	assert.Equal(t, []Location{MetaLocation("http.method"), MetaLocation("http.request.method")}, fs[0].Locations)
	assert.Equal(t, RuleDup, fs[1].Rule)
	assert.Equal(t, []Location{MetaLocation("http.method"), MetaLocation("custom.other")}, fs[1].Locations)
}

// logRecorder records the lines logged by Report.Print.
type logRecorder struct {
	testing.TB
	lines []string
}

func (r *logRecorder) Helper() {}

func (r *logRecorder) Logf(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

// reportFinding returns a span finding of the given stage-independent fields.
func reportFinding(rule, change, value, typ string, spanID uint64, service, name string, locations ...Location) Finding {
	return Finding{
		Rule: rule, Change: change, Value: value, Locations: locations,
		SpanType: typ, SpanID: spanID, Service: service, Name: name,
		Context: fmt.Sprintf("config=default service=%s name=%s span_id=%d", service, name, spanID),
	}
}

func TestReportPrint(t *testing.T) {
	const (
		dm         = Location("span.meta[_dd.p.dm]")
		chunkDM    = Location("chunk.tags[_dd.p.dm]")
		env        = Location("span.meta[env]")
		library    = Location("span.meta[otel.library.name]")
		scope      = Location("span.meta[otel.scope.name]")
		customKey  = Location("span.attributes[custom.key]")
		deployment = Location("span.meta[deployment.environment]")
		deployName = Location("span.meta[deployment.environment.name]")
	)
	long := strings.Repeat("production-", 10)
	r := NewReport(42)
	r.Add("v04",
		reportFinding(RuleCrossLevel, ChangeCreated, "-4", "sql", 10, "svc-a", "op.a", chunkDM, dm),
		reportFinding(RuleCrossLevel, ChangeCreated, "-4", "web", 11, "svc-b", "op.b", chunkDM, dm),
	)
	r.Add("v1",
		reportFinding(RuleDup, ChangeOverwritten, "GET /users/{id}", "web", 20, "svc-v1", "op.v1", customKey, LocationResource),
		reportFinding(RuleDup, ChangeCreated, "GET /users/{id}", "web", 21, "svc-v1", "op.v1", customKey, LocationResource),
	)
	r.Add("otlp-conversion",
		reportFinding(RuleDup, ChangeCreated, "io.opentelemetry.http", "sql", 30, "svc-17", "server.request", library, scope),
		reportFinding(RuleDup, ChangeCreated, "io.opentelemetry.http", "sql", 31, "svc-18", "server.request", library, scope),
		reportFinding(RuleDup, ChangeCreated, "io.opentelemetry.http", "web", 32, "svc-19", "server.request", library, scope),
		reportFinding(RuleConceptDup, ChangeCreated, "prod", "sql", 40, "svc-02", "op.env", env, deployment),
		reportFinding(RuleDup, ChangeCreated, long, "sql", 41, "svc-01", "op.env", env, deployName),
		reportFinding(RuleDup, ChangeCreated, "production", "sql", 42, "svc-02", "op.env", env, deployName, MetaLocation(RandomKeyPrefix+"abc")),
		reportFinding(RuleDup, ChangeCreated, "svc-02-env", "web", 43, "svc-02", "op.env", env, MetaLocation(RandomKeyPrefix+"def"), LocationService),
		Finding{Rule: RuleCrossLevel, Change: ChangeCreated, Value: "edge", Locations: []Location{"chunk.tags[_dd.apm_mode]", "payload.tags[_dd.apm_mode]"}, Context: "config=default chunk trace_id=7"},
	)
	r.Add("otlp-processing",
		reportFinding(RuleCrossLevel, ChangeCreated, "-4", "sql", 50, "svc-c", "op.c", chunkDM, dm),
		reportFinding(RuleCrossLevel, ChangeOverwritten, "-4", "sql", 51, "svc-c", "op.c", chunkDM, dm),
		reportFinding(RuleCrossLevel, ChangeBothChanged, "-4", "sql", 52, "svc-c", "op.c", chunkDM, dm),
		reportFinding(RuleConceptDup, ChangeCreated, "users-db", "sql", 60, "svc-db", "op.db", MetaLocation("db.name"), MetaLocation("db.namespace")),
		reportFinding(RuleDup, ChangeCreated, "dupcheck.resource.x", "", 70, "svc-ev", "op.ev", "span.events[0].attributes[k]", LocationResource),
		reportFinding(RuleDup, ChangeCreated, "dupcheck.resource.x", "", 71, "svc-ev", "op.ev", "span.events[2].attributes[k]", LocationResource),
	)

	rec := &logRecorder{TB: t}
	r.Print(rec, false)

	require.Len(t, rec.lines, 1, "the report must be a single log call")
	assert.Equal(t, `
Trace payload duplicates: 7 issues, 18 occurrences (seed 42)

1. chunk.tags[_dd.p.dm] repeats span.attr[_dd.p.dm]
   created by: v04,
               otlp-processing (created, overwrote existing value, changed both values)
   occurrences: 5 on 2 span types
   example: config=default service=svc-a name=op.a
            value="-4"

2. span.attr[env] repeats another location
   created by: otlp-conversion
   partners: deployment.environment.name (2), dupcheck.random.* (2),
             deployment.environment (1, synonym), +1 more
   occurrences: 4 on 2 span types
   example: config=default service=svc-01 name=op.env
            value="production-production-production-production-production-production-..."

3. span.attr[otel.library.name] repeats span.attr[otel.scope.name]
   created by: otlp-conversion
   occurrences: 3 on 2 span types
   example: config=default service=svc-17 name=server.request
            value="io.opentelemetry.http"

4. span.attr[custom.key] repeats span.resource
   created by: v1 (created, overwrote existing value)
   occurrences: 2 on 1 span type
   example: config=default service=svc-v1 name=op.v1
            value="GET /users/{id}"

5. span.events[*].attributes[k] repeats span.resource
   created by: otlp-processing
   occurrences: 2 on 1 span type
   example: config=default service=svc-ev name=op.ev
            value="dupcheck.resource.x"

6. chunk.tags[_dd.apm_mode] repeats payload.tags[_dd.apm_mode]
   created by: otlp-conversion
   occurrences: 1
   example: config=default chunk trace_id=7
            value="edge"

7. span.attr[db.name] repeats span.attr[db.namespace] (semantic synonyms)
   created by: otlp-processing
   occurrences: 1 on 1 span type
   example: config=default service=svc-db name=op.db
            value="users-db"

Run with -dupcheck.verbose to list every occurrence group.`, rec.lines[0])
	assert.False(t, t.Failed())
}

func TestReportSummary(t *testing.T) {
	env := MetaLocation("env")
	r := NewReport(5)
	r.Add("v04",
		reportFinding(RuleDup, ChangeCreated, "prod-env", "sql", 1, "svc", "op", env, MetaLocation("deployment.environment")),
		reportFinding(RuleDup, ChangeOverwritten, "prod-env", "sql", 2, "svc", "op", env, MetaLocation("deployment.environment.name")),
	)
	r.Add("v1", reportFinding(RuleDup, ChangeCreated, "GET /users", "web", 3, "svc", "op", AttributeLocation("custom"), LocationResource))

	s := r.Summary()
	assert.Equal(t, int64(5), s.Seed)
	assert.Equal(t, 3, s.Occurrences)
	assert.Equal(t, []SummaryIssue{
		{
			Subject:   "span.attr[env]",
			Title:     "span.attr[env] repeats another location",
			Count:     2,
			CreatedBy: []string{"v04 (created, overwrote existing value)"},
			Partners:  map[string]int{"span.attr[deployment.environment]": 1, "span.attr[deployment.environment.name]": 1},
		},
		{
			Subject:   "span.attr[custom]",
			Title:     "span.attr[custom] repeats span.resource",
			Count:     1,
			CreatedBy: []string{"v1"},
			Partners:  map[string]int{"span.resource": 1},
		},
	}, s.Issues)
	assert.Equal(t, r.render(false), s.Text)
	assert.Empty(t, NewReport(1).Summary().Issues)
	assert.NotNil(t, NewReport(1).Summary().Issues, "no issue must encode as an empty list")
}

func TestReportPrintVerbose(t *testing.T) {
	r := NewReport(42)
	long := strings.Repeat("x", 100)
	r.Add("v04",
		Finding{Rule: RuleDup, Change: ChangeCreated, Value: long, Locations: []Location{"span.meta[b]", LocationResource}, SpanType: "sql", Context: "first"},
		Finding{Rule: RuleDup, Change: ChangeCreated, Value: "other", Locations: []Location{"span.meta[b]", LocationResource}, SpanType: "sql", Context: "second"},
	)
	r.Add("v1", Finding{Rule: RuleDup, Change: ChangeOverwritten, Value: "value", Locations: []Location{"span.attributes[a]", LocationName, "span.attributes[c]", LocationService}, SpanType: "web", Context: "third"})

	rec := &logRecorder{TB: t}
	r.Print(rec, true)

	require.Len(t, rec.lines, 2)
	assert.NotContains(t, rec.lines[0], "-dupcheck.verbose")
	assert.Equal(t, "\n"+strings.Join([]string{
		"dupcheck: 2 distinct findings, 3 occurrences (seed 42)",
		`dupcheck: [v04] created dup span.meta[b] == span.resource type=sql  count=2  e.g. first value="` + strings.Repeat("x", maxPrintedValueLen) + `..."`,
		`dupcheck: [v1] overwritten dup span.attributes[a] == span.name type=web  count=1  e.g. third value="value" also=span.attributes[c],span.service`,
	}, "\n"), rec.lines[1])
}

func TestReportPrintWithoutFindings(t *testing.T) {
	rec := &logRecorder{TB: t}
	NewReport(3).Print(rec, false)
	assert.Equal(t, []string{"\nTrace payload duplicates: 0 issues, 0 occurrences (seed 3)"}, rec.lines)
}

func TestReportExampleIsDeterministic(t *testing.T) {
	findings := []Finding{
		{Rule: RuleDup, Change: ChangeCreated, Value: "value-2", Locations: []Location{"span.meta[a]", LocationResource}, Context: "config=b service=svc-b name= span_id=1", SpanID: 1, Service: "svc-b"},
		{Rule: RuleDup, Change: ChangeCreated, Value: "value-1", Locations: []Location{"span.meta[a]", LocationResource}, Context: "config=a service=svc-a name= span_id=3", SpanID: 3, Service: "svc-a"},
		{Rule: RuleDup, Change: ChangeCreated, Value: "value-0", Locations: []Location{"span.meta[a]", LocationResource}, Context: "config=a service=svc-a name= span_id=3", SpanID: 3, Service: "svc-a"},
	}
	printReport := func(fs ...Finding) []string {
		r := NewReport(1)
		r.Add("v04", fs...)
		rec := &logRecorder{TB: t}
		r.Print(rec, true)
		return rec.lines
	}
	want := printReport(findings...)
	require.Len(t, want, 2)
	assert.Contains(t, want[0], "example: config=a service=svc-a name=\n            value=\"value-0\"")
	assert.Contains(t, want[1], `e.g. config=a service=svc-a name= span_id=3 value="value-0"`)
	assert.Equal(t, want, printReport(findings[2], findings[0], findings[1]))
	assert.Equal(t, want, printReport(findings[1], findings[2], findings[0]))
}

func TestReportSubjectIsTheAttribute(t *testing.T) {
	// When the agent created a top-level field and changed an attribute to
	// the same value, the attribute is the subject of the issue.
	r := NewReport(1)
	r.Add("otlp-conversion",
		reportFinding(RuleDup, ChangeCreated, "dupcheck-value", "web", 1, "svc", "op", MetaLocation("k"), LocationResource),
		reportFinding(RuleDup, ChangeCreated, "dupcheck-value", "web", 2, "svc", "op", LocationResource, MetaLocation("k")),
	)
	rec := &logRecorder{TB: t}
	r.Print(rec, false)

	require.Len(t, rec.lines, 1)
	assert.Contains(t, rec.lines[0], "Trace payload duplicates: 1 issue, 2 occurrences (seed 1)\n\n1. span.attr[k] repeats span.resource\n")
	assert.Contains(t, rec.lines[0], "occurrences: 2 on 1 span type\n")
}

func TestReportPartnerNames(t *testing.T) {
	assert.Equal(t, "service", shortLocation(string(LocationService)))
	assert.Equal(t, "db.name", shortLocation(string(MetaLocation("db.name"))))
	assert.Equal(t, "db.name", shortLocation(string(AttributeLocation("db.name"))))
	assert.Equal(t, "span.meta[env]", shortLocation(string(MetaLocation("env"))), "a bare env would read as span.env")
	assert.Equal(t, "chunk.tags[_dd.p.dm]", shortLocation("chunk.tags[_dd.p.dm]"))
	assert.Equal(t, "span.events[*].attributes[k]", shortLocation("span.events[*].attributes[k]"))

	// Partners whose short names collide are printed in full.
	is := &reportIssue{partners: map[string]*reportPartner{
		"span.meta[k]":       {location: "span.meta[k]", count: 2},
		"span.attributes[k]": {location: "span.attributes[k]", count: 1},
		"span.name":          {location: "span.name", count: 1},
	}}
	assert.Equal(t, []string{"span.meta[k] (2)", "span.attributes[k] (1)", "name (1)"}, is.partnerList())
}

func TestFormatCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 6812: "6,812", 1234567: "1,234,567"} {
		assert.Equal(t, want, formatCount(n))
	}
}

func TestNewCorpus(t *testing.T) {
	c := NewCorpus(7)
	assert.Equal(t, c.Traces, NewCorpus(7).Traces, "the corpus must be reproducible from its seed")
	assert.NotEqual(t, c.Traces, NewCorpus(8).Traces)

	types := make(map[string]bool)
	keys := make(map[string]int)
	for _, tr := range c.Traces {
		require.NotEmpty(t, tr.Spans)
		for i, s := range tr.Spans {
			types[s.Type] = true
			assert.Equal(t, tr.Spans[0].TraceID, s.TraceID)
			if i > 0 {
				assert.Equal(t, tr.Spans[0].SpanID, s.ParentID)
			}
			for k := range s.Attributes {
				keys[k]++
			}
		}
	}
	for _, typ := range CorpusSpanTypes {
		assert.True(t, types[typ], "span type %q missing from the corpus", typ)
	}
	for _, k := range corpusKeys() {
		assert.Equal(t, len(CorpusSpanTypes)*(shapePasses-1), keys[k], "key %q must be set once per span type and pass but one", k)
	}
}

func TestNewCorpusValueShapes(t *testing.T) {
	c := NewCorpus(7)
	shapes := make(map[string]map[string]map[valueShape]int)
	for _, tr := range c.Traces {
		for _, s := range tr.Spans {
			if shapes[s.Type] == nil {
				shapes[s.Type] = make(map[string]map[valueShape]int)
			}
			topLevel := []string{s.Service, s.Name, s.Resource, s.Type}
			for k, shape := range s.shapes {
				v, ok := s.Attributes[k]
				require.True(t, ok, "key %q of shape %d not set", k, shape)
				switch shape {
				case shapeTopLevel:
					assert.Contains(t, topLevel, v, "key %q must copy a top-level field", k)
				case shapeUnique:
					for _, tv := range topLevel {
						assert.False(t, sameTemplate(v, tv), "key %q must not share the template of top-level field %q", k, tv)
					}
					for other, ov := range s.Attributes {
						assert.False(t, other != k && sameTemplate(v, ov), "key %q must not share its template with attribute %q", k, other)
					}
				case shapeOtherAttribute:
					assert.True(t, slices.ContainsFunc(slices.Collect(maps.Keys(s.Attributes)), func(other string) bool {
						return strings.HasPrefix(other, RandomKeyPrefix) && s.Attributes[other] == v
					}), "key %q must copy a random attribute", k)
				}
				if shapes[s.Type][k] == nil {
					shapes[s.Type][k] = make(map[valueShape]int)
				}
				shapes[s.Type][k][shape]++
			}
		}
	}
	for _, typ := range CorpusSpanTypes {
		for _, k := range corpusKeys() {
			got := shapes[typ][k]
			// The key is absent from the spans of one pass.
			assert.Equal(t, map[valueShape]int{shapeUnique: 1, shapeTopLevel: 1, shapeOtherAttribute: 1}, got, "type %q key %q", typ, k)
		}
	}
	for key := range len(corpusKeys()) {
		var all []valueShape
		for pass := range shapePasses {
			all = append(all, keyShape(pass, key))
		}
		assert.ElementsMatch(t, []valueShape{shapeAbsent, shapeUnique, shapeTopLevel, shapeOtherAttribute}, all)
	}
}

func TestSameTemplate(t *testing.T) {
	assert.True(t, sameTemplate("SELECT * FROM users WHERE id = 12", "SELECT * FROM users WHERE id = 345"))
	assert.True(t, sameTemplate("404", "404"))
	assert.False(t, sameTemplate("SELECT * FROM users WHERE id = 12", "DELETE FROM sessions WHERE expires < 12"))
	assert.False(t, sameTemplate("200", "404"), "values made only of digits are compared as is")
}

func TestCorpusRendering(t *testing.T) {
	c := NewCorpus(7)
	batch := c.Traces[:2]
	spans := len(batch[0].Spans) + len(batch[1].Spans)

	tp := TracerPayload(batch)
	require.Len(t, tp.Chunks, 2)
	assert.Equal(t, batch[0].Spans[0].Resource, tp.Chunks[0].Spans[0].Resource)
	assert.Equal(t, batch[0].Spans[0].Attributes, tp.Chunks[0].Spans[0].Meta)

	tpV1 := TracerPayloadV1(batch)
	require.Len(t, tpV1.Chunks, 2)
	assert.Equal(t, batch[0].Spans[0].Resource, tpV1.Chunks[0].Spans[0].Resource())
	assert.Equal(t, spans, len(tpV1.Chunks[0].Spans)+len(tpV1.Chunks[1].Spans))

	rs := ResourceSpans(batch)
	require.Equal(t, 1, rs.ScopeSpans().Len())
	assert.Equal(t, spans, rs.ScopeSpans().At(0).Spans().Len())
	attrs := rs.ScopeSpans().At(0).Spans().At(0).Attributes()
	for k, want := range map[string]string{"service.name": batch[0].Spans[0].Service, "resource.name": batch[0].Spans[0].Resource, "span.type": batch[0].Spans[0].Type} {
		v, ok := attrs.Get(k)
		require.True(t, ok, k)
		assert.Equal(t, want, v.Str(), k)
	}
}

func TestNormalizeLocation(t *testing.T) {
	// Event and link indexes are stripped so equivalent locations merge.
	assert.Equal(t, "span.events[*].attributes[k]", normalizeLocation("span.events[3].attributes[k]"))
	assert.Equal(t, "span.links[*].attributes[k]", normalizeLocation("span.links[0].attributes[k]"))
	// Attribute keys made of digits are distinct locations and are kept.
	assert.Equal(t, "span.meta[123]", normalizeLocation("span.meta[123]"))
	assert.NotEqual(t, normalizeLocation("span.meta[123]"), normalizeLocation("span.meta[456]"))
	// Random corpus keys merge.
	assert.Equal(t, "span.meta["+RandomKeyPrefix+"*]", normalizeLocation(Location("span.meta["+RandomKeyPrefix+"abc]")))
}

func TestReportMergesMirrorIssuesAndFormats(t *testing.T) {
	library, scope := MetaLocation("otel.library.name"), MetaLocation("otel.scope.name")
	r := NewReport(1)
	// Which of the two keys the agent wrote depends on the input: either way
	// it is one issue.
	r.Add("otlp-conversion",
		reportFinding(RuleDup, ChangeCreated, "io.opentelemetry.http", "web", 1, "svc", "op", library, scope),
		reportFinding(RuleDup, ChangeCreated, "io.opentelemetry.http", "web", 2, "svc", "op", library, scope),
		reportFinding(RuleDup, ChangeCreated, "io.opentelemetry.http", "sql", 3, "svc", "op", scope, library),
	)
	// The same attribute in the v0.4 and v1 formats is one issue.
	r.Add("v04", reportFinding(RuleDup, ChangeBothChanged, "SELECT ?", "sql", 4, "svc", "op", MetaLocation("db.statement"), LocationResource))
	r.Add("v1", reportFinding(RuleDup, ChangeBothChanged, "SELECT ?", "sql", 5, "svc", "op", AttributeLocation("db.statement"), LocationResource))
	rec := &logRecorder{TB: t}
	r.Print(rec, false)

	require.Len(t, rec.lines, 1)
	assert.Equal(t, `
Trace payload duplicates: 2 issues, 5 occurrences (seed 1)

1. span.attr[otel.library.name] repeats span.attr[otel.scope.name]
   created by: otlp-conversion
   occurrences: 3 on 2 span types
   example: config=default service=svc name=op
            value="io.opentelemetry.http"

2. span.attr[db.statement] repeats span.resource
   created by: v04 (changed both values), v1 (changed both values)
   occurrences: 2 on 1 span type
   example: config=default service=svc name=op
            value="SELECT ?"

Run with -dupcheck.verbose to list every occurrence group.`, rec.lines[0])
}
