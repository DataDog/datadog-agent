// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package helper

import (
	"encoding/json"
	"expvar"

	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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

// DefaultStatusResponse returns a GetStatusDetailsResponse populated with all in-process expvar data.
func DefaultStatusResponse() *pbcore.GetStatusDetailsResponse {
	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{Fields: ExpvarFields()},
	}
}

// DefaultFlareFiles returns the standard set of flare files for a sub-process agent:
func DefaultFlareFiles(settings map[string]interface{}, prefix string) map[string][]byte {
	files := make(map[string][]byte)
	if data, err := json.MarshalIndent(ExpvarData(), "", "  "); err == nil {
		files[prefix+"_status.json"] = data
	}
	if data, err := json.MarshalIndent(settings, "", "  "); err == nil {
		files[prefix+"_runtime_config_dump.json"] = data
	}
	return files
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
