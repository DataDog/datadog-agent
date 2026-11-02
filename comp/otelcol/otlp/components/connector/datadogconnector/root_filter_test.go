// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogconnector

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

type testSpan struct {
	traceID, spanID, parentID byte
	kind                      ptrace.SpanKind
	attrs                     map[string]string
}

var (
	healthcheckAttrs = map[string]string{"http.request.method": "GET", "http.route": "/healthcheck"}
	usersAttrs       = map[string]string{"http.request.method": "GET", "http.route": "/users"}
	pingAttrs        = map[string]string{"http.request.method": "GET", "http.route": "/internal/ping"}
	dbAttrs          = map[string]string{"db.system": "postgresql", "db.statement": "SELECT 1"}
)

// newTestTraces returns traces with one resource per element of resources.
func newTestTraces(resources ...[]testSpan) ptrace.Traces {
	now := time.Now()
	traces := ptrace.NewTraces()
	for _, resource := range resources {
		spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
		for _, s := range resource {
			span := spans.AppendEmpty()
			span.SetTraceID(pcommon.TraceID{15: s.traceID})
			span.SetSpanID(pcommon.SpanID{7: s.spanID})
			if s.parentID != 0 {
				span.SetParentSpanID(pcommon.SpanID{7: s.parentID})
			}
			span.SetKind(s.kind)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-time.Second)))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(now))
			for k, v := range s.attrs {
				span.Attributes().PutStr(k, v)
			}
		}
	}
	return traces
}

func newTestRootFilterConf(t *testing.T) *config.AgentConfig {
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	conf := config.New()
	conf.OTLPReceiver.AttributesTranslator = attributesTranslator
	return conf
}

func TestIgnoredTraces(t *testing.T) {
	rootFilter := newRootFilter([]string{"(GET|POST) /health"}, nil, nil, nil, nil)
	for _, tt := range []struct {
		name        string
		traces      ptrace.Traces
		wantIgnored []indexedTrace
	}{
		{
			name: "matching root ignores its trace only",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckAttrs},
				{1, 2, 1, ptrace.SpanKindClient, dbAttrs},
				{2, 3, 0, ptrace.SpanKindServer, usersAttrs},
			}),
			wantIgnored: []indexedTrace{{0, 1}},
		},
		{
			name: "root is looked up per resource",
			traces: newTestTraces(
				[]testSpan{{1, 1, 0, ptrace.SpanKindServer, healthcheckAttrs}},
				[]testSpan{{1, 2, 1, ptrace.SpanKindServer, pingAttrs}},
			),
			wantIgnored: []indexedTrace{{0, 1}},
		},
		{
			name: "span whose parent is not in its resource is a root",
			traces: newTestTraces([]testSpan{
				{1, 2, 1, ptrace.SpanKindServer, healthcheckAttrs},
				{1, 3, 2, ptrace.SpanKindClient, dbAttrs},
			}),
			wantIgnored: []indexedTrace{{0, 1}},
		},
		{
			name: "matching child does not ignore its trace",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, usersAttrs},
				{1, 2, 1, ptrace.SpanKindServer, healthcheckAttrs},
			}),
		},
		{
			name: "span without parent is the root rather than a span whose parent is missing",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, usersAttrs},
				{1, 3, 9, ptrace.SpanKindServer, healthcheckAttrs},
			}),
		},
		{
			name: "last span without parent is the root",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckAttrs},
				{1, 2, 0, ptrace.SpanKindServer, usersAttrs},
			}),
		},
		{
			name: "first span whose parent is missing is the root",
			traces: newTestTraces([]testSpan{
				{1, 2, 9, ptrace.SpanKindServer, usersAttrs},
				{1, 3, 9, ptrace.SpanKindServer, healthcheckAttrs},
			}),
		},
		{
			name: "first span whose parent is missing is the root when it matches",
			traces: newTestTraces([]testSpan{
				{1, 2, 9, ptrace.SpanKindServer, healthcheckAttrs},
				{1, 3, 9, ptrace.SpanKindServer, usersAttrs},
			}),
			wantIgnored: []indexedTrace{{0, 1}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ignored := ignoredTraces(tt.traces, newTestRootFilterConf(t), rootFilter)
			assert.ElementsMatch(t, tt.wantIgnored, indexedTraces(tt.traces, ignored))
		})
	}
}

func TestIgnoredRootsFilter(t *testing.T) {
	conf := newTestRootFilterConf(t)
	rootFilter := newRootFilter([]string{"(GET|POST) /health"}, nil, nil, nil, nil)

	t.Run("nothing ignored returns no filter", func(t *testing.T) {
		traces := newTestTraces([]testSpan{{1, 1, 0, ptrace.SpanKindServer, usersAttrs}})
		assert.Nil(t, ignoredRootsFilter(traces, conf, rootFilter))
	})

	t.Run("filter excludes the spans of ignored resource traces", func(t *testing.T) {
		traces := newTestTraces(
			[]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckAttrs},
				{1, 2, 1, ptrace.SpanKindClient, dbAttrs},
				{2, 3, 0, ptrace.SpanKindServer, usersAttrs},
			},
			[]testSpan{{1, 4, 1, ptrace.SpanKindServer, pingAttrs}},
		)
		spanFilter := ignoredRootsFilter(traces, conf, rootFilter)
		require.NotNil(t, spanFilter)

		kept := map[pcommon.SpanID]bool{}
		for i := 0; i < traces.ResourceSpans().Len(); i++ {
			rspans := traces.ResourceSpans().At(i)
			forEachSpan(rspans, func(span ptrace.Span, _ pcommon.InstrumentationScope) {
				kept[span.SpanID()] = spanFilter(rspans.Resource(), span)
			})
		}
		assert.Equal(t, map[pcommon.SpanID]bool{{7: 1}: false, {7: 2}: false, {7: 3}: true, {7: 4}: true}, kept)
	})
}

func TestIgnoredTracesByTags(t *testing.T) {
	withAttrs := func(base map[string]string, extra map[string]string) map[string]string {
		attrs := map[string]string{}
		for k, v := range base {
			attrs[k] = v
		}
		for k, v := range extra {
			attrs[k] = v
		}
		return attrs
	}
	healthcheckRoot := withAttrs(healthcheckAttrs, map[string]string{"http.useragent": "kube-probe/1.30"})

	for _, tt := range []struct {
		name             string
		traces           ptrace.Traces
		resourceAttrs    map[string]string
		requireTags      []*config.Tag
		rejectTags       []*config.Tag
		requireTagsRegex []*config.TagRegex
		rejectTagsRegex  []*config.TagRegex
		wantIgnored      []indexedTrace
	}{
		{
			name: "rejected root tag ignores its trace only",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckRoot},
				{1, 2, 1, ptrace.SpanKindClient, dbAttrs},
				{2, 3, 0, ptrace.SpanKindServer, usersAttrs},
			}),
			rejectTags:  []*config.Tag{{K: "http.useragent", V: "kube-probe/1.30"}},
			wantIgnored: []indexedTrace{{0, 1}},
		},
		{
			name: "rejected key without value matches any value",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckRoot},
			}),
			rejectTags:  []*config.Tag{{K: "http.useragent"}},
			wantIgnored: []indexedTrace{{0, 1}},
		},
		{
			name: "rejected tag on a child does not ignore its trace",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, usersAttrs},
				{1, 2, 1, ptrace.SpanKindClient, healthcheckRoot},
			}),
			rejectTags: []*config.Tag{{K: "http.useragent"}},
		},
		{
			name: "rejected regex root tag ignores its trace",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckRoot},
				{2, 2, 0, ptrace.SpanKindServer, usersAttrs},
			}),
			rejectTagsRegex: []*config.TagRegex{{K: "http.useragent", V: regexp.MustCompile("^kube-probe/")}},
			wantIgnored:     []indexedTrace{{0, 1}},
		},
		{
			name: "root without a required tag ignores its trace",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckRoot},
				{2, 2, 0, ptrace.SpanKindServer, usersAttrs},
			}),
			requireTags: []*config.Tag{{K: "http.useragent"}},
			wantIgnored: []indexedTrace{{0, 2}},
		},
		{
			name: "root whose required tag does not match a regex ignores its trace",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, healthcheckRoot},
				{2, 2, 0, ptrace.SpanKindServer, withAttrs(usersAttrs, map[string]string{"http.useragent": "curl/8.0"})},
			}),
			requireTagsRegex: []*config.TagRegex{{K: "http.useragent", V: regexp.MustCompile("^curl/")}},
			wantIgnored:      []indexedTrace{{0, 1}},
		},
		{
			name: "resource attributes are root tags",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, usersAttrs},
			}),
			resourceAttrs: map[string]string{"team": "payments"},
			rejectTags:    []*config.Tag{{K: "team", V: "payments"}},
			wantIgnored:   []indexedTrace{{0, 1}},
		},
		{
			name: "container tags moved out of the resource are not root tags",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, usersAttrs},
			}),
			resourceAttrs: map[string]string{"datadog.container.tag.team": "payments"},
			rejectTags:    []*config.Tag{{K: "datadog.container.tag.team"}},
		},
		{
			name: "env is normalized before matching",
			traces: newTestTraces([]testSpan{
				{1, 1, 0, ptrace.SpanKindServer, usersAttrs},
			}),
			resourceAttrs: map[string]string{"deployment.environment.name": "Staging"},
			rejectTags:    []*config.Tag{{K: "env", V: "staging"}},
			wantIgnored:   []indexedTrace{{0, 1}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < tt.traces.ResourceSpans().Len(); i++ {
				for k, v := range tt.resourceAttrs {
					tt.traces.ResourceSpans().At(i).Resource().Attributes().PutStr(k, v)
				}
			}
			rootFilter := newRootFilter(nil, tt.requireTags, tt.rejectTags, tt.requireTagsRegex, tt.rejectTagsRegex)
			ignored := ignoredTraces(tt.traces, newTestRootFilterConf(t), rootFilter)
			assert.ElementsMatch(t, tt.wantIgnored, indexedTraces(tt.traces, ignored))
		})
	}
}

func TestIgnoredTracesByResourceAndTags(t *testing.T) {
	traces := newTestTraces([]testSpan{
		{1, 1, 0, ptrace.SpanKindServer, healthcheckAttrs},
		{2, 2, 0, ptrace.SpanKindServer, map[string]string{"http.request.method": "GET", "http.route": "/users", "http.useragent": "kube-probe/1.30"}},
		{3, 3, 0, ptrace.SpanKindServer, usersAttrs},
	})
	rootFilter := newRootFilter([]string{"(GET|POST) /health"}, nil, []*config.Tag{{K: "http.useragent"}}, nil, nil)
	ignored := ignoredTraces(traces, newTestRootFilterConf(t), rootFilter)
	assert.ElementsMatch(t, []indexedTrace{{0, 1}, {0, 2}}, indexedTraces(traces, ignored))
}

func TestNewRootFilter(t *testing.T) {
	assert.Nil(t, newRootFilter(nil, nil, nil, nil, nil))
	assert.Nil(t, newRootFilter([]string{}, []*config.Tag{}, nil, nil, nil))
	assert.NotNil(t, newRootFilter([]string{"x"}, nil, nil, nil, nil))
	assert.NotNil(t, newRootFilter(nil, nil, nil, nil, []*config.TagRegex{{K: "k"}}))
}

// indexedTrace is a resourceTrace with the index of its resource, for readable test expectations.
type indexedTrace struct {
	resource int
	traceID  uint64
}

func indexedTraces(traces ptrace.Traces, ignored map[resourceTrace]struct{}) []indexedTrace {
	var indexed []indexedTrace
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		for rt := range ignored {
			if rt.resource == traces.ResourceSpans().At(i).Resource() {
				indexed = append(indexed, indexedTrace{i, rt.traceID})
			}
		}
	}
	return indexed
}
