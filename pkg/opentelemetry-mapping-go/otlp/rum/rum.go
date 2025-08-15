// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rum

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func buildRumPayload(k string, v pcommon.Value, rumPayload map[string]any) {
	parts := strings.Split(k, ".")

	current := rumPayload
	for i, part := range parts {
		if i == len(parts)-1 {
			if v.Type() == pcommon.ValueTypeSlice {
				current[part] = v.Slice().AsRaw()
			} else if v.Type() == pcommon.ValueTypeMap {
				// handle map values by recursively processing nested keys
				mapVal := v.Map().AsRaw()
				if mapVal == nil {
					current[part] = nil
				} else {
					processedMap := make(map[string]any)
					v.Map().Range(func(mapKey string, mapValue pcommon.Value) bool {
						buildRumPayload(mapKey, mapValue, processedMap)
						return true
					})
					current[part] = processedMap
				}
			} else {
				if v.Type() == pcommon.ValueTypeBytes && v.Bytes().Len() == 0 {
					current[part] = nil
				} else {
					current[part] = v.AsRaw()
				}
			}
		} else {
			if _, ok := current[part]; !ok {
				current[part] = make(map[string]any)
			}

			// in case the current part is not a map, we should override it with a map to avoid type assertion errors
			next, ok := current[part].(map[string]any)
			if !ok {
				next = make(map[string]any)
				current[part] = next
			}
			current = next
		}
	}
}

func ConstructRumPayloadFromOTLP(attr pcommon.Map) map[string]any {
	rumPayload := make(map[string]any)
	attr.Range(func(k string, v pcommon.Value) bool {
		if rumAttributeName, exists := OTLPAttributeToRUMPayloadKeyMapping[k]; exists {
			buildRumPayload(rumAttributeName, v, rumPayload)
		} else {
			buildRumPayload(strings.TrimPrefix(k, "datadog."), v, rumPayload)
		}
		return true
	})
	return rumPayload
}
