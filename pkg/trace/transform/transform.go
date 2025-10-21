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
	semconv "go.opentelemetry.io/otel/semconv/v1.15.0"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var emptyString = ""

// OperationAndResourceNameV2Enabled checks if the new operation and resource name logic should be used
func OperationAndResourceNameV2Enabled(conf *config.AgentConfig) bool {
	return !conf.OTLPReceiver.SpanNameAsResourceName && len(conf.OTLPReceiver.SpanNameRemappings) == 0 && !conf.HasFeature("disable_operation_and_resource_name_logic_v2")
}

// OTelCompliantTranslationEnabled checks if the new OTLP to Datadog translations should be used
func OTelCompliantTranslationEnabled(conf *config.AgentConfig) bool {
	return conf.HasFeature("enable_otel_compliant_translation")
}

func otelSpanToDDSpanMinimalOld(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
) *pb.Span {
	spanKind := otelspan.Kind()

	sattr := otelspan.Attributes()
	rattr := otelres.Attributes()

	ddspan := &pb.Span{
		Service:  traceutil.GetOTelAttrFromEitherMap(sattr, rattr, true, attributes.DDNamespaceKeys.Service()),
		Name:     traceutil.GetOTelAttrFromEitherMap(sattr, rattr, true, attributes.DDNamespaceKeys.OperationName()),
		Resource: traceutil.GetOTelAttrFromEitherMap(sattr, rattr, true, attributes.DDNamespaceKeys.ResourceName()),
		Type:     traceutil.GetOTelAttrFromEitherMap(sattr, rattr, true, attributes.DDNamespaceKeys.SpanType()),
		TraceID:  traceutil.OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   traceutil.OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: traceutil.OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Meta:     make(map[string]string, sattr.Len()+rattr.Len()),
		Metrics:  make(map[string]float64),
	}
	if isErrorVal, ok := otelspan.Attributes().Get("datadog.error"); ok {
		ddspan.Error = int32(isErrorVal.Int())
	} else {
		if otelspan.Status().Code() == ptrace.StatusCodeError {
			ddspan.Error = 1
		}
	}

	if incomingSpanKindName := traceutil.GetOTelAttrFromEitherMap(sattr, rattr, true, attributes.DDNamespaceKeys.SpanKind()); incomingSpanKindName != "" {
		ddspan.Meta["span.kind"] = incomingSpanKindName
	}

	code := GetOTelStatusCode(otelspan, otelres, conf.OTLPReceiver.IgnoreMissingDatadogFields)

	if !conf.OTLPReceiver.IgnoreMissingDatadogFields {
		if ddspan.Service == "" {
			ddspan.Service = traceutil.GetOTelService(otelspan, otelres, true)
		}

		if OperationAndResourceNameV2Enabled(conf) {
			if ddspan.Name == "" {
				ddspan.Name = traceutil.GetOTelOperationNameV2(otelspan, otelres)
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
	}

	if code != 0 {
		ddspan.Metrics[traceutil.TagStatusCode] = float64(code)
	}
	if isTopLevel {
		traceutil.SetTopLevel(ddspan, true)
	}
	if isMeasured := traceutil.GetOTelAttrFromEitherMap(sattr, rattr, false, "_dd.measured"); isMeasured == "1" {
		traceutil.SetMeasured(ddspan, true)
	} else if topLevelByKind && (spanKind == ptrace.SpanKindClient || spanKind == ptrace.SpanKindProducer) {
		// When enable_otlp_compute_top_level_by_span_kind is true, compute stats for client-side spans
		traceutil.SetMeasured(ddspan, true)
	}
	for _, peerTagKey := range peerTagKeys {
		if peerTagVal := traceutil.GetOTelAttrFromEitherMap(sattr, rattr, false, peerTagKey); peerTagVal != "" {
			ddspan.Meta[peerTagKey] = peerTagVal
		}
	}
	return ddspan
}

func isDatadogAPMConventionKey(k string) bool {
	return k == "service.name" || k == "operation.name" || k == "resource.name" || k == "span.type" || strings.HasPrefix(k, "datadog.")
}

func otelSpanToDDSpanMinimalOTelCompliantTranslation(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
) *pb.Span {
	spanKind := otelspan.Kind()

	sattr := otelspan.Attributes()
	rattr := otelres.Attributes()

	ignore := conf.OTLPReceiver.IgnoreMissingDatadogFields

	spanName := otelspan.Name()
	ddspan := &pb.Span{
		Name:     attributes.GetOperationName(spanKind, sattr, ignore, true, nil),
		Resource: attributes.GetResourceName(spanKind, sattr, ignore, true, &spanName),
		Service:  attributes.GetService(rattr, true, ignore, true, nil),
		TraceID:  traceutil.OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   traceutil.OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: traceutil.OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Meta:     make(map[string]string, sattr.Len()+rattr.Len()),
		Metrics:  make(map[string]float64, 4),
	}
	ddspan.Meta[attributes.APMConventionKeys.SpanKind()] = attributes.GetSpanKind(spanKind, sattr, ignore, true, nil)

	if otelspan.Status().Code() == ptrace.StatusCodeError {
		ddspan.Error = 1
	}

	code, err := attributes.GetStatusCode(sattr, ignore, true, nil)
	if err != nil {
		log.Errorf("error getting OTel status code: %v", err)
	}
	if code != 0 {
		ddspan.Meta[attributes.APMConventionKeys.HTTPStatusCode()] = strconv.FormatUint(uint64(code), 10)
		ddspan.Metrics[traceutil.TagStatusCode] = float64(code)
	}

	// correct span type logic if using new resource receiver, keep same if on v1. separate from OperationAndResourceNameV2Enabled.
	if !conf.HasFeature("disable_receive_resource_spans_v2") {
		ddspan.Type = attributes.GetSpanType(spanKind, sattr, ignore, true, nil)
	} else {
		ddspan.Type = attributes.GetOTelAttrFromEitherMap(sattr, rattr, true, attributes.DDNamespaceKeys.SpanType(), "span.type")
		if ddspan.Type == "" {
			ddspan.Type = traceutil.SpanKind2Type(otelspan, otelres)
		}
	}
	if isTopLevel {
		traceutil.SetTopLevel(ddspan, true)
	}
	if isMeasured := traceutil.GetOTelAttrFromEitherMap(sattr, rattr, false, "_dd.measured"); isMeasured == "1" {
		traceutil.SetMeasured(ddspan, true)
	} else if topLevelByKind && (spanKind == ptrace.SpanKindClient || spanKind == ptrace.SpanKindProducer) {
		// When enable_otlp_compute_top_level_by_span_kind is true, compute stats for client-side spans
		traceutil.SetMeasured(ddspan, true)
	}
	for _, peerTagKey := range peerTagKeys {
		if peerTagVal := traceutil.GetOTelAttrFromEitherMap(sattr, rattr, false, peerTagKey); peerTagVal != "" {
			ddspan.Meta[peerTagKey] = peerTagVal
		}
	}
	return ddspan
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
	if OTelCompliantTranslationEnabled(conf) {
		return otelSpanToDDSpanMinimalOTelCompliantTranslation(otelspan, otelres, isTopLevel, topLevelByKind, conf, peerTagKeys)
	}
	return otelSpanToDDSpanMinimalOld(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, peerTagKeys)
}

var otlpToAPMConventionKeysMapForSpanAttributes = map[string]string{
	string(semconv127.HTTPResponseStatusCodeKey): attributes.APMConventionKeys.HTTPStatusCode(),
	string(semconv117.HTTPStatusCodeKey):         attributes.APMConventionKeys.HTTPStatusCode(),

	string(semconv127.DBNamespaceKey): attributes.APMConventionKeys.DBName(),
}

// These are keys in the `datadog.*` namespace that belong in span attributes.
var ddNamespaceToAPMConventionKeysForSpanAttributes = map[string]string{
	attributes.DDNamespaceKeys.OperationName():  attributes.APMConventionKeys.OperationName(),
	attributes.DDNamespaceKeys.ResourceName():   attributes.APMConventionKeys.ResourceName(),
	attributes.DDNamespaceKeys.SpanType():       attributes.APMConventionKeys.SpanType(),
	attributes.DDNamespaceKeys.SpanKind():       attributes.APMConventionKeys.SpanKind(),
	attributes.DDNamespaceKeys.HTTPStatusCode(): attributes.APMConventionKeys.HTTPStatusCode(),
	attributes.DDNamespaceKeys.DBName():         attributes.APMConventionKeys.DBName(),
}

// These keys must be set with more complex logic via methods such as attributes.GetEnv, etc.
var reservedAPMConventionKeysForSpanAttributes = map[string]struct{}{
	attributes.APMConventionKeys.OperationName():  {},
	attributes.APMConventionKeys.ResourceName():   {},
	attributes.APMConventionKeys.SpanType():       {},
	attributes.APMConventionKeys.SpanKind():       {},
	attributes.APMConventionKeys.HTTPStatusCode(): {},
	attributes.APMConventionKeys.DBName():         {},
}

func mapSpanOTLPKeyToAPMConventionKey(k string) (string, bool) {
	// 1) Check if key is in DD namespace
	if mappedKey, ok := ddNamespaceToAPMConventionKeysForSpanAttributes[k]; ok {
		return mappedKey, true
	}
	// 2) Check if key is in APM conventions as-is
	if _, ok := reservedAPMConventionKeysForSpanAttributes[k]; ok {
		return k, true
	}

	// 3) Check if key has an HTTP mapping
	if mappedKey, found := attributes.HTTPKeyMappings[k]; found {
		return mappedKey, true
	}
	if suffix, ok := strings.CutPrefix(k, "http.request.header."); ok {
		return "http.request.headers." + suffix, true
	}

	// 4) Check if there is a defined mapping from OTel attribute key -> DD tag key
	if mappedKey, ok := otlpToAPMConventionKeysMapForSpanAttributes[k]; ok {
		return mappedKey, true
	}

	// 5) No mapping found, return the original key
	return k, false
}

var otlpToAPMConventionKeysMapForResourceAttributes = map[string]string{
	// service.name
	string(semconv127.ServiceNameKey): attributes.APMConventionKeys.Service(),
	// version
	string(semconv127.ServiceVersionKey): attributes.APMConventionKeys.Version(),
	// env
	string(semconv127.DeploymentEnvironmentNameKey): attributes.APMConventionKeys.Env(),
	string(semconv117.DeploymentEnvironmentKey):     attributes.APMConventionKeys.Env(),
}

var ddNamespaceToAPMConventionKeysForResourceAttributes = map[string]string{
	attributes.DDNamespaceKeys.Service(): attributes.APMConventionKeys.Service(),
	attributes.DDNamespaceKeys.Version(): attributes.APMConventionKeys.Version(),
	attributes.DDNamespaceKeys.Env():     attributes.APMConventionKeys.Env(),
}

var reservedAPMConventionKeysForResourceAttributes = map[string]struct{}{
	attributes.APMConventionKeys.Service(): {},
	attributes.APMConventionKeys.Version(): {},
	attributes.APMConventionKeys.Env():     {},
}

func mapResourceOTLPKeyToAPMConventionKey(k string) (string, bool) {
	// 1) Check if key is in DD namespace
	if mappedKey, ok := ddNamespaceToAPMConventionKeysForResourceAttributes[k]; ok {
		return mappedKey, true
	}
	// 2) Check if key is in APM conventions as-is
	if _, ok := reservedAPMConventionKeysForResourceAttributes[k]; ok {
		return k, true
	}
	// 3) Check if there is a defined mapping from OTel attribute key -> DD tag key
	if mappedKey, ok := otlpToAPMConventionKeysMapForResourceAttributes[k]; ok {
		return mappedKey, true
	}
	// 4) No mapping found, return the original key
	return k, false
}

func computeAPMKeyForOTLPAttribute(
	key string,
	expectedDDNamespaceKeysForThisLocation map[string]string,
	reservedAPMConventionKeysForThisLocation map[string]struct{},
	otlpToAPMKeyFunc func(string) (string, bool),
	wrongPlaceMapFunc func(string) (string, bool),
	wrongPlacePrefix string,
	ignoreMissingDatadogFields bool,
) (string, bool) {
	// If the key is reserved/explicitly checked for elsewhere in translation, skip it.
	if _, ok := expectedDDNamespaceKeysForThisLocation[key]; ok {
		return "", true
	}
	if _, ok := reservedAPMConventionKeysForThisLocation[key]; ok {
		return "", true
	}

	// 1) Try to map the key from OTel -> DD semantics.
	mappedKey, ok := otlpToAPMKeyFunc(key)
	if ignoreMissingDatadogFields {
		// If this attribute would be used to set a known APM tag, and ignoreMissingDatadogFields is set, skip it.
		// (Only allow known APM tags to be set directly via the `datadog.*` namespace.)
		if _, ok := reservedAPMConventionKeysForThisLocation[mappedKey]; ok {
			// Only allocate wrongPlaceKey when we need to return it
			return wrongPlacePrefix + key, false
		}
	}

	if !ok {
		// 2) Check if key is known but in the wrong place
		// If so, make this explicit with prefix
		if _, ok := wrongPlaceMapFunc(key); ok {
			// Only allocate wrongPlaceKey when we need to return it
			return wrongPlacePrefix + key, false
		}
		// 3) If key has no assigned place, no problem, add as-is
		mappedKey = key
		return mappedKey, true
	}
	return mappedKey, true
}

func setValueInMapByTypeAndPrefixIfAlreadyPopulated(k string, value pcommon.Value, metaMap map[string]string, metricMap map[string]float64, wrongPlacePrefix string) {
	switch value.Type() {
	case pcommon.ValueTypeDouble:
		if _, ok := metricMap[k]; ok {
			k = wrongPlacePrefix + k
		}
		setMetricAndRemapIfNeeded(k, value.Double(), metricMap, wrongPlacePrefix)
	case pcommon.ValueTypeInt:
		if _, ok := metricMap[k]; ok {
			k = wrongPlacePrefix + k
		}
		setMetricAndRemapIfNeeded(k, float64(value.Int()), metricMap, wrongPlacePrefix)
	default:
		if _, ok := metaMap[k]; ok {
			k = wrongPlacePrefix + k
		}
		metaMap[k] = value.AsString()
	}
}

// Returns "false" if the key is in the wrong place.
func conditionallyConsumeOTLPAttribute(
	key string,
	value pcommon.Value,
	outputMetaMap map[string]string,
	outputMetricMap map[string]float64,
	reservedAPMConventionKeysForThisLocation map[string]struct{},
	expectedDDNamespaceKeysForThisLocation map[string]string,
	otlpToAPMKeyFunc func(string) (string, bool),
	wrongPlaceMapFunc func(string) (string, bool),
	wrongPlacePrefix string,
	ignoreMissingDatadogFields bool,
) bool {
	mappedKey, keyInRightPlace := computeAPMKeyForOTLPAttribute(key, expectedDDNamespaceKeysForThisLocation, reservedAPMConventionKeysForThisLocation, otlpToAPMKeyFunc, wrongPlaceMapFunc, wrongPlacePrefix, ignoreMissingDatadogFields)

	if mappedKey == "" {
		return keyInRightPlace
	}

	// If `mappedKey` was already set, e.g. via `datadog.mappedKey`, don't overwrite it.
	if _, ok := outputMetaMap[mappedKey]; !ok {
		setValueInMapByTypeAndPrefixIfAlreadyPopulated(mappedKey, value, outputMetaMap, outputMetricMap, wrongPlacePrefix)
		if key == mappedKey {
			return keyInRightPlace
		}
	}

	if keyInRightPlace {
		// Always preserve the original OTLP key
		setValueInMapByTypeAndPrefixIfAlreadyPopulated(key, value, outputMetaMap, outputMetricMap, wrongPlacePrefix)
	}

	return keyInRightPlace
}

// OtelSpanToDDSpan converts an OTel span to a DD span.
func OtelSpanToDDSpan(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	conf *config.AgentConfig,
) (ddSpan *pb.Span, wrongPlaceKeysCount int) {
	if OTelCompliantTranslationEnabled(conf) {
		return otelSpanToDDSpanOTelCompliantTranslation(otelspan, otelres, lib, conf)
	}
	return otelSpanToDDSpanOld(otelspan, otelres, lib, conf)
}

// GetDDKeyForOTLPAttribute looks for a key in the Datadog HTTP convention that matches the given key from the
// OTLP HTTP convention. Otherwise, check if it is a Datadog APM convention key - if it is, it will be handled with
// specialized logic elsewhere, so return an empty string. If it isn't, return the original key.
func GetDDKeyForOTLPAttribute(k string) string {
	mappedKey, found := attributes.HTTPKeyMappings[k]
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

func conditionallyMapOTLPAttributeToMeta(k string, value string, ddspan *pb.Span, ignoreMissingDatadogFields bool) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	if ddspan.Meta[mappedKey] != "" {
		return
	}

	// Exclude Datadog APM conventions.
	// These are handled above explicitly.
	if mappedKey != "" {
		// Does it have an equivalent DD namespaced key? If so, and ignoreMissingDatadogFields is set, we don't want to set it.
		if _, ok := apmConventionKeysToDDNamespacedKeys[mappedKey]; ok && ignoreMissingDatadogFields {
			return
		}
		SetMetaOTLPIfEmpty(ddspan, mappedKey, value)
	}
}

func conditionallyMapOTLPAttributeToMetric(k string, value float64, ddspan *pb.Span, ignoreMissingDatadogFields bool) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	if _, ok := ddspan.Metrics[mappedKey]; ok {
		return
	}

	// Exclude Datadog APM conventions.
	// These are handled above explicitly.
	if mappedKey != "" {
		// Does it have an equivalent DD namespaced key? If so, and ignoreMissingDatadogFields is set, we don't want to set it.
		if _, ok := apmConventionKeysToDDNamespacedKeys[mappedKey]; ok && ignoreMissingDatadogFields {
			return
		}
		SetMetricOTLPIfEmpty(ddspan, mappedKey, value)
	}
}

// If these DD namespaced keys are found in OTLP attributes, map them to the corresponding keys in ddspan.Meta
var ddNamespacedKeysToAPMConventionKeys = map[string]string{
	attributes.DDNamespaceKeys.Env():            attributes.APMConventionKeys.Env(),
	attributes.DDNamespaceKeys.Version():        attributes.APMConventionKeys.Version(),
	attributes.DDNamespaceKeys.HTTPStatusCode(): "http.status_code",
	"datadog.error.msg":                         "error.msg",
	"datadog.error.type":                        "error.type",
	"datadog.error.stack":                       "error.stack",
}

var apmConventionKeysToDDNamespacedKeys = map[string]string{
	attributes.APMConventionKeys.Env():            attributes.DDNamespaceKeys.Env(),
	attributes.APMConventionKeys.Version():        attributes.DDNamespaceKeys.Version(),
	attributes.APMConventionKeys.HTTPStatusCode(): "http.status_code",
	"error.msg":   "datadog.error.msg",
	"error.type":  "datadog.error.type",
	"error.stack": "datadog.error.stack",
}

func copyAttrToMapIfExists(attributes pcommon.Map, key string, m map[string]string, mappedKey string) {
	if incomingValue := traceutil.GetOTelAttrVal(attributes, false, key); incomingValue != "" {
		m[mappedKey] = incomingValue
	}
}

// GetOTelEnv returns the environment based on OTel span and resource attributes, with span taking precedence.
// Deprecated: use attributes.GetEnv instead.
func GetOTelEnv(span ptrace.Span, res pcommon.Resource, ignoreMissingDatadogFields bool) string {
	env := traceutil.GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, attributes.DDNamespaceKeys.Env())
	if env == "" && !ignoreMissingDatadogFields {
		env = traceutil.GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, string(semconv127.DeploymentEnvironmentNameKey), string(semconv.DeploymentEnvironmentKey))
	}
	return env
}

// GetOTelHostname returns the DD hostname based on OTel span and resource attributes, with span taking precedence.
// Deprecated: use attributes.GetHostname instead.
func GetOTelHostname(span ptrace.Span, res pcommon.Resource, tr *attributes.Translator, fallbackHost string, ignoreMissingDatadogFields bool) string {
	hostname := traceutil.GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, attributes.DDNamespaceKeys.Host())
	if hostname == "" && !ignoreMissingDatadogFields {
		ctx := context.Background()
		src, srcok := tr.ResourceToSource(ctx, res, traceutil.SignalTypeSet, nil)
		if !srcok {
			if v := traceutil.GetOTelAttrValInResAndSpanAttrs(span, res, false, "_dd.hostname"); v != "" {
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
		} else {
			// fallback hostname from Agent conf.Hostname
			return fallbackHost
		}
	}
	return hostname
}

// GetOTelVersion returns the version based on OTel span and resource attributes, with span taking precedence.
// Deprecated: use attributes.GetVersion instead.
func GetOTelVersion(span ptrace.Span, res pcommon.Resource, ignoreMissingDatadogFields bool) string {
	version := traceutil.GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, attributes.DDNamespaceKeys.Version())
	if version == "" && !ignoreMissingDatadogFields {
		version = traceutil.GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, string(semconv.ServiceVersionKey))
	}
	return version
}

// GetOTelContainerID returns the container ID based on OTel span and resource attributes, with span taking precedence.
// Deprecated: use attributes.GetContainerID instead.
func GetOTelContainerID(span ptrace.Span, res pcommon.Resource, ignoreMissingDatadogFields bool) string {
	cid := traceutil.GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, attributes.DDNamespaceKeys.ContainerID())
	if cid == "" && !ignoreMissingDatadogFields {
		cid = traceutil.GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, string(semconv.ContainerIDKey), string(semconv.K8SPodUIDKey))
	}
	return cid
}

// GetOTelStatusCode returns the HTTP status code based on OTel span and resource attributes, with span taking precedence.
// Deprecated: use attributes.GetStatusCode instead.
func GetOTelStatusCode(span ptrace.Span, res pcommon.Resource, ignoreMissingDatadogFields bool) uint32 {
	sattr := span.Attributes()
	rattr := res.Attributes()
	if incomingCode, ok := sattr.Get(attributes.DDNamespaceKeys.HTTPStatusCode()); ok {
		return uint32(incomingCode.Int())
	} else if incomingCode, ok := rattr.Get(attributes.DDNamespaceKeys.HTTPStatusCode()); ok {
		return uint32(incomingCode.Int())
	} else if !ignoreMissingDatadogFields {
		if code, ok := sattr.Get(string(semconv.HTTPStatusCodeKey)); ok {
			return uint32(code.Int())
		}
		if code, ok := sattr.Get("http.response.status_code"); ok {
			return uint32(code.Int())
		}
		if code, ok := rattr.Get(string(semconv.HTTPStatusCodeKey)); ok {
			return uint32(code.Int())
		}
		if code, ok := rattr.Get("http.response.status_code"); ok {
			return uint32(code.Int())
		}
		return 0
	}
	return 0
}

// GetOTelContainerTags returns a list of DD container tags in the OTel resource attributes.
// Tags are always normalized.
// Deprecated: use attributes.GetContainerTags instead.
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
			if val := traceutil.GetOTelAttrVal(rattrs, false, key); val != "" {
				t := normalizeutil.NormalizeTag(key + ":" + val)
				containerTags = append(containerTags, t)
			}
		}
	}
	return containerTags
}

func otelSpanToDDSpanOld(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	conf *config.AgentConfig,
) (ddspan *pb.Span, wrongPlaceKeysCount int) {
	spanKind := otelspan.Kind()
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	isTopLevel := false
	if topLevelByKind {
		isTopLevel = otelspan.ParentSpanID() == pcommon.NewSpanIDEmpty() || spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer
	}
	ddspan = otelSpanToDDSpanMinimalOld(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, nil)

	// 1) DD namespaced keys take precedence over OTLP keys, so use them first
	// 2) Span attributes take precedence over resource attributes in the event of key collisions; so, use span attributes first

	for ddNamespacedKey, apmConventionKey := range ddNamespacedKeysToAPMConventionKeys {
		copyAttrToMapIfExists(otelspan.Attributes(), ddNamespacedKey, ddspan.Meta, apmConventionKey)
		copyAttrToMapIfExists(otelres.Attributes(), ddNamespacedKey, ddspan.Meta, apmConventionKey)
	}

	otelspan.Attributes().Range(func(k string, v pcommon.Value) bool {
		_, keyInRightPlace := computeAPMKeyForOTLPAttribute(k, ddNamespaceToAPMConventionKeysForSpanAttributes, reservedAPMConventionKeysForSpanAttributes, mapSpanOTLPKeyToAPMConventionKey, mapResourceOTLPKeyToAPMConventionKey, "otel.span.", conf.OTLPReceiver.IgnoreMissingDatadogFields)
		if !keyInRightPlace {
			wrongPlaceKeysCount++
		}

		switch v.Type() {
		case pcommon.ValueTypeDouble:
			conditionallyMapOTLPAttributeToMetric(k, v.Double(), ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		case pcommon.ValueTypeInt:
			conditionallyMapOTLPAttributeToMetric(k, float64(v.Int()), ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		default:
			conditionallyMapOTLPAttributeToMeta(k, v.AsString(), ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		}

		return true
	})

	traceID := otelspan.TraceID()
	ddspan.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])
	if !spanMetaHasKey(ddspan, "version") {
		if version := GetOTelVersion(otelspan, otelres, conf.OTLPReceiver.IgnoreMissingDatadogFields); version != "" {
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

	if !conf.OTLPReceiver.IgnoreMissingDatadogFields {
		if !spanMetaHasKey(ddspan, "error.msg") || !spanMetaHasKey(ddspan, "error.type") || !spanMetaHasKey(ddspan, "error.stack") {
			ddspan.Error = Status2Error(otelspan.Status(), otelspan.Events(), ddspan.Meta)
		}

		if !spanMetaHasKey(ddspan, "env") {
			if env := GetOTelEnv(otelspan, otelres, conf.OTLPReceiver.IgnoreMissingDatadogFields); env != "" {
				ddspan.Meta["env"] = env
			}
		}
	}

	otelres.Attributes().Range(func(k string, v pcommon.Value) bool {
		_, keyInRightPlace := computeAPMKeyForOTLPAttribute(k, ddNamespaceToAPMConventionKeysForResourceAttributes, reservedAPMConventionKeysForResourceAttributes, mapResourceOTLPKeyToAPMConventionKey, mapSpanOTLPKeyToAPMConventionKey, "otel.resource.", conf.OTLPReceiver.IgnoreMissingDatadogFields)
		if !keyInRightPlace {
			wrongPlaceKeysCount++
		}

		value := v.AsString()
		conditionallyMapOTLPAttributeToMeta(k, value, ddspan, conf.OTLPReceiver.IgnoreMissingDatadogFields)
		return true
	})

	for k, v := range lib.Attributes().Range {
		ddspan.Meta[k] = v.AsString()
	}

	// Check for db.namespace and conditionally set db.name
	if _, ok := ddspan.Meta["db.name"]; !ok {
		if dbNamespace := traceutil.GetOTelAttrValInResAndSpanAttrs(otelspan, otelres, false, string(semconv127.DBNamespaceKey)); dbNamespace != "" {
			ddspan.Meta["db.name"] = dbNamespace
		}
	}

	return ddspan, wrongPlaceKeysCount
}

func otelSpanToDDSpanOTelCompliantTranslation(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	conf *config.AgentConfig,
) (*pb.Span, int) {
	wrongPlaceKeysCount := 0
	sattr := otelspan.Attributes()
	rattr := otelres.Attributes()

	spanKind := otelspan.Kind()
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	isTopLevel := false
	if topLevelByKind {
		isTopLevel = otelspan.ParentSpanID() == pcommon.NewSpanIDEmpty() || spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer
	}
	ddspan := otelSpanToDDSpanMinimalOTelCompliantTranslation(otelspan, otelres, isTopLevel, topLevelByKind, conf, nil)

	httpStatusCode := ddspan.Metrics[traceutil.TagStatusCode]
	isError, errMsg, errType, errStack := GetErrorFieldsFromStatusAndEventsAndHTTPCode(otelspan.Status(), otelspan.Events(), int(httpStatusCode))
	ddspan.Error = isError
	// Set these error fields directly to meta - permit overwriting them if they are explicitly specified in attributes
	if errMsg != "" {
		ddspan.Meta["error.msg"] = errMsg
	}
	if errType != "" {
		ddspan.Meta["error.type"] = errType
	}
	if errStack != "" {
		ddspan.Meta["error.stack"] = errStack
	}
	if spanKind, ok := ddspan.Meta[attributes.APMConventionKeys.SpanKind()]; ok {
		ddspan.Meta[attributes.APMConventionKeys.SpanKind()] = spanKind
	}

	// 1) Compute fields that require explicit precedence/more complex than 1:1 conversion
	if version := attributes.GetVersion(rattr, conf.OTLPReceiver.IgnoreMissingDatadogFields, true, nil); version != "" {
		ddspan.Meta[attributes.APMConventionKeys.Version()] = version
	}
	if env := attributes.GetEnv(rattr, conf.OTLPReceiver.IgnoreMissingDatadogFields, true, &emptyString); env != "" {
		ddspan.Meta[attributes.APMConventionKeys.Env()] = env
	}

	if dbName := traceutil.GetOTelAttrVal(sattr, false, attributes.DDNamespaceKeys.DBName(), attributes.APMConventionKeys.DBName(), string(semconv127.DBNamespaceKey)); dbName != "" {
		ddspan.Meta[attributes.APMConventionKeys.DBName()] = dbName
	}

	// 2) Resolve OTel semantics keys
	sattr.Range(func(k string, v pcommon.Value) bool {
		if k == "analytics.event" {
			if v, err := strconv.ParseBool(v.AsString()); err == nil {
				if _, ok := ddspan.Metrics[sampler.KeySamplingRateEventExtraction]; !ok {
					if v {
						ddspan.Metrics[sampler.KeySamplingRateEventExtraction] = 1
					} else {
						ddspan.Metrics[sampler.KeySamplingRateEventExtraction] = 0
					}
				}
			}
			return true
		}
		keyInRightPlace := conditionallyConsumeOTLPAttribute(k, v, ddspan.Meta, ddspan.Metrics, reservedAPMConventionKeysForSpanAttributes, ddNamespaceToAPMConventionKeysForSpanAttributes, mapSpanOTLPKeyToAPMConventionKey, mapResourceOTLPKeyToAPMConventionKey, "otel.span.", conf.OTLPReceiver.IgnoreMissingDatadogFields)
		if !keyInRightPlace {
			wrongPlaceKeysCount++
		}
		return true
	})
	rattr.Range(func(k string, v pcommon.Value) bool {
		keyInRightPlace := conditionallyConsumeOTLPAttribute(k, v, ddspan.Meta, ddspan.Metrics, reservedAPMConventionKeysForResourceAttributes, ddNamespaceToAPMConventionKeysForResourceAttributes, mapResourceOTLPKeyToAPMConventionKey, mapSpanOTLPKeyToAPMConventionKey, "otel.resource.", conf.OTLPReceiver.IgnoreMissingDatadogFields)
		if !keyInRightPlace {
			wrongPlaceKeysCount++
		}
		return true
	})

	// 3) Merge the maps into the span
	for k, v := range lib.Attributes().Range {
		setValueInMapByTypeAndPrefixIfAlreadyPopulated(k, v, ddspan.Meta, ddspan.Metrics, "otel.scope.")
	}

	// 4) Use OTLP fields other than attributes
	traceID := otelspan.TraceID()
	ddspan.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])

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
		ddspan.Meta[string(semconv117.OtelLibraryNameKey)] = lib.Name()
	}
	if lib.Version() != "" {
		ddspan.Meta[string(semconv117.OtelLibraryVersionKey)] = lib.Version()
	}
	ddspan.Meta[string(semconv117.OtelStatusCodeKey)] = otelspan.Status().Code().String()
	if msg := otelspan.Status().Message(); msg != "" {
		ddspan.Meta[string(semconv117.OtelStatusDescriptionKey)] = msg
	}

	return ddspan, wrongPlaceKeysCount
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

func setMetricAndRemapIfNeeded(k string, v float64, metricMap map[string]float64, wrongPlacePrefix string) {
	outKey := k
	if _, ok := metricMap[k]; ok {
		outKey = wrongPlacePrefix + outKey
	}
	switch outKey {
	case "sampling.priority":
		metricMap["_sampling_priority_v1"] = v
	default:
		metricMap[outKey] = v
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

// GetErrorFieldsFromStatusAndEventsAndHTTPCode checks the given status and events and applies any potential error and messages
// to the given span attributes.
func GetErrorFieldsFromStatusAndEventsAndHTTPCode(status ptrace.Status, events ptrace.SpanEventSlice, httpcode int) (int32, string, string, string) {
	errMsg := ""
	errType := ""
	errStack := ""
	if status.Code() != ptrace.StatusCodeError {
		return 0, "", "", ""
	}
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if strings.ToLower(e.Name()) != "exception" {
			continue
		}
		attrs := e.Attributes()
		if v, ok := attrs.Get(string(semconv117.ExceptionMessageKey)); ok {
			errMsg = v.AsString()
		}
		if v, ok := attrs.Get(string(semconv117.ExceptionTypeKey)); ok {
			errType = v.AsString()
		}
		if v, ok := attrs.Get(string(semconv117.ExceptionStacktraceKey)); ok {
			errStack = v.AsString()
		}
	}
	if errMsg == "" {
		// no error message was extracted, find alternatives
		if status.Message() != "" {
			// use the status message
			errMsg = status.Message()
		} else if httpcode != 0 {
			errMsg = http.StatusText(httpcode)
		}
	}
	return 1, errMsg, errType, errStack
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
		if v, ok := attrs.Get(string(semconv117.ExceptionMessageKey)); ok {
			metaMap["error.msg"] = v.AsString()
		}
		if v, ok := attrs.Get(string(semconv117.ExceptionTypeKey)); ok {
			metaMap["error.type"] = v.AsString()
		}
		if v, ok := attrs.Get(string(semconv117.ExceptionStacktraceKey)); ok {
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
