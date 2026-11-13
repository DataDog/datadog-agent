// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package transform implements mappings from OTLP to DD semantics, and helpers
package transform

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tinylib/msgp/msgp"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// OperationAndResourceNameV2Enabled checks if the new operation and resource name logic should be used
func OperationAndResourceNameV2Enabled(conf *config.AgentConfig) bool {
	return !conf.OTLPReceiver.SpanNameAsResourceName && len(conf.OTLPReceiver.SpanNameRemappings) == 0 && !conf.HasFeature("disable_operation_and_resource_name_logic_v2")
}

// otelSpanToDDSpanMinimal is the internal implementation shared by OtelSpanToDDSpanMinimalWithTagKeys and
// OtelSpanToDDSpan, accepting a pre-created accessor to avoid repeated allocation.
func otelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	spanAccessor semantics.Accessor,
	tagKeys OTelSpanTagKeys,
) *pb.Span {
	spanKind := otelspan.Kind()

	sattr := otelspan.Attributes()
	rattr := otelres.Attributes()

	ddspan := &pb.Span{
		Service:  GetOTelServiceWithAccessor(spanAccessor, true),
		TraceID:  OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Meta:     make(map[string]string, sattr.Len()+rattr.Len()),
		Metrics:  make(map[string]float64),
	}

	if otelspan.Status().Code() == ptrace.StatusCodeError {
		ddspan.Error = 1
	}

	if OperationAndResourceNameV2Enabled(conf) {
		ddspan.Name = GetOTelOperationNameV2WithAccessor(otelspan, spanAccessor)
		ddspan.Resource = GetOTelResourceV2WithAccessor(otelspan, spanAccessor)
	} else {
		ddspan.Name = GetOTelOperationNameV1(otelspan, otelres, lib, conf.OTLPReceiver.SpanNameAsResourceName, conf.OTLPReceiver.SpanNameRemappings, true)
		ddspan.Resource = GetOTelResourceV1(otelspan, otelres)
	}
	// This path bypasses the trace-agent's normalization step so we explicitly do it here.
	ddspan.Name, _ = normalizeutil.NormalizeName(ddspan.Name)

	// correct span type logic if using new resource receiver, keep same if on v1. separate from OperationAndResourceNameV2Enabled.
	if !conf.HasFeature("disable_receive_resource_spans_v2") {
		ddspan.Type = GetOTelSpanTypeWithAccessor(otelspan, spanAccessor)
	} else {
		ddspan.Type = LookupSemanticStringFromDualMaps(rattr, sattr, semantics.ConceptSpanType, true)
		if ddspan.Type == "" {
			ddspan.Type = SpanKind2Type(otelspan, otelres)
		}
	}

	ddspan.Meta["span.kind"] = OTelSpanKindName(spanKind)

	reg := semantics.DefaultRegistry()
	if code, ok := semantics.LookupInt64(reg, spanAccessor, semantics.ConceptHTTPStatusCode); ok && code >= 0 {
		ddspan.Metrics[traceutil.TagStatusCode] = float64(code)
	}
	if grpcCode := semantics.LookupString(reg, spanAccessor, semantics.ConceptGRPCStatusCode); grpcCode != "" {
		ddspan.Meta[string(semantics.ConceptGRPCStatusCode)] = grpcCode
	}
	if isTopLevel {
		traceutil.SetTopLevel(ddspan, true)
	}
	if v, ok := semantics.LookupInt64(reg, spanAccessor, semantics.ConceptDDMeasured); ok && v == 1 {
		traceutil.SetMeasured(ddspan, true)
	} else if topLevelByKind && (spanKind == ptrace.SpanKindClient || spanKind == ptrace.SpanKindProducer) {
		// When enable_otlp_compute_top_level_by_span_kind is true, compute stats for client-side spans
		traceutil.SetMeasured(ddspan, true)
	}
	// TODO(DSEM-658): migrate peer tag lookups to use the semantics library.
	// Use direct lookup only: the peer tag keys already list every precursor from peer_tags.ini (semantic-core).
	copyOTelAttributeTags(sattr, rattr, tagKeys.peer, ddspan.Meta)
	// This path bypasses the trace-agent's normalization step so we explicitly do it here.
	if pSvc, ok := ddspan.Meta[string(semantics.ConceptPeerService)]; ok {
		ddspan.Meta[string(semantics.ConceptPeerService)], _ = normalizeutil.NormalizePeerService(pSvc)
	}
	if bSvc, ok := ddspan.Meta[string(semantics.ConceptDDBaseService)]; ok {
		ddspan.Meta[string(semantics.ConceptDDBaseService)], _ = normalizeutil.NormalizePeerService(bSvc)
	}
	ddspan.Duration, _ = normalizeutil.FixDuration(ddspan.Start, ddspan.Duration)
	ddspan.Start, _ = normalizeutil.FixStartTime(ddspan.Start, ddspan.Duration)
	// Copy span-derived primary tag values into Meta so the APM stats
	// Concentrator's matchingAdditionalMetricTags (which reads span.Meta[key])
	// can aggregate on them. The minimal conversion does not copy all
	// attributes, so these must be pulled in explicitly like peer tags above.
	copyOTelAttributeTags(sattr, rattr, tagKeys.primary, ddspan.Meta)
	// Preserve the raw W3C tracestate so downstream consumers of the minimal
	// span (e.g. the APM stats Concentrator) can recover head-sampling
	// probability and weight stats accordingly. The full OtelSpanToDDSpan
	// conversion already does this; mirror it here.
	// An explicit _sample_rate attribute set by an upstream tracer takes
	// precedence over the value decoded from the tracestate below. Apply it first
	// so SetSampleRateFromTracestate's "gated on absence" guard preserves it.
	SetSampleRateFromAttribute(ddspan, sattr)
	if ts := otelspan.TraceState().AsRaw(); ts != "" {
		ddspan.Meta["w3c.tracestate"] = ts
		// Decode the head-based sampling probability from the tracestate and set
		// _sample_rate so the APM stats Concentrator scales stats back up by the
		// head-sampling weight (1/_sample_rate). Gated on absence to preserve any
		// explicit upstream value (including the _sample_rate attribute above).
		SetSampleRateFromTracestate(ddspan, ts)
	}
	return ddspan
}

// OTelSpanTagKeys holds immutable peer and primary tag selections reusable across spans.
// Its zero value selects no tags.
type OTelSpanTagKeys struct {
	peer    map[string]struct{}
	primary map[string]struct{}
}

// NewOTelSpanTagKeys prepares tag selections without retaining the input slices.
func NewOTelSpanTagKeys(peerTagKeys, primaryTagKeys []string) OTelSpanTagKeys {
	prepare := func(keys []string) map[string]struct{} {
		if len(keys) == 0 {
			return nil
		}
		set := make(map[string]struct{}, len(keys))
		for _, key := range keys {
			set[key] = struct{}{}
		}
		return set
	}
	return OTelSpanTagKeys{peer: prepare(peerTagKeys), primary: prepare(primaryTagKeys)}
}

// OtelSpanToDDSpanMinimal converts an OTel span to a DD span with only the minimal fields
// needed for APM stats calculation. Only use with OTLPTracesToConcentratorInputs.
//
// Deprecated: use OtelSpanToDDSpanMinimalWithTagKeys, which reuses tag selections
// across spans. This wrapper rebuilds them on every call and is kept so that
// pkg/trace/otel versions built against the previous signature keep compiling.
func OtelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
	primaryTagKeys []string,
) *pb.Span {
	return OtelSpanToDDSpanMinimalWithTagKeys(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, NewOTelSpanTagKeys(peerTagKeys, primaryTagKeys))
}

// OtelSpanToDDSpanMinimalWithTagKeys converts an OTel span to a DD span with only the minimal fields
// needed for APM stats calculation. Only use with OTLPTracesToConcentratorInputs.
// tagKeys selects the peer and span-derived primary tags to copy into Meta; prepare
// it once with NewOTelSpanTagKeys and reuse it across spans.
func OtelSpanToDDSpanMinimalWithTagKeys(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	tagKeys OTelSpanTagKeys,
) *pb.Span {
	spanAccessor := semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes())
	return otelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, spanAccessor, tagKeys)
}

func copyOTelAttributeTags(sattr, rattr pcommon.Map, keys map[string]struct{}, meta map[string]string) {
	if len(keys) == 0 {
		return
	}
	// Resource fallback first; nonempty span values then take precedence.
	for _, attrs := range [2]pcommon.Map{rattr, sattr} {
		for key, value := range attrs.All() {
			if _, requested := keys[key]; !requested {
				continue
			}
			if s := value.AsString(); s != "" {
				meta[key] = s
			}
		}
	}
}

func isDatadogAPMConventionKey(k string) bool {
	return k == "service.name" || k == "operation.name" || k == "resource.name" || k == "span.type"
}

// GetDDKeyForOTLPAttribute looks for a key in the Datadog HTTP convention that matches the given key from the
// OTLP HTTP convention. Otherwise, check if it is a Datadog APM convention key - if it is, it will be handled with
// specialized logic elsewhere, so return an empty string. If it isn't, return the original key.
func GetDDKeyForOTLPAttribute(k string) string {
	mappedKey, found := attributes.HTTPMappings[k]
	switch {
	case found:
		break
	case strings.HasPrefix(k, "http.request.header."):
		mappedKey = "http.request.headers." + strings.TrimPrefix(k, "http.request.header.")
	case !isDatadogAPMConventionKey(k):
		mappedKey = k
	default:
		return ""
	}
	return mappedKey
}

func conditionallyMapOTLPAttributeToMeta(k string, value string, ddspan *pb.Span) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	if _, ok := ddspan.MetaStruct[mappedKey]; ok {
		return
	}
	if ddspan.Meta[mappedKey] != "" {
		return
	}

	// Exclude Datadog APM conventions.
	// These are handled explicitly elsewhere.
	if mappedKey != "" {
		SetMetaOTLPIfEmpty(ddspan, mappedKey, value)
	}
}

// isMetaStructOTLPKey matches report roots and their dotted descendants.
// Custom events and fingerprints are read from ordinary metadata.
func isMetaStructOTLPKey(k string) bool {
	for _, prefix := range []string{"appsec.events", "appsec.fingerprint"} {
		if k == prefix || strings.HasPrefix(k, prefix+".") {
			return false
		}
	}
	for _, root := range []string{
		"appsec", "iast", "_dd.appsec_shadow_deployed", "_dd.stack", "threat_intel",
		"api.security", "api.entity", "api.definition", "api.endpoint", "http.request.body", "ai_guard", "_llmobs",
	} {
		if k == root || strings.HasPrefix(k, root+".") {
			return true
		}
	}
	return false
}

// SetMetaStructOTLP preserves MessagePack maps under known structured report keys.
// Other keys and byte encodings retain the normal OTLP attribute mapping.
func SetMetaStructOTLP(s *pb.Span, k string, v pcommon.Value) bool {
	if v.Type() != pcommon.ValueTypeBytes || !isMetaStructOTLPKey(k) {
		return false
	}
	data := v.Bytes().AsRaw()
	if msgp.NextType(data) != msgp.MapType {
		return false
	}
	remaining, err := msgp.Skip(data)
	if err != nil || len(remaining) != 0 {
		return false
	}
	if s.MetaStruct == nil {
		s.MetaStruct = make(map[string][]byte)
	}
	s.MetaStruct[k] = bytes.Clone(data)
	delete(s.Meta, k)
	return true
}

func conditionallyMapOTLPAttributeToMetric(k string, value float64, ddspan *pb.Span) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	if _, ok := ddspan.Metrics[mappedKey]; ok {
		return
	}

	// Exclude Datadog APM conventions.
	// These are handled explicitly elsewhere.
	if mappedKey != "" {
		SetMetricOTLPIfEmpty(ddspan, mappedKey, value)
	}
}

// GetOTelEnv returns the environment based on OTel span and resource attributes, with span taking precedence.
func GetOTelEnv(span ptrace.Span, res pcommon.Resource) string {
	return LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptDeploymentEnv, true)
}

// GetOTelHostname returns the DD hostname based on OTel span and resource attributes, with span taking precedence.
func GetOTelHostname(span ptrace.Span, res pcommon.Resource, tr *attributes.Translator, fallbackHost string) string {
	ctx := context.Background()
	src, srcok := tr.ResourceToSource(ctx, res, SignalTypeSet, nil)
	if !srcok {
		if v := GetOTelAttrValInResAndSpanAttrs(span, res, false, "_dd.hostname"); v != "" {
			src = source.Source{
				Kind:             source.HostnameKind,
				Identifier:       v, //nolint:staticcheck // SA1019: intentional during Step 1 of the Source.Identifier migration (datadog-agent#51116); this call site migrates to SourceIdentifier.Primary in Step 2
				SourceIdentifier: source.SourceIdentifier{Primary: v},
			}
			srcok = true
		}
	}
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			return src.Identifier //nolint:staticcheck // SA1019: intentional during Step 1 of the Source.Identifier migration (datadog-agent#51116); this call site migrates to SourceIdentifier.Primary in Step 2
		default:
			// We are not on a hostname (serverless), hence the hostname is empty
			return ""
		}
	}
	// fallback hostname from Agent conf.Hostname
	return fallbackHost
}

// GetOTelVersion returns the version based on OTel span and resource attributes, with span taking precedence.
func GetOTelVersion(span ptrace.Span, res pcommon.Resource) string {
	return LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptServiceVersion, true)
}

// GetOTelContainerID returns the container ID based on OTel span and resource attributes, with span taking precedence.
func GetOTelContainerID(span ptrace.Span, res pcommon.Resource) string {
	return LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptContainerID, true)
}

// GetOTelContainerOrPodID returns the container ID based on OTel span and resource attributes, with span taking precedence.
//
// The Kubernetes pod UID will be used as a fallback if the container ID is not found.
// This is only done for backward compatibility; consider using GetOTelContainerID instead.
func GetOTelContainerOrPodID(span ptrace.Span, res pcommon.Resource) string {
	accessor := semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes())
	if id := LookupSemanticStringWithAccessor(accessor, semantics.ConceptContainerID, true); id != "" {
		return id
	}
	return LookupSemanticStringWithAccessor(accessor, semantics.ConceptK8sPodUID, true)
}

// GetOTelStatusCode returns the HTTP status code based on OTel span and resource attributes, with span taking precedence.
func GetOTelStatusCode(span ptrace.Span, res pcommon.Resource) uint32 {
	code, ok := LookupSemanticInt64(span.Attributes(), res.Attributes(), semantics.ConceptHTTPStatusCode)
	if !ok || code < 0 {
		return 0
	}
	return uint32(code)
}

// GetOTelContainerTags returns a list of DD container tags in the OTel resource attributes.
// Tags are always normalized.
func GetOTelContainerTags(rattrs pcommon.Map, tagKeys []string) []string {
	var containerTags []string
	containerTagsMap := attributes.ContainerTagsFromResourceAttributes(rattrs)
	for _, key := range tagKeys {
		if mappedKey, ok := attributes.ContainerMappings[key]; ok {
			// If the key has a mapping in ContainerMappings, use the mapped key
			if val, ok := containerTagsMap[mappedKey]; ok {
				t := normalizeutil.NormalizeTag(mappedKey + ":" + val)
				containerTags = append(containerTags, t)
			}
		} else {
			// Otherwise populate as additional container tags
			if val := GetOTelAttrVal(rattrs, false, key); val != "" {
				t := normalizeutil.NormalizeTag(key + ":" + val)
				containerTags = append(containerTags, t)
			}
		}
	}
	return containerTags
}

// OtelSpanToDDSpan converts an OTel span to a DD span.
func OtelSpanToDDSpan(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	conf *config.AgentConfig,
) *pb.Span {
	spanKind := otelspan.Kind()
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	isTopLevel := false
	if topLevelByKind {
		isTopLevel = otelspan.ParentSpanID() == pcommon.NewSpanIDEmpty() || spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer
	}
	// Create one shared accessor for all span+resource lookups in this function and in the
	// minimal span conversion below, avoiding repeated allocation of accessor objects.
	spanAccessor := semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes())
	// No tag selections here: the full conversion below copies all span and
	// resource attributes into Meta, so peer and primary tags are already present.
	ddspan := otelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, spanAccessor, OTelSpanTagKeys{})

	// Span attributes take precedence over resource attributes in the event of key collisions; so, use span attributes first
	otelspan.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			conditionallyMapOTLPAttributeToMetric(k, v.Double(), ddspan)
		case pcommon.ValueTypeInt:
			conditionallyMapOTLPAttributeToMetric(k, float64(v.Int()), ddspan)
		case pcommon.ValueTypeBytes:
			if !SetMetaStructOTLP(ddspan, k, v) {
				conditionallyMapOTLPAttributeToMeta(k, v.AsString(), ddspan)
			}
		default:
			conditionallyMapOTLPAttributeToMeta(k, v.AsString(), ddspan)
		}

		return true
	})

	traceID := otelspan.TraceID()
	ddspan.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])
	if !spanMetaHasKey(ddspan, "version") {
		if version := LookupSemanticStringWithAccessor(spanAccessor, semantics.ConceptServiceVersion, true); version != "" {
			ddspan.Meta["version"] = version
		}
	}

	if otelspan.Events().Len() > 0 {
		ddspan.SpanEvents = OTelSpanEventsToDD(otelspan.Events())
	}
	TagSpanIfContainsExceptionEvent(otelspan, ddspan)
	if otelspan.Links().Len() > 0 {
		ddspan.SpanLinks = OTelSpanLinksToDD(otelspan.Links())
	}

	// Note: w3c.tracestate is set by otelSpanToDDSpanMinimal above.
	scopeConventionGateEnabled := !conf.HasFeature("disable_otel_scope_convention")
	if lib.Name() != "" {
		if scopeConventionGateEnabled {
			ddspan.Meta[string(semconv.OtelScopeNameKey)] = lib.Name()
		}
		// otel.library.name is a deprecated alias of otel.scope.name but MUST still be
		// reported with the same value for backward compatibility.
		ddspan.Meta[string(semconv.OtelLibraryNameKey)] = lib.Name()
	}
	if lib.Version() != "" {
		if scopeConventionGateEnabled {
			ddspan.Meta[string(semconv.OtelScopeVersionKey)] = lib.Version()
		}
		ddspan.Meta[string(semconv.OtelLibraryVersionKey)] = lib.Version()
	}
	ddspan.Meta[string(semconv.OtelStatusCodeKey)] = otelspan.Status().Code().String()
	if msg := otelspan.Status().Message(); msg != "" {
		ddspan.Meta[string(semconv.OtelStatusDescriptionKey)] = msg
	}

	if !spanMetaHasKey(ddspan, "error.msg") || !spanMetaHasKey(ddspan, "error.type") || !spanMetaHasKey(ddspan, "error.stack") {
		ddspan.Error = Status2Error(otelspan.Status(), otelspan.Events(), ddspan.Meta)
	}

	if !spanMetaHasKey(ddspan, "env") {
		if env := LookupSemanticStringWithAccessor(spanAccessor, semantics.ConceptDeploymentEnv, true); env != "" {
			ddspan.Meta["env"] = env
		}
	}

	otelres.Attributes().Range(func(k string, v pcommon.Value) bool {
		// Span attributes take precedence over resource attributes. Numeric span attributes
		// are stored in Metrics rather than Meta, so skip resource attributes whose key is
		// already present in Metrics to avoid emitting a stale duplicate in Meta.
		if _, ok := ddspan.Metrics[GetDDKeyForOTLPAttribute(k)]; ok {
			return true
		}
		value := v.AsString()
		conditionallyMapOTLPAttributeToMeta(k, value, ddspan)
		return true
	})

	for k, v := range lib.Attributes().Range {
		ddspan.Meta[k] = v.AsString()
	}

	// Check for db.namespace and conditionally set db.name.
	// db.namespace is a resource-level attribute; check resource attrs first, then span attrs.
	if _, ok := ddspan.Meta["db.name"]; !ok {
		if dbNamespace := LookupSemanticStringWithAccessor(
			semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes()),
			semantics.ConceptDBNamespace, false,
		); dbNamespace != "" {
			ddspan.Meta["db.name"] = dbNamespace
		}
	}

	return ddspan
}

// TagSpanIfContainsExceptionEvent tags spans that contain at least on exception span event.
func TagSpanIfContainsExceptionEvent(otelspan ptrace.Span, ddspan *pb.Span) {
	for i := range otelspan.Events().Len() {
		if otelspan.Events().At(i).Name() == "exception" {
			ddspan.Meta["_dd.span_events.has_exception"] = "true"
			return
		}
	}
}

// OTelSpanEventsToDD converts OTel span events into Datadog span events.
func OTelSpanEventsToDD(events ptrace.SpanEventSlice) []*pb.SpanEvent {
	ddEvents := make([]*pb.SpanEvent, 0, events.Len())
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		ddEvent := &pb.SpanEvent{
			TimeUnixNano: uint64(e.Timestamp()),
			Name:         e.Name(),
		}
		if e.Attributes().Len() > 0 {
			ddEvent.Attributes = make(map[string]*pb.AttributeAnyValue, e.Attributes().Len())
			e.Attributes().Range(func(k string, v pcommon.Value) bool {
				setEventAttribute(ddEvent.Attributes, k, v)
				return true
			})
		}
		ddEvents = append(ddEvents, ddEvent)
	}
	return ddEvents
}

// setEventAttribute sets the OTel attribute k=v in the Datadog span event attributes. Scalars
// and slices of scalars of a single type (or empty slices) are kept typed, bytes are base64
// encoded and empty values are dropped. Other slices are flattened into "k.0", "k.1", ... and maps into
// "k.<key>", as for span links, since the Datadog span event can't represent them.
func setEventAttribute(attrs map[string]*pb.AttributeAnyValue, k string, v pcommon.Value) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		attrs[k] = &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: v.Str()}
	case pcommon.ValueTypeBool:
		attrs[k] = &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_BOOL_VALUE, BoolValue: v.Bool()}
	case pcommon.ValueTypeInt:
		attrs[k] = &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_INT_VALUE, IntValue: v.Int()}
	case pcommon.ValueTypeDouble:
		attrs[k] = &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_DOUBLE_VALUE, DoubleValue: v.Double()}
	case pcommon.ValueTypeBytes:
		attrs[k] = &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: v.AsString()}
	case pcommon.ValueTypeSlice:
		s := v.Slice()
		if arr, ok := eventAttributeArray(s); ok {
			attrs[k] = &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: arr}
			return
		}
		for i := 0; i < s.Len(); i++ {
			setEventAttribute(attrs, k+"."+strconv.Itoa(i), s.At(i))
		}
	case pcommon.ValueTypeMap:
		v.Map().Range(func(mk string, mv pcommon.Value) bool {
			setEventAttribute(attrs, k+"."+mk, mv)
			return true
		})
	}
}

// eventAttributeArray converts s into a Datadog span event array attribute, empty when s is
// empty. It reports false when the values of s aren't all scalars of the same type, which a
// Datadog array can't represent.
func eventAttributeArray(s pcommon.Slice) (*pb.AttributeArray, bool) {
	if s.Len() == 0 {
		return &pb.AttributeArray{}, true
	}
	typ := s.At(0).Type()
	for i := 1; i < s.Len(); i++ {
		if s.At(i).Type() != typ {
			return nil, false
		}
	}
	values := make([]*pb.AttributeArrayValue, 0, s.Len())
	for i := 0; i < s.Len(); i++ {
		v := s.At(i)
		switch typ {
		case pcommon.ValueTypeStr:
			values = append(values, &pb.AttributeArrayValue{Type: pb.AttributeArrayValue_STRING_VALUE, StringValue: v.Str()})
		case pcommon.ValueTypeBool:
			values = append(values, &pb.AttributeArrayValue{Type: pb.AttributeArrayValue_BOOL_VALUE, BoolValue: v.Bool()})
		case pcommon.ValueTypeInt:
			values = append(values, &pb.AttributeArrayValue{Type: pb.AttributeArrayValue_INT_VALUE, IntValue: v.Int()})
		case pcommon.ValueTypeDouble:
			values = append(values, &pb.AttributeArrayValue{Type: pb.AttributeArrayValue_DOUBLE_VALUE, DoubleValue: v.Double()})
		default:
			return nil, false
		}
	}
	return &pb.AttributeArray{Values: values}, true
}

// OTelSpanLinksToDD converts OTel span links into Datadog span links.
func OTelSpanLinksToDD(links ptrace.SpanLinkSlice) []*pb.SpanLink {
	ddLinks := make([]*pb.SpanLink, 0, links.Len())
	for i := 0; i < links.Len(); i++ {
		l := links.At(i)
		traceID := l.TraceID()
		ddLink := &pb.SpanLink{
			TraceID:     OTelTraceIDToUint64(traceID),
			TraceIDHigh: binary.BigEndian.Uint64(traceID[:8]),
			SpanID:      OTelSpanIDToUint64(l.SpanID()),
			Tracestate:  l.TraceState().AsRaw(),
		}
		if flags := l.Flags(); flags != 0 {
			// Only the low 8 bits are W3C trace flags; OTLP uses bits 8 and 9 for is_remote. The
			// high bit marks the flags as set, telling "not sampled" apart from "unknown".
			ddLink.Flags = flags&0xff | 1<<31
		}
		if l.Attributes().Len() > 0 {
			ddLink.Attributes = make(map[string]string, l.Attributes().Len())
			l.Attributes().Range(func(k string, v pcommon.Value) bool {
				flattenLinkAttribute(ddLink.Attributes, k, v)
				return true
			})
		}
		ddLinks = append(ddLinks, ddLink)
	}
	return ddLinks
}

// flattenLinkAttribute sets the OTel attribute k=v in the string-valued Datadog span link
// attributes, flattening slices into "k.0", "k.1", ... and maps into "k.<key>" as tracers do.
func flattenLinkAttribute(attrs map[string]string, k string, v pcommon.Value) {
	switch v.Type() {
	case pcommon.ValueTypeSlice:
		s := v.Slice()
		for i := 0; i < s.Len(); i++ {
			flattenLinkAttribute(attrs, k+"."+strconv.Itoa(i), s.At(i))
		}
	case pcommon.ValueTypeMap:
		v.Map().Range(func(mk string, mv pcommon.Value) bool {
			flattenLinkAttribute(attrs, k+"."+mk, mv)
			return true
		})
	default:
		attrs[k] = v.AsString()
	}
}

// SetMetaOTLP sets the k/v OTLP attribute pair as a tag on span s.
func SetMetaOTLP(s *pb.Span, k, v string) {
	switch k {
	case "operation.name":
		s.Name = v
	case "service.name":
		s.Service = v
	case "resource.name":
		s.Resource = v
	case "span.type":
		s.Type = v
	case "analytics.event":
		if v, err := strconv.ParseBool(v); err == nil {
			if v {
				s.Metrics[sampler.KeySamplingRateEventExtraction] = 1
			} else {
				s.Metrics[sampler.KeySamplingRateEventExtraction] = 0
			}
		}
	default:
		s.Meta[k] = v
	}
}

// SetMetaOTLPIfEmpty sets the k/v OTLP attribute pair as a tag on span s, if the corresponding value hasn't been set already.
func SetMetaOTLPIfEmpty(s *pb.Span, k, v string) {
	switch k {
	case "operation.name":
		if s.Name == "" {
			s.Name = v
		}
	case "service.name":
		if s.Service == "" {
			s.Service = v
		}
	case "resource.name":
		if s.Resource == "" {
			s.Resource = v
		}
	case "span.type":
		if s.Type == "" {
			s.Type = v
		}
	case "analytics.event":
		if v, err := strconv.ParseBool(v); err == nil {
			if _, ok := s.Metrics[sampler.KeySamplingRateEventExtraction]; !ok {
				if v {
					s.Metrics[sampler.KeySamplingRateEventExtraction] = 1
				} else {
					s.Metrics[sampler.KeySamplingRateEventExtraction] = 0
				}
			}
		}
	default:
		s.Meta[k] = v
	}
}

// SetMetricOTLP sets the k/v OTLP attribute pair as a metric on span s.
func SetMetricOTLP(s *pb.Span, k string, v float64) {
	switch k {
	case "sampling.priority":
		s.Metrics["_sampling_priority_v1"] = v
	default:
		s.Metrics[k] = v
	}
}

// SetMetricOTLPIfEmpty sets the k/v OTLP attribute pair as a metric on span s, if the corresponding value hasn't been set already.
func SetMetricOTLPIfEmpty(s *pb.Span, k string, v float64) {
	var key string
	switch k {
	case "sampling.priority":
		key = "_sampling_priority_v1"
	default:
		key = k
	}
	if _, ok := s.Metrics[key]; !ok {
		s.Metrics[key] = v
	}
}

// Status2Error checks the given status and events and applies any potential error and messages
// to the given span attributes.
// Note: This function uses the semantic registry for http.status_code lookup precedence.
// Previously it used NEW-first precedence (http.response.status_code before http.status_code).
func Status2Error(status ptrace.Status, _ ptrace.SpanEventSlice, metaMap map[string]string) int32 {
	if status.Code() != ptrace.StatusCodeError {
		return 0
	}
	if _, ok := metaMap["error.msg"]; !ok {
		// no error message was extracted, find alternatives
		if status.Message() != "" {
			// use the status message
			metaMap["error.msg"] = status.Message()
		} else {
			// Use semantic registry lookup for http.status_code
			// `http.status_code` was renamed to `http.response.status_code` in the HTTP stabilization from v1.23.
			// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes
			accessor := semantics.NewStringMapAccessor(metaMap)
			httpCode := semantics.LookupString(semantics.DefaultRegistry(), accessor, semantics.ConceptHTTPStatusCode)
			if httpCode != "" {
				httpCodeInt, _ := strconv.Atoi(httpCode) // Value returned in case of error will not pass the next test
				if httptext := http.StatusText(httpCodeInt); httptext != "" {
					metaMap["error.msg"] = fmt.Sprintf("%s %s", httpCode, httptext)
				} else {
					metaMap["error.msg"] = httpCode
				}
			}
		}
	}
	return 1
}

// GetFirstFromMap checks each key in the given keys in the map and returns the first key-value pair whose
// key matches, or empty strings if none matches.
func GetFirstFromMap(m map[string]string, keys ...string) (string, string) {
	for _, key := range keys {
		if val := m[key]; val != "" {
			return key, val
		}
	}
	return "", ""
}

func spanMetaHasKey(s *pb.Span, k string) bool {
	_, ok := s.Meta[k]
	return ok
}
