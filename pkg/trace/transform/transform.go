// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package transform implements mappings from OTLP to DD semantics, and helpers
package transform

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	traceutilotel "github.com/DataDog/datadog-agent/pkg/trace/otel/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OperationAndResourceNameV2Enabled checks if the new operation and resource name logic should be used
func OperationAndResourceNameV2Enabled(conf *config.AgentConfig) bool {
	return !conf.OTLPReceiver.SpanNameAsResourceName && len(conf.OTLPReceiver.SpanNameRemappings) == 0 && !conf.HasFeature("disable_operation_and_resource_name_logic_v2")
}

// otelSpanToDDSpanMinimal is the internal implementation shared by OtelSpanToDDSpanMinimal and
// OtelSpanToDDSpan, accepting a pre-created accessor to avoid repeated allocation.
func otelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
	spanAccessor semantics.Accessor,
) *pb.Span {
	spanKind := otelspan.Kind()

	sattr := otelspan.Attributes()
	rattr := otelres.Attributes()

	ddspan := &pb.Span{
		Service:  traceutilotel.GetOTelServiceWithAccessor(spanAccessor, true),
		TraceID:  traceutilotel.OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   traceutilotel.OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: traceutilotel.OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Meta:     make(map[string]string, sattr.Len()+rattr.Len()),
		Metrics:  make(map[string]float64),
	}

	if otelspan.Status().Code() == ptrace.StatusCodeError {
		ddspan.Error = 1
	}

	if OperationAndResourceNameV2Enabled(conf) {
		ddspan.Name = traceutilotel.GetOTelOperationNameV2WithAccessor(otelspan, spanAccessor)
		ddspan.Resource = traceutilotel.GetOTelResourceV2WithAccessor(otelspan, spanAccessor)
	} else {
		ddspan.Name = traceutilotel.GetOTelOperationNameV1(otelspan, otelres, lib, conf.OTLPReceiver.SpanNameAsResourceName, conf.OTLPReceiver.SpanNameRemappings, true)
		ddspan.Resource = traceutilotel.GetOTelResourceV1(otelspan, otelres)
	}

	// correct span type logic if using new resource receiver, keep same if on v1. separate from OperationAndResourceNameV2Enabled.
	if !conf.HasFeature("disable_receive_resource_spans_v2") {
		ddspan.Type = traceutilotel.GetOTelSpanTypeWithAccessor(otelspan, spanAccessor)
	} else {
		ddspan.Type = traceutilotel.LookupSemanticStringFromDualMaps(rattr, sattr, semantics.ConceptSpanType, true)
		if ddspan.Type == "" {
			ddspan.Type = traceutilotel.SpanKind2Type(otelspan, otelres)
		}
	}

	ddspan.Meta["span.kind"] = traceutilotel.OTelSpanKindName(spanKind)

	reg := semantics.DefaultRegistry()
	if code, ok := semantics.LookupInt64(reg, spanAccessor, semantics.ConceptHTTPStatusCode); ok && code >= 0 {
		ddspan.Metrics[traceutil.TagStatusCode] = float64(code)
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
	// Use direct lookup only: peerTagKeys already lists every precursor from peer_tags.ini (semantic-core).
	for _, peerTagKey := range peerTagKeys {
		if peerTagVal := traceutilotel.GetOTelAttrFromEitherMap(sattr, rattr, false, peerTagKey); peerTagVal != "" {
			ddspan.Meta[peerTagKey] = peerTagVal
		}
	}
	return ddspan
}

// OtelSpanToDDSpanMinimal converts an OTel span to a DD span with only the minimal fields
// needed for APM stats calculation. Only use with OTLPTracesToConcentratorInputs.
func OtelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
) *pb.Span {
	spanAccessor := semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes())
	return otelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, peerTagKeys, spanAccessor)
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
	if ddspan.Meta[mappedKey] != "" {
		return
	}

	// Exclude Datadog APM conventions.
	// These are handled explicitly elsewhere.
	if mappedKey != "" {
		SetMetaOTLPIfEmpty(ddspan, mappedKey, value)
	}
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
	return traceutilotel.LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptDeploymentEnv, true)
}

// GetOTelHostname returns the DD hostname based on OTel span and resource attributes, with span taking precedence.
func GetOTelHostname(span ptrace.Span, res pcommon.Resource, tr *attributes.Translator, fallbackHost string) string {
	ctx := context.Background()
	src, srcok := tr.ResourceToSource(ctx, res, traceutilotel.SignalTypeSet, nil)
	if !srcok {
		if v := traceutilotel.GetOTelAttrValInResAndSpanAttrs(span, res, false, "_dd.hostname"); v != "" {
			src = source.Source{Kind: source.HostnameKind, Identifier: v}
			srcok = true
		}
	}
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			return src.Identifier
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
	return traceutilotel.LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptServiceVersion, true)
}

// GetOTelContainerID returns the container ID based on OTel span and resource attributes, with span taking precedence.
func GetOTelContainerID(span ptrace.Span, res pcommon.Resource) string {
	return traceutilotel.LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptContainerID, true)
}

// GetOTelContainerOrPodID returns the container ID based on OTel span and resource attributes, with span taking precedence.
//
// The Kubernetes pod UID will be used as a fallback if the container ID is not found.
// This is only done for backward compatibility; consider using GetOTelContainerID instead.
func GetOTelContainerOrPodID(span ptrace.Span, res pcommon.Resource) string {
	accessor := semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes())
	if id := traceutilotel.LookupSemanticStringWithAccessor(accessor, semantics.ConceptContainerID, true); id != "" {
		return id
	}
	return traceutilotel.LookupSemanticStringWithAccessor(accessor, semantics.ConceptK8sPodUID, true)
}

// GetOTelStatusCode returns the HTTP status code based on OTel span and resource attributes, with span taking precedence.
func GetOTelStatusCode(span ptrace.Span, res pcommon.Resource) uint32 {
	code, ok := traceutilotel.LookupSemanticInt64(span.Attributes(), res.Attributes(), semantics.ConceptHTTPStatusCode)
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
			if val := traceutilotel.GetOTelAttrVal(rattrs, false, key); val != "" {
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
	ddspan := otelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, nil, spanAccessor)

	// Span attributes take precedence over resource attributes in the event of key collisions; so, use span attributes first
	otelspan.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			conditionallyMapOTLPAttributeToMetric(k, v.Double(), ddspan)
		case pcommon.ValueTypeInt:
			conditionallyMapOTLPAttributeToMetric(k, float64(v.Int()), ddspan)
		default:
			conditionallyMapOTLPAttributeToMeta(k, v.AsString(), ddspan)
		}

		return true
	})

	traceID := otelspan.TraceID()
	ddspan.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])
	if !spanMetaHasKey(ddspan, "version") {
		if version := traceutilotel.LookupSemanticStringWithAccessor(spanAccessor, semantics.ConceptServiceVersion, true); version != "" {
			ddspan.Meta["version"] = version
		}
	}

	if otelspan.Events().Len() > 0 {
		ddspan.Meta["events"] = MarshalEvents(otelspan.Events())
	}
	TagSpanIfContainsExceptionEvent(otelspan, ddspan)
	if otelspan.Links().Len() > 0 {
		ddspan.Meta["_dd.span_links"] = MarshalLinks(otelspan.Links())
	}

	if otelspan.TraceState().AsRaw() != "" {
		ddspan.Meta["w3c.tracestate"] = otelspan.TraceState().AsRaw()
	}
	if lib.Name() != "" {
		ddspan.Meta[string(semconv.OtelLibraryNameKey)] = lib.Name()
	}
	if lib.Version() != "" {
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
		if env := traceutilotel.LookupSemanticStringWithAccessor(spanAccessor, semantics.ConceptDeploymentEnv, true); env != "" {
			ddspan.Meta["env"] = env
		}
	}

	otelres.Attributes().Range(func(k string, v pcommon.Value) bool {
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
		if dbNamespace := traceutilotel.LookupSemanticStringWithAccessor(
			semantics.NewOTelSpanAccessor(otelres.Attributes(), otelspan.Attributes()),
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

// MarshalEvents marshals events into JSON.
func MarshalEvents(events ptrace.SpanEventSlice) string {
	var str strings.Builder
	str.WriteString("[")
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if i > 0 {
			str.WriteString(",")
		}
		var wrote bool
		str.WriteString("{")
		if v := e.Timestamp(); v != 0 {
			str.WriteString(`"time_unix_nano":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
			wrote = true
		}
		if v := e.Name(); v != "" {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"name":`)
			if name, err := json.Marshal(v); err == nil {
				str.WriteString(string(name))
			} else {
				// still collect the event information, if possible
				log.Errorf("Error parsing span event name %v, using name 'redacted' instead", name)
				str.WriteString(`"redacted"`)
			}
			wrote = true
		}
		if e.Attributes().Len() > 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"attributes":{`)
			j := 0
			e.Attributes().Range(func(k string, v pcommon.Value) bool {
				// collect the attribute only if the key is json-parseable, else drop the attribute
				if key, err := json.Marshal(k); err == nil {
					if j > 0 {
						str.WriteString(",")
					}
					str.WriteString(string(key))
					str.WriteString(":")
					if val, err := json.Marshal(v.AsRaw()); err == nil {
						str.WriteString(string(val))
					} else {
						log.Warnf("Trouble parsing the following attribute value, dropping: %v", v.AsString())
						str.WriteString(`"redacted"`)
					}
					j++
				} else {
					log.Errorf("Error parsing the following attribute key on span event %v, dropping attribute: %v", e.Name(), k)
					e.SetDroppedAttributesCount(e.DroppedAttributesCount() + 1)
				}
				j++
				return true
			})
			str.WriteString("}")
			wrote = true
		}
		if v := e.DroppedAttributesCount(); v != 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"dropped_attributes_count":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
		}
		str.WriteString("}")
	}
	str.WriteString("]")
	return str.String()
}

// MarshalLinks marshals span links into JSON.
func MarshalLinks(links ptrace.SpanLinkSlice) string {
	var str strings.Builder
	str.WriteString("[")
	for i := 0; i < links.Len(); i++ {
		l := links.At(i)
		if i > 0 {
			str.WriteString(",")
		}
		t := l.TraceID()
		str.WriteString(`{"trace_id":"`)
		str.WriteString(hex.EncodeToString(t[:]))
		s := l.SpanID()
		str.WriteString(`","span_id":"`)
		str.WriteString(hex.EncodeToString(s[:]))
		str.WriteString(`"`)
		if ts := l.TraceState().AsRaw(); len(ts) > 0 {
			str.WriteString(`,"tracestate":"`)
			str.WriteString(ts)
			str.WriteString(`"`)
		}
		if l.Attributes().Len() > 0 {
			str.WriteString(`,"attributes":{`)
			var b bool
			l.Attributes().Range(func(k string, v pcommon.Value) bool {
				if b {
					str.WriteString(",")
				}
				b = true
				str.WriteString(`"`)
				str.WriteString(k)
				str.WriteString(`":"`)
				str.WriteString(v.AsString())
				str.WriteString(`"`)
				return true
			})
			str.WriteString("}")
		}
		if l.DroppedAttributesCount() > 0 {
			str.WriteString(`,"dropped_attributes_count":`)
			str.WriteString(strconv.FormatUint(uint64(l.DroppedAttributesCount()), 10))
		}
		str.WriteString("}")
	}
	str.WriteString("]")
	return str.String()
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
