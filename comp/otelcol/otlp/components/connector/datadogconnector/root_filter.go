// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogconnector

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// resourceTrace identifies the spans of a trace within one resource, which the trace-agent processes as a chunk.
type resourceTrace struct {
	resource pcommon.Resource
	traceID  uint64
}

type scopedSpan struct {
	span  ptrace.Span
	scope pcommon.InstrumentationScope
}

// ignoredRootsFilter returns a span filter that excludes from stats the traces whose root resource rootFilter
// denies, like the trace-agent does with apm_config.ignore_resources, or nil when no trace is ignored.
func ignoredRootsFilter(traces ptrace.Traces, conf *config.AgentConfig, rootFilter *filters.Blacklister) func(pcommon.Resource, ptrace.Span) bool {
	ignored := ignoredTraces(traces, conf, rootFilter)
	if len(ignored) == 0 {
		return nil
	}
	return func(res pcommon.Resource, span ptrace.Span) bool {
		_, ok := ignored[resourceTrace{res, transform.OTelTraceIDToUint64(span.TraceID())}]
		return !ok
	}
}

// ignoredTraces returns the resource traces with a root that rootFilter denies. It builds the chunks of the
// trace-agent's OTLP receiver, one per trace and resource, and picks their root with traceutil.GetRoot; the root
// is matched on its converted Datadog span.
func ignoredTraces(traces ptrace.Traces, conf *config.AgentConfig, rootFilter *filters.Blacklister) map[resourceTrace]struct{} {
	var ignored map[resourceTrace]struct{}
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rspans := traces.ResourceSpans().At(i)
		// traceutil.GetRoot only reads the span and parent IDs.
		chunks := make(map[uint64]pb.Trace)
		otelSpans := make(map[*pb.Span]scopedSpan)
		forEachSpan(rspans, func(span ptrace.Span, scope pcommon.InstrumentationScope) {
			ddspan := &pb.Span{SpanID: transform.OTelSpanIDToUint64(span.SpanID()), ParentID: transform.OTelSpanIDToUint64(span.ParentSpanID())}
			traceID := transform.OTelTraceIDToUint64(span.TraceID())
			chunks[traceID] = append(chunks[traceID], ddspan)
			otelSpans[ddspan] = scopedSpan{span, scope}
		})
		for traceID, chunk := range chunks {
			root := otelSpans[traceutil.GetRoot(chunk)]
			if allowed, _ := rootFilter.Allows(transform.OtelSpanToDDSpan(root.span, rspans.Resource(), root.scope, conf)); allowed {
				continue
			}
			if ignored == nil {
				ignored = make(map[resourceTrace]struct{})
			}
			ignored[resourceTrace{rspans.Resource(), traceID}] = struct{}{}
		}
	}
	return ignored
}

func forEachSpan(rspans ptrace.ResourceSpans, f func(ptrace.Span, pcommon.InstrumentationScope)) {
	for j := 0; j < rspans.ScopeSpans().Len(); j++ {
		scopeSpans := rspans.ScopeSpans().At(j)
		for k := 0; k < scopeSpans.Spans().Len(); k++ {
			f(scopeSpans.Spans().At(k), scopeSpans.Scope())
		}
	}
}
