package rum

import (
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"

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

func ConstructRumPayloadFromOTLP(attr pcommon.Map) map[string]any {
	rumPayload := make(map[string]any)
	attr.Range(func(k string, v pcommon.Value) bool {
		if rumAttributeName, exists := OTLPAttributeToRUMPayloadKeyMapping[k]; exists {
			buildRumPayload(rumAttributeName, v, rumPayload)
			return true
		}

		trimmedKey := strings.TrimPrefix(k, "datadog.")
		buildRumPayload(trimmedKey, v, rumPayload)
		return true
	})
	return rumPayload
}

type RUMPayload struct {
	Type string
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

	ddRequestId := queryParams.Get("dd-request-id")
	if ddRequestId != "" {
		attributes.PutStr("dd-request-id", ddRequestId)
	}

	ddApiKey := queryParams.Get("dd-api-key")
	if ddApiKey != "" {
		attributes.PutStr("dd-api-key", ddApiKey)
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
		rumKey, exists := RUMPayloadKeyToOTLPAttributeMapping[key]

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
