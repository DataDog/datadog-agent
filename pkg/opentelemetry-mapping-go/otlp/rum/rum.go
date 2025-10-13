// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func buildRumPayload(k string, v pcommon.Value, rumPayload map[string]any) {
	parts := strings.Split(k, ".")
	current := rumPayload

	for i, part := range parts {
		if i != len(parts)-1 {
			existing, ok := current[part]
			switch {
			case !ok:
				current[part] = make(map[string]any)
			default:
				if _, isMap := existing.(map[string]any); !isMap {
					// force override if it's not a map
					current[part] = make(map[string]any)
				}
			}
			current = current[part].(map[string]any)
			continue
		}

		switch v.Type() {
		case pcommon.ValueTypeSlice:
			current[part] = v.Slice().AsRaw()
		case pcommon.ValueTypeMap:
			if v.Map().Len() == 0 {
				current[part] = nil
				return
			}
			processedMap := make(map[string]any)
			v.Map().Range(func(mapKey string, mapValue pcommon.Value) bool {
				buildRumPayload(mapKey, mapValue, processedMap)
				return true
			})
			current[part] = processedMap
		case pcommon.ValueTypeBytes:
			if v.Bytes().Len() == 0 {
				current[part] = nil
				return
			}
			current[part] = v.AsRaw()
		default:
			current[part] = v.AsRaw()
		}
	}
}

// ConstructRumPayloadFromOTLP constructs a RUM payload from OTLP attributes.
func ConstructRumPayloadFromOTLP(attr pcommon.Map) map[string]any {
	rumPayload := make(map[string]any)
	attr.Range(func(k string, v pcommon.Value) bool {
		if rumAttributeName, exists := otlpAttributeToRUMPayloadKeyMapping[k]; exists {
			buildRumPayload(rumAttributeName, v, rumPayload)
			return true
		}

		trimmedKey := strings.TrimPrefix(k, "datadog.")
		buildRumPayload(trimmedKey, v, rumPayload)
		return true
	})
	return rumPayload
}

func parseIDs(payload map[string]any) (pcommon.TraceID, pcommon.SpanID, error) {
	ddMetadata, ok := payload["_dd"].(map[string]any)
	if !ok {
		return pcommon.NewTraceIDEmpty(), pcommon.NewSpanIDEmpty(), fmt.Errorf("failed to find _dd metadata in payload")
	}

	traceIDString, ok := ddMetadata["trace_id"].(string)
	if !ok {
		return pcommon.NewTraceIDEmpty(), pcommon.NewSpanIDEmpty(), fmt.Errorf("failed to retrieve traceID from payload")
	}
	traceID, err := strconv.ParseUint(traceIDString, 10, 64)
	if err != nil {
		return pcommon.NewTraceIDEmpty(), pcommon.NewSpanIDEmpty(), fmt.Errorf("failed to parse traceID: %w", err)
	}

	spanIDString, ok := ddMetadata["span_id"].(string)
	if !ok {
		return pcommon.NewTraceIDEmpty(), pcommon.NewSpanIDEmpty(), fmt.Errorf("failed to retrieve spanID from payload")
	}
	spanID, err := strconv.ParseUint(spanIDString, 10, 64)
	if err != nil {
		return pcommon.NewTraceIDEmpty(), pcommon.NewSpanIDEmpty(), fmt.Errorf("failed to parse spanID: %w", err)
	}

	return uInt64ToTraceID(0, traceID), uInt64ToSpanID(spanID), nil
}

func parseDDForwardIntoResource(attributes pcommon.Map, ddforward string) {
	u, err := url.Parse(ddforward)
	if err != nil {
		return
	}

	queryParams := u.Query()
	batchTime := queryParams.Get("batch_time")
	if batchTime != "" {
		attributes.PutStr("batch_time", batchTime)
	}

	ddTags := queryParams.Get("ddtags")
	if ddTags != "" {
		ddTagsMap := attributes.PutEmptyMap("ddtags")
		for _, tag := range strings.Split(ddTags, ",") {
			parts := strings.SplitN(tag, ":", 2)
			if len(parts) == 2 {
				ddTagsMap.PutStr(parts[0], parts[1])
			}
		}
	}

	ddSource := queryParams.Get("ddsource")
	if ddSource != "" {
		attributes.PutStr("ddsource", ddSource)
	}

	ddEvpOrigin := queryParams.Get("dd-evp-origin")
	if ddEvpOrigin != "" {
		attributes.PutStr("dd-evp-origin", ddEvpOrigin)
	}

	ddRequestID := queryParams.Get("dd-request-id")
	if ddRequestID != "" {
		attributes.PutStr("dd-request-id", ddRequestID)
	}

	ddAPIKey := queryParams.Get("dd-api-key")
	if ddAPIKey != "" {
		attributes.PutStr("dd-api-key", ddAPIKey)
	}
}

func uInt64ToTraceID(high, low uint64) pcommon.TraceID {
	traceID := [16]byte{}
	binary.BigEndian.PutUint64(traceID[0:8], high)
	binary.BigEndian.PutUint64(traceID[8:16], low)
	return pcommon.TraceID(traceID)
}

func uInt64ToSpanID(id uint64) pcommon.SpanID {
	spanID := [8]byte{}
	binary.BigEndian.PutUint64(spanID[:], id)
	return pcommon.SpanID(spanID)
}

func flattenJSON(payload map[string]any) map[string]any {
	flat := make(map[string]any)
	var recurse func(map[string]any, string)
	recurse = func(m map[string]any, prefix string) {
		for k, v := range m {
			fullKey := k
			if prefix != "" {
				fullKey = prefix + "." + k
			}
			if nested, ok := v.(map[string]any); ok {
				recurse(nested, fullKey)
			} else {
				flat[fullKey] = v
			}
		}
	}
	recurse(payload, "")
	return flat
}

func setOTLPAttributes(flatPayload map[string]any, attributes pcommon.Map) {
	for key, val := range flatPayload {
		rumKey, exists := rumPayloadKeyToOTLPAttributeMapping[key]

		if !exists {
			rumKey = "datadog" + "." + key
		}

		switch v := val.(type) {
		case string:
			attributes.PutStr(rumKey, v)
		case bool:
			attributes.PutBool(rumKey, v)
		case float64:
			attributes.PutDouble(rumKey, v)
		case map[string]any:
			objVal := attributes.PutEmptyMap(rumKey)
			setOTLPAttributes(v, objVal)
		case []any:
			arrVal := attributes.PutEmptySlice(rumKey)
			appendToOTLPSlice(arrVal, v)
		default:
			attributes.PutStr(rumKey, fmt.Sprintf("%v", v))
		}
	}
}

func appendToOTLPSlice(slice pcommon.Slice, val any) {
	switch v := val.(type) {
	case string:
		slice.AppendEmpty().SetStr(v)
	case bool:
		slice.AppendEmpty().SetBool(v)
	case float64:
		slice.AppendEmpty().SetDouble(v)
	case map[string]any:
		elemMap := slice.AppendEmpty().SetEmptyMap()
		setOTLPAttributes(v, elemMap)
	case []any:
		subSlice := slice.AppendEmpty().SetEmptySlice()
		for _, inner := range v {
			appendToOTLPSlice(subSlice, inner)
		}
	default:
		slice.AppendEmpty().SetStr(fmt.Sprintf("%v", val))
	}
}

type paramValue struct {
	ParamKey string
	SpanAttr string
	Fallback string
}

func getParamValue(rattrs pcommon.Map, lattrs pcommon.Map, param paramValue) string {
	if param.SpanAttr != "" {
		parts := strings.Split(param.SpanAttr, ".")
		m := lattrs
		for i, part := range parts {
			if v, ok := m.Get(part); ok {
				if i == len(parts)-1 {
					return v.AsString()
				}
				if v.Type() == pcommon.ValueTypeMap {
					m = v.Map()
				}
			}
		}
	}
	if v, ok := rattrs.Get(param.ParamKey); ok {
		return v.AsString()
	}
	return param.Fallback
}

func buildDDTags(rattrs pcommon.Map, lattrs pcommon.Map) string {
	requiredTags := []paramValue{
		{ParamKey: "service", SpanAttr: "service.name", Fallback: "otlpresourcenoservicename"},
		{ParamKey: "version", SpanAttr: "service.version", Fallback: ""},
		{ParamKey: "sdk_version", SpanAttr: "_dd.sdk_version", Fallback: ""},
		{ParamKey: "env", Fallback: "default"},
	}

	tagMap := make(map[string]string)

	if v, ok := rattrs.Get("ddtags"); ok && v.Type() == pcommon.ValueTypeMap {
		v.Map().Range(func(k string, val pcommon.Value) bool {
			tagMap[k] = val.AsString()
			return true
		})
	}

	for _, tag := range requiredTags {
		val := getParamValue(rattrs, lattrs, tag)
		if val != tag.Fallback {
			tagMap[tag.ParamKey] = val
		}
	}

	var tagParts []string
	for k, v := range tagMap {
		tagParts = append(tagParts, k+":"+v)
	}

	return strings.Join(tagParts, ",")
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// BuildIntakeURLPathAndParameters builds the intake URL path and parameters to send RUM payloads to Datadog RUM backend.
func BuildIntakeURLPathAndParameters(rattrs pcommon.Map, lattrs pcommon.Map) string {
	var parts []string

	batchTimeParam := paramValue{ParamKey: "batch_time", Fallback: strconv.FormatInt(time.Now().UnixMilli(), 10)}
	parts = append(parts, batchTimeParam.ParamKey+"="+getParamValue(rattrs, lattrs, batchTimeParam))

	parts = append(parts, "ddtags="+buildDDTags(rattrs, lattrs))

	ddsourceParam := paramValue{ParamKey: "ddsource", SpanAttr: "source", Fallback: "browser"}
	parts = append(parts, ddsourceParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddsourceParam))

	ddEvpOriginParam := paramValue{ParamKey: "dd-evp-origin", SpanAttr: "source", Fallback: "browser"}
	parts = append(parts, ddEvpOriginParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddEvpOriginParam))

	ddRequestID, err := randomID()
	if err != nil {
		return ""
	}
	ddRequestIDParam := paramValue{ParamKey: "dd-request-id", SpanAttr: "", Fallback: ddRequestID}
	parts = append(parts, ddRequestIDParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddRequestIDParam))

	ddAPIKeyParam := paramValue{ParamKey: "dd-api-key", SpanAttr: "", Fallback: ""}
	parts = append(parts, ddAPIKeyParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddAPIKeyParam))

	return "/api/v2/rum?" + strings.Join(parts, "&")
}
