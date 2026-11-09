// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

// ListEntryIdentity fingerprints endpoint routing fields without retaining the API key.
func ListEntryIdentity(entry map[string]any) (string, bool) {
	identityFields := make(map[string]any, len(entry))
	for key, value := range entry {
		if !strings.EqualFold(key, "api_key") {
			identityFields[key] = value
		}
	}
	encoded, err := json.Marshal(identityFields)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), true
}

// NormalizeListShapeEntries normalizes a list-shape additional_endpoints config value into
// []any, handling JSON strings, []any of map[any]any (YAML-sourced), and []map[string]any
// (registered defaults). Map-shaped elements are converted to map[string]any; any other element
// (e.g. a bare string, or nil from a blank YAML list item) is preserved as-is so a round-trip
// write doesn't silently drop it. Callers matching against api_key fields should type-assert each
// element and skip non-map entries. Returns ok=false for any other top-level shape.
func NormalizeListShapeEntries(raw any) ([]any, bool) {
	switch typed := raw.(type) {
	case string:
		var entries []any
		if err := json.Unmarshal([]byte(typed), &entries); err != nil {
			return nil, false
		}
		for i, item := range entries {
			entries[i] = normalizeListEntry(item)
		}
		return entries, true
	case []any:
		entries := make([]any, len(typed))
		for i, item := range typed {
			entries[i] = normalizeListEntry(item)
		}
		return entries, true
	case []map[string]any:
		entries := make([]any, len(typed))
		for i, item := range typed {
			entries[i] = item
		}
		return entries, true
	default:
		return nil, false
	}
}

// normalizeListEntry converts a map-shaped list element to map[string]any, or returns it
// unchanged if it isn't map-shaped.
func normalizeListEntry(item any) any {
	switch m := item.(type) {
	case map[string]any:
		return m
	case map[any]any:
		converted := make(map[string]any, len(m))
		for k, v := range m {
			converted[fmt.Sprintf("%v", k)] = v
		}
		return converted
	default:
		return item
	}
}

// CaseInsensitiveStringField looks up a string field case-insensitively.
func CaseInsensitiveStringField(entry map[string]any, field string) (string, bool) {
	_, value, ok := CaseInsensitiveStringFieldWithKey(entry, field)
	return value, ok
}

// CaseInsensitiveStringFieldWithKey looks up a string field case-insensitively
// and returns its original key casing.
func CaseInsensitiveStringFieldWithKey(entry map[string]any, field string) (string, string, bool) {
	for k, v := range entry {
		if !strings.EqualFold(k, field) {
			continue
		}
		s, ok := v.(string)
		return k, s, ok
	}
	return "", "", false
}
