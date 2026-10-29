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

// In DDOT, the trace-agent behind the datadog exporter drops the traces whose root resource matches
// apm_config.ignore_resources (Agent.Process in pkg/trace/agent). The connector computes stats on its own, so it
// applies the same rule here, so that spans and stats are dropped for exactly the same traces.
//
// The trace-agent's OTLP receiver builds one chunk per resource and trace ID, with the spans in OTLP order.
// The stats conversion groups the spans of the whole batch by trace ID, env, version, hostname and container ID, and in random map order.
// traceutil.GetRoot depends on both chunking and order, so the root of a stats chunk can differ from the trace-agent's.
// This difference is the reason why root filtering logic cannot be embedded in stats conversion directly.

// resourceTrace identifies the spans of a trace within one resource, which the trace-agent processes as a chunk.
type resourceTrace struct {
	resource pcommon.Resource
	traceID  uint64
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
// is matched on its resource name, computed as in the trace-agent's OTLP span conversion.
func ignoredTraces(traces ptrace.Traces, conf *config.AgentConfig, rootFilter *filters.Blacklister) map[resourceTrace]struct{} {
	var ignored map[resourceTrace]struct{}
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rspans := traces.ResourceSpans().At(i)
		// traceutil.GetRoot only reads the span and parent IDs, and depends on the span order, so the spans are
		// appended in OTLP order, as the trace-agent's OTLP receiver does.
		chunks := make(map[uint64]pb.Trace)
		otelSpans := make(map[*pb.Span]ptrace.Span)
		forEachSpan(rspans, func(span ptrace.Span) {
			ddspan := &pb.Span{SpanID: transform.OTelSpanIDToUint64(span.SpanID()), ParentID: transform.OTelSpanIDToUint64(span.ParentSpanID())}
			traceID := transform.OTelTraceIDToUint64(span.TraceID())
			chunks[traceID] = append(chunks[traceID], ddspan)
			otelSpans[ddspan] = span
		})
		for traceID, chunk := range chunks {
			root := otelSpans[traceutil.GetRoot(chunk)]
			if allowed, _ := rootFilter.AllowsString(resourceName(conf, root, rspans.Resource())); allowed {
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

// resourceName returns the resource name of span, as transform.OtelSpanToDDSpan computes it.
func resourceName(conf *config.AgentConfig, span ptrace.Span, res pcommon.Resource) string {
	if transform.OperationAndResourceNameV2Enabled(conf) {
		return transform.GetOTelResourceV2(span, res)
	}
	return transform.GetOTelResourceV1(span, res)
}

func forEachSpan(rspans ptrace.ResourceSpans, f func(ptrace.Span)) {
	for j := 0; j < rspans.ScopeSpans().Len(); j++ {
		spans := rspans.ScopeSpans().At(j).Spans()
		for k := 0; k < spans.Len(); k++ {
			f(spans.At(k))
		}
	}
}
