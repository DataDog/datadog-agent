// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"
)

// NormalizeListShapeEntries normalizes a list-shape additional_endpoints config value into
// []map[string]any, handling both []any of map[any]any (YAML-sourced) and []map[string]any
// (registered defaults). Returns ok=false for any other shape.
func NormalizeListShapeEntries(raw any) ([]map[string]any, bool) {
	switch typed := raw.(type) {
	case []any:
		entries := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			switch m := item.(type) {
			case map[string]any:
				entries = append(entries, m)
			case map[any]any:
				converted := make(map[string]any, len(m))
				for k, v := range m {
					converted[fmt.Sprintf("%v", k)] = v
				}
				entries = append(entries, converted)
			}
		}
		return entries, true
	case []map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

// CaseInsensitiveStringField looks up a string field case-insensitively.
func CaseInsensitiveStringField(entry map[string]any, field string) (string, bool) {
	for k, v := range entry {
		if !strings.EqualFold(k, field) {
			continue
		}
		s, ok := v.(string)
		return s, ok
	}
	return "", false
}
