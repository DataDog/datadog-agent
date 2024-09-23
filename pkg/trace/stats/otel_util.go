// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"slices"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.17.0"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// chunkKey is used to group TraceChunks
type chunkKey struct {
	traceIDUInt64 uint64
	env           string
	version       string
	hostname      string
	cid           string
}

// OTLPTracesToConcentratorInputs converts eligible OTLP spans to Concentrator.Input.
// The converted Inputs only have the minimal number of fields for APM stats calculation and are only meant
// to be used in Concentrator.Add(). Do not use them for other purposes.
func OTLPTracesToConcentratorInputs(
	traces ptrace.Traces,
	conf *config.AgentConfig,
	containerTagKeys []string,
	peerTagKeys []string,
) []Input {
	spanByID, resByID, scopeByID := traceutil.IndexOTelSpans(traces)
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	topLevelSpans := traceutil.GetTopLevelOTelSpans(spanByID, resByID, topLevelByKind)
	ignoreResNames := make(map[string]struct{})
	for _, resName := range conf.Ignore["resource"] {
		ignoreResNames[resName] = struct{}{}
	}
	chunks := make(map[chunkKey]*pb.TraceChunk)
	containerTagsByID := make(map[string][]string)
	for spanID, otelspan := range spanByID {
		otelres := resByID[spanID]
		if _, exists := ignoreResNames[traceutil.GetOTelResource(otelspan, otelres)]; exists {
			continue
		}
		// TODO(songy23): use AttributeDeploymentEnvironmentName once collector version upgrade is unblocked
		env := traceutil.GetOTelAttrValInResAndSpanAttrs(otelspan, otelres, true, "deployment.environment.name", semconv.AttributeDeploymentEnvironment)
		hostname := traceutil.GetOTelHostname(otelspan, otelres, conf.OTLPReceiver.AttributesTranslator, conf.Hostname)
		version := traceutil.GetOTelAttrValInResAndSpanAttrs(otelspan, otelres, true, semconv.AttributeServiceVersion)
		cid := traceutil.GetOTelAttrValInResAndSpanAttrs(otelspan, otelres, true, semconv.AttributeContainerID, semconv.AttributeK8SPodUID)
		var ctags []string
		if cid != "" {
			ctags = traceutil.GetOTelContainerTags(otelres.Attributes(), containerTagKeys)
			if ctags != nil {
				// Make sure container tags are sorted per APM stats intake requirement
				if !slices.IsSorted(ctags) {
					slices.Sort(ctags)
				}
				containerTagsByID[cid] = ctags
			}
		}
		ckey := chunkKey{
			traceIDUInt64: traceutil.OTelTraceIDToUint64(otelspan.TraceID()),
			env:           env,
			version:       version,
			hostname:      hostname,
			cid:           cid,
		}
		chunk, ok := chunks[ckey]
		if !ok {
			chunk = &pb.TraceChunk{}
			chunks[ckey] = chunk
		}
		_, isTop := topLevelSpans[spanID]
		chunk.Spans = append(chunk.Spans, otelSpanToDDSpan(otelspan, otelres, scopeByID[spanID], isTop, topLevelByKind, conf, peerTagKeys))
	}

	inputs := make([]Input, 0, len(chunks))
	for ckey, chunk := range chunks {
		pt := traceutil.ProcessedTrace{
			TraceChunk:     chunk,
			Root:           traceutil.GetRoot(chunk.Spans),
			TracerEnv:      ckey.env,
			AppVersion:     ckey.version,
			TracerHostname: ckey.hostname,
		}
		inputs = append(inputs, Input{
			Traces:        []traceutil.ProcessedTrace{pt},
			ContainerID:   ckey.cid,
			ContainerTags: containerTagsByID[ckey.cid],
		})
	}
	return inputs
}

// otelSpanToDDSpan converts an OTel span to a DD span.
// The converted DD span only has the minimal number of fields for APM stats calculation and is only meant
// to be used in OTLPTracesToConcentratorInputs. Do not use them for other purposes.
// TODO(OTEL-1726): use the same function here and in pkg/trace/api/otlp.go
func otelSpanToDDSpan(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
) *pb.Span {
	ddspan := &pb.Span{
		Service:  traceutil.GetOTelService(otelspan, otelres, true),
		Name:     traceutil.GetOTelOperationName(otelspan, otelres, lib, conf.OTLPReceiver.SpanNameAsResourceName, conf.OTLPReceiver.SpanNameRemappings, true),
		Resource: traceutil.GetOTelResource(otelspan, otelres),
		TraceID:  traceutil.OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   traceutil.OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: traceutil.OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Type:     traceutil.GetOTelSpanType(otelspan, otelres),
	}
	spanKind := otelspan.Kind()
	traceutil.SetMeta(ddspan, "span.kind", traceutil.OTelSpanKindName(spanKind))
	code := traceutil.GetOTelStatusCode(otelspan)
	if code != 0 {
		traceutil.SetMetric(ddspan, tagStatusCode, float64(code))
	}
	if otelspan.Status().Code() == ptrace.StatusCodeError {
		ddspan.Error = 1
	}
	if isTopLevel {
		traceutil.SetTopLevel(ddspan, true)
	}
	if isMeasured := traceutil.GetOTelAttrVal(otelspan.Attributes(), false, "_dd.measured"); isMeasured == "1" {
		traceutil.SetMeasured(ddspan, true)
	} else if topLevelByKind && (spanKind == ptrace.SpanKindClient || spanKind == ptrace.SpanKindProducer) {
		// When enable_otlp_compute_top_level_by_span_kind is true, compute stats for client-side spans
		traceutil.SetMeasured(ddspan, true)
	}
	for _, peerTagKey := range peerTagKeys {
		if peerTagVal := traceutil.GetOTelAttrValInResAndSpanAttrs(otelspan, otelres, false, peerTagKey); peerTagVal != "" {
			traceutil.SetMeta(ddspan, peerTagKey, peerTagVal)
		}
	}
	return ddspan
}
