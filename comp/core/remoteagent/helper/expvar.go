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

// ExpvarFields returns a map of all currently registered expvar key-value pairs.
// Each value is the JSON string returned by the expvar's String() method.
func ExpvarFields() map[string]string {
	fields := make(map[string]string)
	expvar.Do(func(kv expvar.KeyValue) {
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

// ExpvarData returns all currently registered expvar values as a structured map.
// Values are unmarshaled from JSON where possible; otherwise stored as raw strings.
func ExpvarData() map[string]any {
	data := make(map[string]any)
	expvar.Do(func(kv expvar.KeyValue) {
		var v any
		if json.Unmarshal([]byte(kv.Value.String()), &v) != nil {
			v = kv.Value.String()
		}
		data[kv.Key] = v
	})
	return data
}

// DefaultFlareFiles returns the standard set of flare files for a sub-process agent:
// "<prefix>_status.json" with all current expvar values and "<prefix>_runtime_config_dump.json" with all config settings.
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
