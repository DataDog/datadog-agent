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

// RemoveEmptyFields recursively removes empty/zero-value fields from JSON data.
func RemoveEmptyFields(data any) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, val := range v {
			if cleaned := RemoveEmptyFields(val); !isEmpty(cleaned) {
				result[key] = cleaned
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	case []any:
		result := make([]any, 0, len(v))
		for _, elem := range v {
			if cleaned := RemoveEmptyFields(elem); !isEmpty(cleaned) {
				result = append(result, cleaned)
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	default:
		return data
	}
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case bool:
		return !val
	case float64: // JSON numbers are always float64
		return val == 0
	case map[string]any, []any:
		return false // handled by len check in RemoveEmptyFields
	default:
		return false
	}
}

// PrintJSON writes JSON output to the provided writer, optionally pretty-printed
func PrintJSON(w io.Writer, rawJSON any, prettyPrintJSON bool) error {
	var result []byte
	var err error

	// convert to bytes and indent
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
