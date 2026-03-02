// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const customContainerTagPrefix = "datadog.container.tag."

// scopeSpansData holds scope name/version and raw span message bytes for one ScopeSpans.
type scopeSpansData struct {
	ScopeName   string
	ScopeVersion string
	SpanBytes   [][]byte
}

// buildPayloadFromResourceSpansBytes parses one ResourceSpans message, builds a Payload (TracerPayload)
// using the same rules as receiveResourceSpansV2, and sends it on o.out. No pdata or ReceiveResourceSpans.
// Returns the number of spans in this ResourceSpans for metrics.
func (o *OTLPReceiver) buildPayloadFromResourceSpansBytes(ctx context.Context, httpHeader http.Header, data []byte) (spanCount int64, err error) {
	resourceAttrs, scopeSpansList, err := parseResourceSpansToData(data)
	if err != nil {
		return 0, err
	}
	if len(resourceAttrs) == 0 && len(scopeSpansList) == 0 {
		return 0, nil
	}

	// Build pcommon.Map/Resource only for source and container tags (minimal use of pdata).
	resMap := pcommon.NewMap()
	for k, v := range resourceAttrs {
		resMap.PutStr(k, v)
	}
	src, srcok := o.conf.OTLPReceiver.AttributesTranslator.AttributesToSource(ctx, resMap)
	var hostname string
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			hostname = src.Identifier
		default:
			hostname = ""
		}
	} else {
		hostname = o.conf.Hostname
		src = source.Source{Kind: source.HostnameKind, Identifier: hostname}
	}

	// Container tags and filtered resource attrs (so span Meta doesn't duplicate container tags).
	otelRes := pcommon.NewResource()
	for k, v := range resourceAttrs {
		otelRes.Attributes().PutStr(k, v)
	}
	containerTagsMap, filteredRes := attributes.ConsumeContainerTagsFromResource(otelRes)
	resAttrsFiltered := make(map[string]string)
	filteredRes.Attributes().Range(func(k string, v pcommon.Value) bool {
		resAttrsFiltered[k] = v.AsString()
		return true
	})
	containerTagsBuilder := flatten(containerTagsMap)
	containerID := getFirstFromMaps(nil, resourceAttrs, string(semconv.ContainerIDKey), string(semconv.K8SPodUIDKey))
	if o.conf.HasFeature("enable_otlp_container_tags_v2") {
		containerID = getFirstFromMaps(nil, resourceAttrs, string(semconv.ContainerIDKey))
	} else {
		if tags := getContainerTags(o.conf.ContainerTags, containerID); tags != "" {
			appendTags(containerTagsBuilder, tags)
		} else if src.Kind == source.AWSECSFargateKind {
			appendTags(containerTagsBuilder, src.Tag())
		}
	}
	containerTags := containerTagsBuilder.String()
	env := getFirstFromMaps(nil, resourceAttrs, string(semconv127.DeploymentEnvironmentNameKey), string(semconv.DeploymentEnvironmentKey))
	lang := getFirstFromMaps(nil, resourceAttrs, string(semconv.TelemetrySDKLanguageKey))
	tracerVersion := "otlp-" + getFirstFromMaps(nil, resourceAttrs, string(semconv.TelemetrySDKVersionKey))

	tracesByID := make(map[uint64]pb.Trace)
	priorityByID := make(map[uint64]sampler.SamplingPriority)

	for _, sc := range scopeSpansList {
		for _, spanBytes := range sc.SpanBytes {
			span, err := parseSpanBytesToDDSpan(spanBytes, resAttrsFiltered, sc.ScopeName, sc.ScopeVersion, o.conf)
			if err != nil {
				continue
			}
			spanCount++
			traceIDU64 := span.TraceID
			if tracesByID[traceIDU64] == nil {
				tracesByID[traceIDU64] = pb.Trace{}
			}
			if p, ok := span.Metrics["_sampling_priority_v1"]; ok {
				priorityByID[traceIDU64] = sampler.SamplingPriority(p)
			}
			tracesByID[traceIDU64] = append(tracesByID[traceIDU64], span)
		}
	}

	tagstats := &info.TagStats{
		Tags: info.Tags{
			Lang:            lang,
			TracerVersion:   tracerVersion,
			EndpointVersion: "opentelemetry_grpc_v1",
		},
		Stats: info.NewStats(),
	}
	clientComputedStats := getFirstFromMaps(nil, resourceAttrs, keyStatsComputed) != "" || isHeaderTrue(header.ComputedStats, httpHeader.Get(header.ComputedStats))
	p := Payload{
		Source:                 tagstats,
		ClientComputedStats:    clientComputedStats,
		ClientComputedTopLevel: o.conf.HasFeature("enable_otlp_compute_top_level_by_span_kind"),
		TracerPayload: &pb.TracerPayload{
			Hostname:      hostname,
			Chunks:        o.createChunks(tracesByID, priorityByID),
			Env:           env,
			ContainerID:   containerID,
			LanguageName:  tagstats.Lang,
			TracerVersion: tagstats.TracerVersion,
		},
	}
	if len(containerTags) > 0 {
		p.TracerPayload.Tags = map[string]string{tagContainersTags: containerTags}
	}
	_ = o.statsd.Count("datadog.trace_agent.otlp.spans", spanCount, tagstats.AsTags(), 1)
	_ = o.statsd.Count("datadog.trace_agent.otlp.traces", int64(len(tracesByID)), tagstats.AsTags(), 1)
	o.out <- &p
	return spanCount, nil
}

// parseSpanBytesToDDSpan parses raw OTLP Span message bytes and returns a complete *pb.Span.
// Attributes are written directly into span.Meta (string) and span.Metrics (numeric) with raw
// OTLP keys; identity is computed from them, then keys are remapped to DD convention.
func parseSpanBytesToDDSpan(spanBytes []byte, resAttrsFiltered map[string]string, scopeName, scopeVersion string, conf *config.AgentConfig) (*pb.Span, error) {
	span := &pb.Span{
		Meta:    make(map[string]string, 24),
		Metrics: make(map[string]float64),
	}
	var eventsBuf, linksBuf strings.Builder
	eventsBuf.WriteString("[")
	linksBuf.WriteString("[")
	firstEvent, firstLink := true, true
	eventAttrsReuse := make(map[string]interface{})
	linkAttrsReuse := make(map[string]string)

	name, kind, statusCode, statusMessage, traceState, traceID, parentSpanID, hasExceptionEvent, err := parseSpanWireIntoDDSpan(
		spanBytes, span, &eventsBuf, &linksBuf, &firstEvent, &firstLink,
		eventAttrsReuse, linkAttrsReuse,
	)
	if err != nil {
		return nil, err
	}

	// Identity from span.Meta + span.Metrics (raw keys) and resAttrsFiltered.
	if transform.OperationAndResourceNameV2Enabled(conf) {
		span.Service = getOTelServiceFromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics, true)
		span.Name = getOTelOperationNameV2FromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics, name, kind)
		span.Resource = getOTelResourceV2FromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics, name, kind)
		span.Type = getOTelSpanTypeFromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics, kind)
	} else {
		span.Service = getOTelServiceFromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics, true)
		span.Name = name
		if span.Name == "" {
			span.Name = otelSpanKindName(kind)
		}
		span.Resource = getOTelResourceV2FromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics, name, kind)
		span.Type = getOTelSpanTypeFromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics, kind)
	}
	span.Meta["span.kind"] = otelSpanKindName(kind)
	if code := getOTelStatusCodeFromMetaMetrics(span.Meta, resAttrsFiltered, span.Metrics); code != 0 {
		span.Metrics[traceutil.TagStatusCode] = float64(code)
	}
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	isTopLevel := topLevelByKind && (parentSpanID == [8]byte{} || kind == 2 || kind == 5)
	if isTopLevel {
		traceutil.SetTopLevel(span, true)
	}
	if measured := getFirstFromMetaMetricsRes(span.Meta, span.Metrics, resAttrsFiltered, "_dd.measured"); measured == "1" {
		traceutil.SetMeasured(span, true)
	} else if topLevelByKind && (kind == 3 || kind == 4) {
		traceutil.SetMeasured(span, true)
	}

	// Remap span.Meta and span.Metrics from raw OTLP keys to DD keys; drop APM convention keys (handled by identity).
	remapSpanMetaAndMetrics(span)

	for k, v := range resAttrsFiltered {
		if mappedKey := transform.GetDDKeyForOTLPAttribute(k); mappedKey != "" && span.Meta[mappedKey] == "" {
			transform.SetMetaOTLPIfEmpty(span, mappedKey, v)
		}
	}
	span.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])
	if traceState != "" {
		span.Meta["w3c.tracestate"] = traceState
	}
	if scopeName != "" {
		span.Meta[string(semconv117.OtelLibraryNameKey)] = scopeName
	}
	if scopeVersion != "" {
		span.Meta[string(semconv117.OtelLibraryVersionKey)] = scopeVersion
	}
	span.Meta[string(semconv117.OtelStatusCodeKey)] = otelStatusCodename(statusCode)
	if statusMessage != "" {
		span.Meta[string(semconv117.OtelStatusDescriptionKey)] = statusMessage
	}
	eventsStr := eventsBuf.String() + "]"
	if eventsStr != "]" {
		span.Meta["events"] = eventsStr
	}
	if hasExceptionEvent {
		span.Meta["_dd.span_events.has_exception"] = "true"
	}
	linksStr := linksBuf.String() + "]"
	if linksStr != "]" {
		span.Meta["_dd.span_links"] = linksStr
	}
	if statusCode == 2 && span.Meta["error.msg"] == "" && statusMessage != "" {
		span.Meta["error.msg"] = statusMessage
	}
	return span, nil
}

// remapSpanMetaAndMetrics rewrites span.Meta and span.Metrics from raw OTLP keys to DD keys.
// Keys that map to "" (APM convention keys already used for identity) are removed.
func remapSpanMetaAndMetrics(span *pb.Span) {
	var metaDelete []string
	metaSet := make(map[string]string)
	for k, v := range span.Meta {
		mapped := transform.GetDDKeyForOTLPAttribute(k)
		if mapped == "" {
			metaDelete = append(metaDelete, k)
		} else if mapped != k {
			if span.Meta[mapped] == "" {
				metaSet[mapped] = v
			}
			metaDelete = append(metaDelete, k)
		}
	}
	for _, k := range metaDelete {
		delete(span.Meta, k)
	}
	for k, v := range metaSet {
		span.Meta[k] = v
	}
	var metricsDelete []string
	metricsSet := make(map[string]float64)
	for k, v := range span.Metrics {
		mapped := transform.GetDDKeyForOTLPAttribute(k)
		if mapped == "" {
			metricsDelete = append(metricsDelete, k)
		} else if mapped != k {
			if _, ok := span.Metrics[mapped]; !ok {
				metricsSet[mapped] = v
			}
			metricsDelete = append(metricsDelete, k)
		}
	}
	for _, k := range metricsDelete {
		delete(span.Metrics, k)
	}
	for k, v := range metricsSet {
		span.Metrics[k] = v
	}
}

func otelStatusCodename(code int32) string {
	switch code {
	case 0:
		return "Unset"
	case 1:
		return "Ok"
	case 2:
		return "Error"
	default:
		return "Unset"
	}
}

