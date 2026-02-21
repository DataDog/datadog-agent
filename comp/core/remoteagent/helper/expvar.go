// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package helper

import (
	"encoding/json"
	"expvar"
)

// ExpvarFields returns a map of all currently registered expvar key-value pairs
func ExpvarFields() map[string]string {
	fields := make(map[string]string)
	expvar.Do(func(kv expvar.KeyValue) {
		var v any
		if err := json.Unmarshal([]byte(kv.Value.String()), &v); err == nil {
			if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
				fields[kv.Key] = string(pretty)
				return
			}
		}
		fields[kv.Key] = kv.Value.String()
	})
	return fields
}

// ExpvarData returns all currently registered expvar values as a structured map
func ExpvarData() map[string]any {
	data := make(map[string]any)
	expvar.Do(func(kv expvar.KeyValue) {
		var v any
		if err := json.Unmarshal([]byte(kv.Value.String()), &v); err == nil {
			data[kv.Key] = v
		} else {
			data[kv.Key] = kv.Value.String()
		}
	})
	return data
}
