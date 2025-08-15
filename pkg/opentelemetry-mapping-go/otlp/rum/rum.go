// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rum

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func buildRumPayload(k string, v pcommon.Value, rumPayload map[string]any) {
	parts := strings.Split(k, ".")
	current := rumPayload

	for i, part := range parts {
		if i != len(parts)-1 {
			existing, ok := current[part]
			switch {
			case !ok:
				current[part] = make(map[string]any)
			default:
				if _, isMap := existing.(map[string]any); !isMap {
					// force override if it's not a map
					current[part] = make(map[string]any)
				}
			}
			current = current[part].(map[string]any)
			continue
		}

		switch v.Type() {
		case pcommon.ValueTypeSlice:
			current[part] = v.Slice().AsRaw()
		case pcommon.ValueTypeMap:
			if v.Map().Len() == 0 {
				current[part] = nil
				return
			}
			processedMap := make(map[string]any)
			v.Map().Range(func(mapKey string, mapValue pcommon.Value) bool {
				buildRumPayload(mapKey, mapValue, processedMap)
				return true
			})
			current[part] = processedMap
		case pcommon.ValueTypeBytes:
			if v.Bytes().Len() == 0 {
				current[part] = nil
				return
			}
			current[part] = v.AsRaw()
		default:
			current[part] = v.AsRaw()
		}
	}
}

func ConstructRumPayloadFromOTLP(attr pcommon.Map) map[string]any {
	rumPayload := make(map[string]any)
	attr.Range(func(k string, v pcommon.Value) bool {
		if rumAttributeName, exists := OTLPAttributeToRUMPayloadKeyMapping[k]; exists {
			buildRumPayload(rumAttributeName, v, rumPayload)
			return true
		}

		trimmedKey := strings.TrimPrefix(k, "datadog.")
		buildRumPayload(trimmedKey, v, rumPayload)
		return true
	})
	return rumPayload
}
