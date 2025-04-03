// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package transform implements mappings from OTLP to DD semantics, and helpers
package transform

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.6.1"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
)

const (
	// KeyDatadogService is the key for the service name in the Datadog namespace
	KeyDatadogService = "datadog.service"
	// KeyDatadogName is the key for the operation name in the Datadog namespace
	KeyDatadogName = "datadog.name"
	// KeyDatadogResource is the key for the resource name in the Datadog namespace
	KeyDatadogResource = "datadog.resource"
	// KeyDatadogSpanKind is the key for the span kind in the Datadog namespace
	KeyDatadogSpanKind = "datadog.span.kind"
	// KeyDatadogType is the key for the span type in the Datadog namespace
	KeyDatadogType = "datadog.type"
	// KeyDatadogError is the key for the error flag in the Datadog namespace
	KeyDatadogError = "datadog.error"
	// KeyDatadogErrorMsg is the key for the error message in the Datadog namespace
	KeyDatadogErrorMsg = "datadog.error.msg"
	// KeyDatadogErrorType is the key for the error type in the Datadog namespace
	KeyDatadogErrorType = "datadog.error.type"
	// KeyDatadogErrorStack is the key for the error stack in the Datadog namespace
	KeyDatadogErrorStack = "datadog.error.stack"
	// KeyDatadogVersion is the key for the version in the Datadog namespace
	KeyDatadogVersion = "datadog.version"
	// KeyDatadogHTTPStatusCode is the key for the HTTP status code in the Datadog namespace
	KeyDatadogHTTPStatusCode = "datadog.http_status_code"
	// KeyDatadogHost is the key for the host in the Datadog namespace
	KeyDatadogHost = "datadog.host"
	// KeyDatadogEnvironment is the key for the environment in the Datadog namespace
	KeyDatadogEnvironment = "datadog.env"
	// KeyDatadogContainerID is the key for the container ID in the Datadog namespace
	KeyDatadogContainerID = "datadog.container_id"
	// KeyDatadogContainerTags is the key for the container tags in the Datadog namespace
	KeyDatadogContainerTags = "datadog.container_tags"
)

// OperationAndResourceNameV2Enabled checks if the new operation and resource name logic should be used
func OperationAndResourceNameV2Enabled(conf *config.AgentConfig) bool {
	return !conf.OTLPReceiver.SpanNameAsResourceName && len(conf.OTLPReceiver.SpanNameRemappings) == 0 && conf.HasFeature("enable_operation_and_resource_name_logic_v2")
}

// OtelSpanToDDSpanMinimal otelSpanToDDSpan converts an OTel span to a DD span.
// The converted DD span only has the minimal number of fields for APM stats calculation and is only meant
// to be used in OTLPTracesToConcentratorInputs. Do not use them for other purposes.
func OtelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
) *pb.Span {
	spanKind := otelspan.Kind()

	ddspan := &pb.Span{
		Service:  traceutil.GetOTelAttrVal(otelspan.Attributes(), true, KeyDatadogService),
		Name:     traceutil.GetOTelAttrVal(otelspan.Attributes(), true, KeyDatadogName),
		Resource: traceutil.GetOTelAttrVal(otelspan.Attributes(), true, KeyDatadogResource),
		Type:     traceutil.GetOTelAttrVal(otelspan.Attributes(), true, KeyDatadogType),
		TraceID:  traceutil.OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   traceutil.OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: traceutil.OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Meta:     make(map[string]string, otelspan.Attributes().Len()+otelres.Attributes().Len()),
		Metrics:  make(map[string]float64),
	}
	if isErrorVal, ok := otelspan.Attributes().Get(KeyDatadogError); ok {
		ddspan.Error = int32(isErrorVal.Int())
	} else {
		if otelspan.Status().Code() == ptrace.StatusCodeError {
			ddspan.Error = 1
		}
	}

	if incomingSpanKindName := traceutil.GetOTelAttrVal(otelspan.Attributes(), true, KeyDatadogSpanKind); incomingSpanKindName != "" {
		ddspan.Meta["span.kind"] = incomingSpanKindName
	}

	if !conf.OTLPReceiver.IgnoreMissingDatadogFields {
		if ddspan.Service == "" {
			ddspan.Service = traceutil.GetOTelService(otelres, true)
		}

		if OperationAndResourceNameV2Enabled(conf) {
			if ddspan.Name == "" {
				ddspan.Name = traceutil.GetOTelOperationNameV2(otelspan)
			}
			if ddspan.Resource == "" {
				ddspan.Resource = traceutil.GetOTelResourceV2(otelspan, otelres)
			}
		} else {
			if ddspan.Name == "" {
				ddspan.Name = traceutil.GetOTelOperationNameV1(otelspan, otelres, lib, conf.OTLPReceiver.SpanNameAsResourceName, conf.OTLPReceiver.SpanNameRemappings, true)
			}
			if ddspan.Resource == "" {
				ddspan.Resource = traceutil.GetOTelResourceV1(otelspan, otelres)
			}
		}

		if ddspan.Type == "" {
			// correct span type logic if using new resource receiver, keep same if on v1. separate from OperationAndResourceNameV2Enabled.
			if !conf.HasFeature("disable_receive_resource_spans_v2") {
				ddspan.Type = traceutil.GetOTelSpanType(otelspan, otelres)
			} else {
				ddspan.Type = traceutil.GetOTelAttrValInResAndSpanAttrs(otelspan, otelres, true, "span.type")
				if ddspan.Type == "" {
					ddspan.Type = traceutil.SpanKind2Type(otelspan, otelres)
				}
			}
		}

		if !spanMetaHasKey(ddspan, "span.kind") {
			ddspan.Meta["span.kind"] = traceutil.OTelSpanKindName(spanKind)
		}
		var code uint32
		if incomingCode, ok := otelspan.Attributes().Get(KeyDatadogHTTPStatusCode); ok {
			code = uint32(incomingCode.Int())
		} else {
			code = traceutil.GetOTelStatusCode(otelspan)
		}
		if code != 0 {
			ddspan.Metrics[traceutil.TagStatusCode] = float64(code)
		}
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
			ddspan.Meta[peerTagKey] = peerTagVal
		}
	}
	return ddspan
}

func isDatadogAPMConventionKey(k string) bool {
	return k == "service.name" || k == "operation.name" || k == "resource.name" || k == "span.type" || strings.HasPrefix(k, "datadog.")
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
		mappedKey = fmt.Sprintf("http.request.headers.%s", strings.TrimPrefix(k, "http.request.header."))
	case !isDatadogAPMConventionKey(k):
		mappedKey = k
	default:
		return ""
	}
	return mappedKey
}

func setMetaOTLPWithSemConvMappings(k string, value string, ddspan *pb.Span, ignoreMissingDatadogFields bool) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	// Exclude Datadog APM conventions.
	// These are handled above explicitly.
	if mappedKey != "" {
		if _, ok := metaKeysToDDSemanticsKeys[mappedKey]; ok {
			if ddspan.Meta[mappedKey] != "" || ignoreMissingDatadogFields {
				return
			}
		}
		SetMetaOTLP(ddspan, mappedKey, value)
	}
}

func setMetricOTLPWithSemConvMappings(k string, value float64, ddspan *pb.Span, ignoreMissingDatadogFields bool) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	// Exclude Datadog APM conventions.
	// These are handled above explicitly.
	if mappedKey != "" {
		if _, ok := metaKeysToDDSemanticsKeys[mappedKey]; ok {
			if _, ok := ddspan.Metrics[mappedKey]; ok || ignoreMissingDatadogFields {
				return
			}
		}
		SetMetricOTLP(ddspan, mappedKey, value)
	}
}

var ddSemanticsKeysToMetaKeys = map[string]string{
	KeyDatadogEnvironment:    "env",
	KeyDatadogVersion:        "version",
	KeyDatadogHTTPStatusCode: "http.status_code",
	KeyDatadogErrorMsg:       "error.msg",
	KeyDatadogErrorType:      "error.type",
	KeyDatadogErrorStack:     "error.stack",
}

var metaKeysToDDSemanticsKeys = map[string]string{
	"env":              KeyDatadogEnvironment,
	"version":          KeyDatadogVersion,
	"http.status_code": KeyDatadogHTTPStatusCode,
	"error.msg":        KeyDatadogErrorMsg,
	"error.type":       KeyDatadogErrorType,
	"error.stack":      KeyDatadogErrorStack,
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
	ddspan := OtelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, nil)

	for ddSemanticKey, ddSpanMetaKey := range ddSemanticsKeysToMetaKeys {
		if incomingValue := traceutil.GetOTelAttrVal(otelspan.Attributes(), false, ddSemanticKey); incomingValue != "" {
			ddspan.Meta[ddSpanMetaKey] = incomingValue
		}
	}

	otelres.Attributes().Range(func(k string, v pcommon.Value) bool {
		value := v.AsString()
		setMetaOTLPWithSemConvMappings(k, value, ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		return true
	})

	traceID := otelspan.TraceID()
	ddspan.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])
	if !spanMetaHasKey(ddspan, "version") {
		if serviceVersion, ok := otelres.Attributes().Get(semconv.AttributeServiceVersion); ok {
			ddspan.Meta["version"] = serviceVersion.AsString()
		}
	}

	if otelspan.Events().Len() > 0 {
		ddspan.Meta["events"] = MarshalEvents(otelspan.Events())
	}
	TagSpanIfContainsExceptionEvent(otelspan, ddspan)
	if otelspan.Links().Len() > 0 {
		ddspan.Meta["_dd.span_links"] = MarshalLinks(otelspan.Links())
	}

	otelspan.Attributes().Range(func(k string, v pcommon.Value) bool {
		if strings.HasPrefix(k, "datadog.") {
			return true
		}
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			setMetricOTLPWithSemConvMappings(k, v.Double(), ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		case pcommon.ValueTypeInt:
			setMetricOTLPWithSemConvMappings(k, float64(v.Int()), ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		default:
			setMetaOTLPWithSemConvMappings(k, v.AsString(), ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		}

		return true
	})

	if otelspan.TraceState().AsRaw() != "" {
		ddspan.Meta["w3c.tracestate"] = otelspan.TraceState().AsRaw()
	}
	if lib.Name() != "" {
		ddspan.Meta[semconv.OtelLibraryName] = lib.Name()
	}
	if lib.Version() != "" {
		ddspan.Meta[semconv.OtelLibraryVersion] = lib.Version()
	}
	ddspan.Meta[semconv.OtelStatusCode] = otelspan.Status().Code().String()
	if msg := otelspan.Status().Message(); msg != "" {
		ddspan.Meta[semconv.OtelStatusDescription] = msg
	}

	if !conf.OTLPReceiver.IgnoreMissingDatadogFields {
		if !spanMetaHasKey(ddspan, "error.msg") || !spanMetaHasKey(ddspan, "error.type") || !spanMetaHasKey(ddspan, "error.stack") {
			ddspan.Error = Status2Error(otelspan.Status(), otelspan.Events(), ddspan.Meta)
		}

		if !spanMetaHasKey(ddspan, "env") {
			if env := traceutil.GetOTelEnv(otelres); env != "" {
				ddspan.Meta["env"] = env
			}
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

// SetMetricOTLP sets the k/v OTLP attribute pair as a metric on span s.
func SetMetricOTLP(s *pb.Span, k string, v float64) {
	switch k {
	case "sampling.priority":
		s.Metrics["_sampling_priority_v1"] = v
	default:
		s.Metrics[k] = v
	}
}

// Status2Error checks the given status and events and applies any potential error and messages
// to the given span attributes.
func Status2Error(status ptrace.Status, events ptrace.SpanEventSlice, metaMap map[string]string) int32 {
	if status.Code() != ptrace.StatusCodeError {
		return 0
	}
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if strings.ToLower(e.Name()) != "exception" {
			continue
		}
		attrs := e.Attributes()
		if v, ok := attrs.Get(semconv.AttributeExceptionMessage); ok {
			metaMap["error.msg"] = v.AsString()
		}
		if v, ok := attrs.Get(semconv.AttributeExceptionType); ok {
			metaMap["error.type"] = v.AsString()
		}
		if v, ok := attrs.Get(semconv.AttributeExceptionStacktrace); ok {
			metaMap["error.stack"] = v.AsString()
		}
	}
	if _, ok := metaMap["error.msg"]; !ok {
		// no error message was extracted, find alternatives
		if status.Message() != "" {
			// use the status message
			metaMap["error.msg"] = status.Message()
		} else if _, httpcode := GetFirstFromMap(metaMap, "http.response.status_code", "http.status_code"); httpcode != "" {
			// `http.status_code` was renamed to `http.response.status_code` in the HTTP stabilization from v1.23.
			// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes

			// http.status_text was removed in spec v0.7.0 (https://github.com/open-telemetry/opentelemetry-specification/pull/972)
			// TODO (OTEL-1791) Remove this and use a map from status code to status text.
			if httptext, ok := metaMap["http.status_text"]; ok {
				metaMap["error.msg"] = fmt.Sprintf("%s %s", httpcode, httptext)
			} else {
				metaMap["error.msg"] = httpcode
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
