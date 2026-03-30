// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package json

import (
	"encoding/json"
	"fmt"
	"io"
)

// removeEmptyFields recursively removes empty string fields from JSON data.
// Preserves empty maps and arrays as they may be part of the API contract.
func removeEmptyFields(data any) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, val := range v {
			cleaned := removeEmptyFields(val)
			// Keep if not nil and not empty string
			if str, ok := cleaned.(string); cleaned != nil && (!ok || str != "") {
				result[key] = cleaned
			}
		}
		return result
	case []any:
		result := make([]any, 0, len(v))
		for _, elem := range v {
			cleaned := removeEmptyFields(elem)
			// Keep if not nil and not empty string
			if str, ok := cleaned.(string); cleaned != nil && (!ok || str != "") {
				result = append(result, cleaned)
			}
		}
		return result
	default:
		return data
	}
}

// PrintJSON writes JSON output to the provided writer, optionally pretty-printed.
func PrintJSON(w io.Writer, rawJSON any, prettyPrintJSON bool, removeEmpty bool, searchTerm string) error {
	var result []byte
	var err error

	// Unmarshal if input is json.RawMessage
	if v, ok := rawJSON.(json.RawMessage); ok {
		var unmarshaled any
		if err := json.Unmarshal(v, &unmarshaled); err != nil {
			return err
		}
		rawJSON = unmarshaled
	}

	// Check for empty results if search term provided
	if searchTerm != "" {
		if m, ok := rawJSON.(map[string]any); ok {
			if entities, ok := m["Entities"].(map[string]any); ok && len(entities) == 0 {
				return fmt.Errorf("no entities found matching %q", searchTerm)
			}
		}
	}

	// Remove empty fields if requested
	if removeEmpty {
		rawJSON = removeEmptyFields(rawJSON)
	}

	// Marshal to bytes
	if prettyPrintJSON {
		result, err = json.MarshalIndent(rawJSON, "", "  ")
	} else {
		result, err = json.Marshal(rawJSON)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(result))
	if err != nil {
		return err
	}

	return nil
}
