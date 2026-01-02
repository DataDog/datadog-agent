// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processor provides JSON-aware preprocessing for stateful log encoding.
// It extracts message fields from JSON logs and serializes remaining fields into ordered json_context.
package processor

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ExtractionResult contains the result of JSON preprocessing
type ExtractionResult struct {
	// IsJSON indicates whether the input was valid JSON
	IsJSON bool
	// Message is the extracted message field (empty if not found or not JSON)
	Message string
	// JSONContext is the ordered, serialized remaining JSON fields (nil if not JSON or extraction failed)
	JSONContext []byte
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

// PreprocessJSON attempts to extract a message field from JSON logs and serialize remaining fields.
func PreprocessJSON(content []byte) ExtractionResult {
	fail := ExtractionResult{IsJSON: false}

	// Check if it's a JSON object (handles leading whitespace)
	if !isJSONObject(content) {
		return fail
	}

	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return fail
	}

	// Try to extract message using layered strategy
	message, extractedPath := extractMessage(data)
	if message == "" {
		return fail
	}

	// Remove the extracted message field from data for jsoncontext construction
	removeFieldByPath(data, extractedPath)

	// If no fields remain after removing the message, keep json_context nil (avoid sending "{}").
	if len(data) == 0 {
		return ExtractionResult{
			IsJSON:      true,
			Message:     message,
			JSONContext: nil,
		}
	}

	// Serialize remaining fields as JSON context.
	// encoding/json marshals maps with deterministic key ordering for better compression.
	jsonContext, err := json.Marshal(data)
	if err != nil {
		return fail
	}

	return ExtractionResult{
		IsJSON:      true,
		Message:     message,
		JSONContext: jsonContext,
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
