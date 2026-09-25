// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"strings"
	"time"
)

func flattenJSONLeaves(basePath string, keys map[string]string, value any, timestamp time.Time) []CachedValue {
	rootMap, ok := asJSONMap(value)
	if !ok {
		return nil
	}

	leaves := make([]CachedValue, 0, len(rootMap))
	var walk func(path string, current any)
	walk = func(path string, current any) {
		nested, ok := asJSONMap(current)
		if ok {
			for childName, childValue := range nested {
				walk(path+"/"+canonicalizeLeafName(childName), childValue)
			}
			return
		}

		leaves = append(leaves, CachedValue{
			Key: CacheKey{
				Path: path,
				Keys: cloneKeys(keys),
			},
			Entry: CacheEntry{
				Value:     current,
				Timestamp: timestamp,
				Keys:      cloneKeys(keys),
			},
		})
	}

	for childName, childValue := range rootMap {
		walk(basePath+"/"+canonicalizeLeafName(childName), childValue)
	}
	return leaves
}

func asJSONMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return typed, true
}

func canonicalizeLeafName(name string) string {
	_, local, ok := splitModuleQualifiedName(name)
	if ok {
		return local
	}
	return name
}

func expandModuleQualifiedSegment(segment string) []string {
	module, local, ok := splitModuleQualifiedName(segment)
	if !ok {
		return []string{segment}
	}
	if strings.HasPrefix(module, "openconfig") && isOpenConfigRootSegment(local) {
		return []string{"openconfig", local}
	}
	return []string{local}
}

func splitModuleQualifiedName(segment string) (module string, local string, ok bool) {
	idx := strings.LastIndex(segment, ":")
	if idx <= 0 || idx == len(segment)-1 {
		return "", segment, false
	}
	return segment[:idx], segment[idx+1:], true
}

func isOpenConfigRootSegment(local string) bool {
	switch local {
	case "interfaces", "system", "components", "lldp", "network-instances", "routing":
		return true
	default:
		return false
	}
}
