package translator

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv "go.opentelemetry.io/collector/semconv/v1.5.0"
)

var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func GetBuffer() *bytes.Buffer {
	buffer := bufferPool.Get().(*bytes.Buffer)
	buffer.Reset()
	return buffer
}

func PutBuffer(buffer *bytes.Buffer) {
	bufferPool.Put(buffer)
}

type RUMPayload struct {
	Type string
}

func parseW3CTraceContext(traceparent string) (traceID pcommon.TraceID, spanID pcommon.SpanID, err error) {
	// W3C traceparent format: version-traceID-spanID-flags
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		return pcommon.NewTraceIDEmpty(), pcommon.NewSpanIDEmpty(), fmt.Errorf("invalid traceparent format: %s", traceparent)
	}

	// Parse trace ID (32 hex characters)
	traceIDBytes, err := hex.DecodeString(parts[1])
	if err != nil || len(traceIDBytes) != 16 {
		return pcommon.NewTraceIDEmpty(), pcommon.SpanID{}, fmt.Errorf("invalid trace ID: %s", parts[1])
	}
	copy(traceID[:], traceIDBytes)

	// Parse span ID
	spanIDBytes, err := hex.DecodeString(parts[2])
	if err != nil || len(spanIDBytes) != 8 {
		return pcommon.NewTraceIDEmpty(), pcommon.NewSpanIDEmpty(), fmt.Errorf("invalid parent ID: %s", parts[2])
	}
	copy(spanID[:], spanIDBytes)

	return traceID, spanID, nil
}

func parseIDs(payload map[string]any, req *http.Request) (pcommon.TraceID, pcommon.SpanID, error) {
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

func parseRUMRequestIntoResource(resource pcommon.Resource, payload map[string]any, req *http.Request, rawRequestBody []byte) {
	resource.Attributes().PutStr(semconv.AttributeServiceName, "browser-rum-sdk")

	prettyPayload, _ := json.MarshalIndent(payload, "", "\t")
	resource.Attributes().PutStr("pretty_payload", string(prettyPayload))

	// Store URL query parameters as attributes
	queryAttrs := resource.Attributes().PutEmptyMap("request_query")
	for paramName, paramValues := range req.URL.Query() {
		paramValueList := queryAttrs.PutEmptySlice(paramName)
		for _, paramValue := range paramValues {
			paramValueList.AppendEmpty().SetStr(paramValue)
		}
	}

	resource.Attributes().PutStr("request_ddforward", req.URL.Query().Get("ddforward"))
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

type AttributeType string

const (
	StringAttribute        AttributeType = "str"
	BoolAttribute          AttributeType = "bool"
	NumberAttribute        AttributeType = "num"
	IntegerAttribute       AttributeType = "int"
	ObjectAttribute        AttributeType = "obj"
	ArrayAttribute         AttributeType = "arr"
	StringOrArrayAttribute AttributeType = "str_or_arr"
)

type AttributeMeta struct {
	OTLPName string
	Type     AttributeType
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

func setAttributes(flatPayload map[string]any, attributes pcommon.Map) {
	for key, val := range flatPayload {
		meta, exists := attributeMetaMap[key]

		rumKey := ""
		typ := StringAttribute
		if exists {
			rumKey = meta.OTLPName
			typ = meta.Type
		} else {
			rumKey = "datadog" + "." + key
			typ = StringAttribute
		}

		switch typ {
		case StringAttribute:
			if s, ok := val.(string); ok {
				attributes.PutStr(rumKey, s)
			}
		case BoolAttribute:
			if b, ok := val.(bool); ok {
				attributes.PutBool(rumKey, b)
			}
		case NumberAttribute:
			if f, ok := val.(float64); ok {
				attributes.PutDouble(rumKey, f)
			}
		case IntegerAttribute:
			if i, ok := val.(int64); ok {
				attributes.PutInt(rumKey, i)
			} else if f, ok := val.(float64); ok {
				i := int64(f)
				attributes.PutInt(rumKey, i)
			}
		case ObjectAttribute:
			if o, ok := val.(map[string]any); ok {
				objVal := attributes.PutEmptyMap(rumKey)
				for k, v := range o {
					objVal.PutStr(k, fmt.Sprintf("%v", v))
				}
			}
		case ArrayAttribute:
			if a, ok := val.([]any); ok {
				arrVal := attributes.PutEmptySlice(rumKey)
				for _, v := range a {
					arrVal.AppendEmpty().SetStr(fmt.Sprintf("%v", v))
				}
			}
		case StringOrArrayAttribute:
			if s, ok := val.(string); ok {
				attributes.PutStr(rumKey, s)
			} else if a, ok := val.([]any); ok {
				arrVal := attributes.PutEmptySlice(rumKey)
				for _, v := range a {
					arrVal.AppendEmpty().SetStr(fmt.Sprintf("%v", v))
				}
			}
		}
	}
}
