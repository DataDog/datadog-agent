// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogconnector

import (
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// In DDOT, the trace-agent behind the datadog exporter drops the traces whose root span matches
// apm_config.ignore_resources or fails apm_config.filter_tags and apm_config.filter_tags_regex (Agent.Process in
// pkg/trace/agent). The connector computes stats on its own, so it applies the same rules here, so that spans and
// stats are dropped for exactly the same traces.
//
// The trace-agent's OTLP receiver builds one chunk per resource and trace ID, with the spans in OTLP order.
// The stats conversion groups the spans of the whole batch by trace ID, env, version, hostname and container ID, and in random map order.
// traceutil.GetRoot depends on both chunking and order, so the root of a stats chunk can differ from the trace-agent's.
// This difference is the reason why root filtering logic cannot be embedded in stats conversion directly.

// rootFilter holds the trace-agent's rules that drop a whole trace based on its root span.
type rootFilter struct {
	// conf is the trace-agent's configuration. The root span is converted with it, rather than with the connector's
	// configuration, so that it gets the resource name and tags the trace-agent matches: apm_config.features and the
	// exporter's span name settings change them.
	conf *config.AgentConfig
	// ignoreResources is the trace-agent's apm_config.ignore_resources, or nil when unset.
	ignoreResources *filters.Blacklister
}

// newRootFilter returns the root filter of the rules of the trace-agent's configuration conf, or nil when there are
// none.
func newRootFilter(conf *config.AgentConfig) *rootFilter {
	if conf == nil {
		return nil
	}
	f := &rootFilter{conf: conf}
	if len(conf.Ignore["resource"]) > 0 {
		f.ignoreResources = filters.NewBlacklister(conf.Ignore["resource"])
	}
	if f.ignoreResources == nil && !f.hasTagRules() {
		return nil
	}
	return f
}

// hasTagRules reports whether the trace-agent has apm_config.filter_tags or apm_config.filter_tags_regex rules.
func (f *rootFilter) hasTagRules() bool {
	return len(f.conf.RequireTags) > 0 || len(f.conf.RejectTags) > 0 || len(f.conf.RequireTagsRegex) > 0 || len(f.conf.RejectTagsRegex) > 0
}

// resourceTrace identifies the spans of a trace within one resource, which the trace-agent processes as a chunk.
type resourceTrace struct {
	resource pcommon.Resource
	traceID  uint64
}

// scopedSpan is a span with the instrumentation scope it belongs to.
type scopedSpan struct {
	span  ptrace.Span
	scope pcommon.InstrumentationScope
}

// ignoredRootsFilter returns a span filter that excludes from stats the traces whose root rootFilter denies, like
// the trace-agent does, or nil when no trace is ignored.
func ignoredRootsFilter(traces ptrace.Traces, rootFilter *rootFilter) func(pcommon.Resource, ptrace.Span) bool {
	ignored := ignoredTraces(traces, rootFilter)
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
// is matched on its resource name and tags, computed as in the trace-agent's OTLP span conversion.
func ignoredTraces(traces ptrace.Traces, rootFilter *rootFilter) map[resourceTrace]struct{} {
	var ignored map[resourceTrace]struct{}
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rspans := traces.ResourceSpans().At(i)
		// traceutil.GetRoot only reads the span and parent IDs, and depends on the span order, so the spans are
		// appended in OTLP order, as the trace-agent's OTLP receiver does.
		chunks := make(map[uint64]pb.Trace)
		otelSpans := make(map[*pb.Span]scopedSpan)
		forEachSpan(rspans, func(span ptrace.Span, scope pcommon.InstrumentationScope) {
			ddspan := &pb.Span{SpanID: transform.OTelSpanIDToUint64(span.SpanID()), ParentID: transform.OTelSpanIDToUint64(span.ParentSpanID())}
			traceID := transform.OTelTraceIDToUint64(span.TraceID())
			chunks[traceID] = append(chunks[traceID], ddspan)
			otelSpans[ddspan] = scopedSpan{span, scope}
		})
		// tagsResource is the resource the trace-agent's OTLP receiver converts the spans with, computed once per
		// resource and only when the root tags are needed.
		var tagsResource *pcommon.Resource
		for traceID, chunk := range chunks {
			root := otelSpans[traceutil.GetRoot(chunk)]
			if rootFilter.allowsResource(root.span, rspans.Resource()) {
				if !rootFilter.hasTagRules() {
					continue
				}
				if tagsResource == nil {
					res := spanConversionResource(rootFilter.conf, rspans.Resource())
					tagsResource = &res
				}
				if rootFilter.allowsTags(rootTags(rootFilter.conf, root, *tagsResource)) {
					continue
				}
			}
			if ignored == nil {
				ignored = make(map[resourceTrace]struct{})
			}
			ignored[resourceTrace{rspans.Resource(), traceID}] = struct{}{}
		}
	}
	return ignored
}

// allowsResource reports whether the resource name of root passes apm_config.ignore_resources.
func (f *rootFilter) allowsResource(root ptrace.Span, res pcommon.Resource) bool {
	if f.ignoreResources == nil {
		return true
	}
	allowed, _ := f.ignoreResources.AllowsString(resourceName(f.conf, root, res))
	return allowed
}

// allowsTags reports whether root passes apm_config.filter_tags and apm_config.filter_tags_regex.
func (f *rootFilter) allowsTags(root *pb.Span) bool {
	return !filters.FilteredByTags(root, f.conf.RequireTags, f.conf.RejectTags, f.conf.RequireTagsRegex, f.conf.RejectTagsRegex)
}

// resourceName returns the resource name of span, as transform.OtelSpanToDDSpan computes it.
func resourceName(conf *config.AgentConfig, span ptrace.Span, res pcommon.Resource) string {
	if transform.OperationAndResourceNameV2Enabled(conf) {
		return transform.GetOTelResourceV2(span, res)
	}
	return transform.GetOTelResourceV1(span, res)
}

// spanConversionResource returns the resource that the trace-agent's OTLP receiver passes to
// transform.OtelSpanToDDSpan: unless disable_otlp_container_tags_v2 is set, the container tags are moved out of it,
// so they are not span tags. res itself is left unmodified.
func spanConversionResource(conf *config.AgentConfig, res pcommon.Resource) pcommon.Resource {
	if conf.HasFeature("disable_otlp_container_tags_v2") {
		return res
	}
	_, res = attributes.ConsumeContainerTagsFromResource(res)
	return res
}

// rootTags returns root converted as the trace-agent's OTLP receiver does, with the tag changes that the
// trace-agent's normalizer makes before applying apm_config.filter_tags: peer.service, _dd.base_service and env are
// normalized, and an invalid http.status_code is removed. Keep in sync with Agent.normalize in
// pkg/trace/agent/normalizer.go.
func rootTags(conf *config.AgentConfig, root scopedSpan, res pcommon.Resource) *pb.Span {
	ddspan := transform.OtelSpanToDDSpan(root.span, res, root.scope, conf)
	reg := semantics.DefaultRegistry()
	accessor := semantics.NewDDSpanAccessor(ddspan.Meta, ddspan.Metrics)
	if pSvc := semantics.LookupString(reg, accessor, semantics.ConceptPeerService); pSvc != "" {
		ddspan.Meta[string(semantics.ConceptPeerService)], _ = normalizeutil.NormalizePeerService(pSvc)
	}
	if bSvc := semantics.LookupString(reg, accessor, semantics.ConceptDDBaseService); bSvc != "" {
		ddspan.Meta[string(semantics.ConceptDDBaseService)], _ = normalizeutil.NormalizePeerService(bSvc)
	}
	if env := semantics.LookupString(reg, accessor, semantics.ConceptDDEnv); env != "" {
		ddspan.Meta["env"] = normalizeutil.NormalizeTagValue(env)
	}
	if sc, ok := ddspan.Meta["http.status_code"]; ok && !normalizeutil.IsValidStatusCode(sc) {
		delete(ddspan.Meta, "http.status_code")
	}
	return ddspan
}

func forEachSpan(rspans ptrace.ResourceSpans, f func(ptrace.Span, pcommon.InstrumentationScope)) {
	for j := 0; j < rspans.ScopeSpans().Len(); j++ {
		sspans := rspans.ScopeSpans().At(j)
		spans := sspans.Spans()
		for k := 0; k < spans.Len(); k++ {
			f(spans.At(k), sspans.Scope())
		}
	}
}
