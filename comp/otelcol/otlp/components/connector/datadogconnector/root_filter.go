// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogconnector

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// keyStatsComputed marks spans whose APM stats were computed by the client; the connector skips their resource.
const keyStatsComputed = "_dd.stats_computed"

// resourceTrace identifies the spans of a trace within one resource, which the trace-agent processes as a chunk.
type resourceTrace struct {
	resource int
	traceID  pcommon.TraceID
}

// dropIgnoredTraces removes the traces whose root resource rootFilter denies, like the trace-agent does
// with apm_config.ignore_resources. traces is read-only, so it is returned as is when no trace is ignored,
// and copied without the ignored spans otherwise.
func dropIgnoredTraces(traces ptrace.Traces, conf *config.AgentConfig, rootFilter *filters.Blacklister) ptrace.Traces {
	ignored := ignoredTraces(traces, conf, rootFilter)
	if len(ignored) == 0 {
		return traces
	}
	kept := ptrace.NewTraces()
	traces.CopyTo(kept)
	for i := 0; i < kept.ResourceSpans().Len(); i++ {
		scopeSpans := kept.ResourceSpans().At(i).ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			scopeSpans.At(j).Spans().RemoveIf(func(span ptrace.Span) bool {
				_, ok := ignored[resourceTrace{i, span.TraceID()}]
				return ok
			})
		}
	}
	return kept
}

// ignoredTraces returns the resource traces with a root that rootFilter denies. Like traceutil.GetRoot, the root
// of a trace is its span without parent, or else a span whose parent is not among the spans of its resource.
// The root is matched on its converted Datadog span.
func ignoredTraces(traces ptrace.Traces, conf *config.AgentConfig, rootFilter *filters.Blacklister) map[resourceTrace]struct{} {
	var ignored map[resourceTrace]struct{}
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rspans := traces.ResourceSpans().At(i)
		if isStatsComputed(rspans.Resource().Attributes()) {
			continue // the connector computes no stats for this resource
		}
		spanIDs := make(map[pcommon.SpanID]struct{})
		hasRoot := make(map[pcommon.TraceID]bool)
		computed := false
		forEachSpan(rspans, func(span ptrace.Span, _ pcommon.InstrumentationScope) {
			spanIDs[span.SpanID()] = struct{}{}
			if span.ParentSpanID().IsEmpty() {
				hasRoot[span.TraceID()] = true
			}
			computed = computed || isStatsComputed(span.Attributes())
		})
		if computed {
			continue
		}
		forEachSpan(rspans, func(span ptrace.Span, scope pcommon.InstrumentationScope) {
			if !span.ParentSpanID().IsEmpty() {
				if _, hasParent := spanIDs[span.ParentSpanID()]; hasParent || hasRoot[span.TraceID()] {
					return
				}
			}
			if allowed, _ := rootFilter.Allows(transform.OtelSpanToDDSpan(span, rspans.Resource(), scope, conf)); allowed {
				return
			}
			if ignored == nil {
				ignored = make(map[resourceTrace]struct{})
			}
			ignored[resourceTrace{i, span.TraceID()}] = struct{}{}
		})
	}
	return ignored
}

// isStatsComputed reports whether attrs carry a truthy client-computed stats marker, as the stats conversion checks it.
func isStatsComputed(attrs pcommon.Map) bool {
	value := transform.GetOTelAttrVal(attrs, true, keyStatsComputed)
	return value != "" && value != "false"
}

func forEachSpan(rspans ptrace.ResourceSpans, f func(ptrace.Span, pcommon.InstrumentationScope)) {
	for j := 0; j < rspans.ScopeSpans().Len(); j++ {
		scopeSpans := rspans.ScopeSpans().At(j)
		for k := 0; k < scopeSpans.Spans().Len(); k++ {
			f(scopeSpans.Spans().At(k), scopeSpans.Scope())
		}
	}
}
