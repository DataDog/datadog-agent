// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processor provides JSON-aware preprocessing for stateful log encoding.
// It extracts message fields from JSON logs and serializes remaining fields into ordered json_context.
package processor

import (
	"bytes"
	stdjson "encoding/json"
	"sort"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

var jsonAPI = jsoniter.Config{UseNumber: true}.Froze()

// ExtractionResult contains the result of JSON preprocessing
type ExtractionResult struct {
	// IsJSON indicates whether the input was valid JSON
	IsJSON bool
	// Message is the extracted message field (empty if not found or not JSON)
	Message string
	// MessageKey is the JSON key the message was extracted from (e.g. "msg", "message")
	MessageKey string
	// JSONContextSchema is a comma-separated sorted list of escaped JSON paths for the JSON context.
	JSONContextSchema string
	// JSONContextKeys contains the escaped JSON paths corresponding to JSONContextValues, in order.
	JSONContextKeys []string
	// JSONContextValues contains the values corresponding to JSONContextKeys, in order.
	// Primitive values preserve their JSON type. Boxed nested objects/arrays are preserved as
	// decoded map/slice values so the transport layer can encode them as raw JSON.
	JSONContextValues []interface{}
}

const (
	jsonContextMaxFlattenDepth = 2
	jsonContextMaxObjectKeys   = 16
)

var boxedSubtreeNames = map[string]struct{}{
	"attributes": {},
	"attrs":      {},
	"headers":    {},
	"labels":     {},
	"metadata":   {},
	"tags":       {},
}

type jsonContextItem struct {
	key   string
	value interface{}
}

// Common top-level message field names (Layer 0)
// These cover the vast majority of structured logs from popular logging libraries
var topLevelMessageKeys = []string{
	"message",
	"msg",
	"log",
	"text",
}

// Common nested paths (Layer 1 fallback)
// Some applications wrap their log message in a data/event/payload envelope
var nestedMessagePaths = []string{
	"data.message",    // Generic data wrapper
	"event.message",   // Event-based logs
	"payload.message", // Payload wrapper
}

// unmarshalJSON decodes JSON into a map using UseNumber to preserve 64-bit integer precision.
// Without UseNumber, integers larger than 2^53 (e.g. trace IDs, span IDs) silently
// round-trip through float64 and lose precision.
func unmarshalJSON(content []byte, v interface{}) error {
	return jsonAPI.Unmarshal(content, v)
}

// PreprocessJSON attempts to extract a message field from JSON logs and serialize remaining fields.
func PreprocessJSON(content []byte) ExtractionResult {
	fail := ExtractionResult{IsJSON: false}

	// Check if it's a JSON object (handles leading whitespace)
	if !isJSONObject(content) {
		return fail
	}

	// Parse JSON
	var data map[string]interface{}
	if err := unmarshalJSON(content, &data); err != nil {
		return fail
	}

	// Try to extract message using layered strategy
	message, extractedPath := extractMessage(data)
	if message == "" {
		return fail
	}

	// Remove the extracted message field from data for jsoncontext construction
	removeFieldByPath(data, extractedPath)

	// If no fields remain after removing the message, no context to send.
	if len(data) == 0 {
		return ExtractionResult{
			IsJSON:     true,
			Message:    message,
			MessageKey: extractedPath,
		}
	}

	// Schema-based encoding: extract sorted JSON paths and values.
	keys, values := extractSchemaAndValues(data)

	return ExtractionResult{
		IsJSON:            true,
		Message:           message,
		MessageKey:        extractedPath,
		JSONContextSchema: schemaString(keys),
		JSONContextKeys:   keys,
		JSONContextValues: values,
	}
}

// extractSchemaAndValues extracts sorted JSON paths and their corresponding values from a JSON map.
// Stable shallow primitives are flattened into leaf paths. Deep, wide, map-like, or array values
// are boxed as subtree values to avoid one-off schemas for arbitrary nested JSON.
func extractSchemaAndValues(data map[string]interface{}) ([]string, []interface{}) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]jsonContextItem, 0, len(keys))
	for _, k := range keys {
		appendJSONContextItems(&items, escapeJSONPathSegment(k), data[k], 1)
	}

	contextKeys := make([]string, len(items))
	values := make([]interface{}, len(items))
	for i, item := range items {
		contextKeys[i] = item.key
		values[i] = item.value
	}

	return contextKeys, values
}

func appendJSONContextItems(items *[]jsonContextItem, path string, value interface{}, depth int) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if shouldBoxJSONObject(path, len(typed), depth) {
			appendJSONContextSubtree(items, path, typed)
			return
		}
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			appendJSONContextItems(items, path+"."+escapeJSONPathSegment(key), typed[key], depth+1)
		}
	case []interface{}:
		appendJSONContextSubtree(items, path, typed)
	default:
		*items = append(*items, jsonContextItem{
			key:   path,
			value: normalizeJSONValue(typed),
		})
	}
}

func appendJSONContextSubtree(items *[]jsonContextItem, path string, value interface{}) {
	*items = append(*items, jsonContextItem{
		key:   path,
		value: value,
	})
}

func shouldBoxJSONObject(path string, keyCount int, depth int) bool {
	if keyCount == 0 || keyCount > jsonContextMaxObjectKeys || depth >= jsonContextMaxFlattenDepth {
		return true
	}
	_, ok := boxedSubtreeNames[lastPathSegment(path)]
	return ok
}

func lastPathSegment(path string) string {
	escaped := false
	for index := len(path) - 1; index >= 0; index-- {
		switch path[index] {
		case '\\':
			escaped = !escaped
		case '.':
			if !escaped {
				return path[index+1:]
			}
			escaped = false
		default:
			escaped = false
		}
	}
	return path
}

func escapeJSONPathSegment(segment string) string {
	if !strings.ContainsAny(segment, `.\`) {
		return segment
	}
	var builder strings.Builder
	builder.Grow(len(segment) + 1)
	for _, char := range segment {
		if char == '.' || char == '\\' {
			builder.WriteByte('\\')
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func schemaString(keys []string) string {
	return strings.Join(keys, ",")
}

// normalizeJSONValue preserves primitive JSON types and keeps nested objects/arrays intact.
func normalizeJSONValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return val
	case stdjson.Number:
		return val
	case float64:
		return val
	case bool:
		return val
	case nil:
		return nil
	default:
		return val
	}
}

// extractMessage attempts to extract a message field using the layered strategy
func extractMessage(data map[string]interface{}) (string, string) {
	// Layer 0: Top-level common keys
	for _, key := range topLevelMessageKeys {
		if val, ok := data[key]; ok {
			if str, ok := val.(string); ok && str != "" {
				return str, key
			}
		}
	}

	// Layer 1: Common nested paths (e.g., data.message, event.message)
	for _, path := range nestedMessagePaths {
		if val := getValueByPath(data, path); val != "" {
			return val, path
		}
	}

	return "", ""
}

// getValueByPath retrieves a string value from nested JSON using dot notation
// e.g., "data.message" -> data["data"]["message"]
func getValueByPath(data map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return ""
	}

	current := data
	for _, part := range parts[:len(parts)-1] {
		val, ok := current[part]
		if !ok {
			return ""
		}

		nextMap, ok := val.(map[string]interface{})
		if !ok {
			return ""
		}
		current = nextMap
	}

	leaf, ok := current[parts[len(parts)-1]]
	if !ok {
		return ""
	}
	str, _ := leaf.(string)
	return str
}

// removeFieldByPath removes a field from nested JSON using dot notation
func removeFieldByPath(data map[string]interface{}, path string) {
	if path == "" {
		return
	}

	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		// Top-level key
		delete(data, parts[0])
		return
	}

	// Navigate to parent
	current := data
	for i := 0; i < len(parts)-1; i++ {
		val, ok := current[parts[i]]
		if !ok {
			return
		}
		if nextMap, ok := val.(map[string]interface{}); ok {
			current = nextMap
		} else {
			return
		}
	}

	// Delete the final key
	delete(current, parts[len(parts)-1])
}

// isJSONObject checks if content is a JSON object, handling leading whitespace
func isJSONObject(content []byte) bool {
	trimmed := bytes.TrimLeft(content, " \t\n\r")
	return len(trimmed) > 0 && trimmed[0] == '{'
}
