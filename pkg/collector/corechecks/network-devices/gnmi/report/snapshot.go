// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

type snapshotIndex map[string][]client.CachedValue

func indexSnapshot(snapshot []client.CachedValue) snapshotIndex {
	byPath := make(snapshotIndex, len(snapshot))
	for _, cached := range snapshot {
		byPath[cached.Key.Path] = append(byPath[cached.Key.Path], cached)
	}
	return byPath
}

func firstStringValue(index snapshotIndex, path string, keys map[string]string) string {
	if len(keys) > 0 {
		cached, ok := lookupCachedValue(index, path, keys)
		if !ok {
			return ""
		}
		value, ok := stringValue(cached.Entry.Value)
		if !ok {
			return ""
		}
		return value
	}

	for _, cached := range sortCachedValues(index[path]) {
		if value, ok := stringValue(cached.Entry.Value); ok {
			return value
		}
	}
	return ""
}

// ResolveDeviceHostname returns the device hostname from the gNMI snapshot.
func ResolveDeviceHostname(snapshot []client.CachedValue, metadata config.MetadataConfig) string {
	return resolveDeviceHostname(indexSnapshot(snapshot), metadata)
}

func resolveDeviceHostname(index snapshotIndex, metadata config.MetadataConfig) string {
	resolved := metadata.Resolved()
	if hostname := firstStringValue(index, resolved.Device.Hostname, nil); hostname != "" {
		return hostname
	}

	for path, values := range index {
		if !strings.HasSuffix(path, "/hostname") {
			continue
		}
		for _, cached := range values {
			if value, ok := stringValue(cached.Entry.Value); ok {
				return value
			}
		}
	}
	return ""
}

func firstInt32Value(index snapshotIndex, path string, keys map[string]string) (int32, bool) {
	if len(keys) > 0 {
		cached, ok := lookupCachedValue(index, path, keys)
		if !ok {
			return 0, false
		}
		value, ok := int64Value(cached.Entry.Value)
		if !ok {
			return 0, false
		}
		return int32(value), true
	}

	for _, cached := range sortCachedValues(index[path]) {
		if value, ok := int64Value(cached.Entry.Value); ok {
			return int32(value), true
		}
	}
	return 0, false
}

func lookupCachedValue(index snapshotIndex, path string, keys map[string]string) (client.CachedValue, bool) {
	for _, cached := range index[path] {
		if keysEqual(cached.Key.Keys, keys) {
			return cached, true
		}
	}
	return client.CachedValue{}, false
}

func lookupNumericValue(index snapshotIndex, path string, keys map[string]string) (float64, bool) {
	cached, ok := lookupCachedValue(index, path, keys)
	if !ok {
		return 0, false
	}
	return decodeNumericValue(cached.Entry.Value)
}

func indexCachedValuesByKeys(entries []client.CachedValue) map[string]client.CachedValue {
	indexed := make(map[string]client.CachedValue, len(entries))
	for _, cached := range entries {
		indexed[cacheKeysID(cached.Key.Keys)] = cached
	}
	return indexed
}

func cacheKeysID(keys map[string]string) string {
	if len(keys) == 0 {
		return ""
	}
	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+keys[name])
	}
	return strings.Join(parts, ",")
}

func pathsShareParent(pathA, pathB string) (string, bool) {
	parentA, okA := pathParent(pathA)
	parentB, okB := pathParent(pathB)
	if !okA || !okB {
		return "", false
	}
	if parentA != parentB {
		return "", false
	}
	return parentA, true
}

func pathParent(path string) (string, bool) {
	trimmed := strings.TrimSuffix(path, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx <= 0 {
		return "", false
	}
	return trimmed[:idx], true
}

func keysEqual(actual, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func sortCachedValues(entries []client.CachedValue) []client.CachedValue {
	if len(entries) <= 1 {
		return entries
	}
	sorted := append([]client.CachedValue(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key.String() < sorted[j].Key.String()
	})
	return sorted
}

const componentPathSegment = "/components/component/"

func componentNameKey(metadata config.MetadataConfig) string {
	for _, keyName := range metadata.Resolved().Device.Keys {
		if keyName != "" {
			return keyName
		}
	}
	return "name"
}

func componentNames(index snapshotIndex, metadata config.MetadataConfig) []string {
	nameKey := componentNameKey(metadata)
	names := make(map[string]struct{})
	for path, values := range index {
		if !strings.Contains(path, componentPathSegment) {
			continue
		}
		for _, cached := range values {
			if name := cached.Key.Keys[nameKey]; name != "" {
				names[name] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func resolveDeviceComponentName(index snapshotIndex, metadata config.MetadataConfig) string {
	names := componentNames(index, metadata)
	if len(names) == 0 {
		return ""
	}
	for _, name := range names {
		if strings.EqualFold(name, "Chassis") {
			return name
		}
	}
	return names[0]
}

func keysMatch(actual, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func stringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return "", false
		}
		return trimmed, true
	case []byte:
		if len(typed) == 0 {
			return "", false
		}
		return formatColonSepBytes(typed), true
	default:
		if value == nil {
			return "", false
		}
		formatted := strings.TrimSpace(fmt.Sprint(value))
		if formatted == "" {
			return "", false
		}
		return formatted, true
	}
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func formatColonSepBytes(val []byte) string {
	octetsList := make([]string, 0, len(val))
	for _, b := range val {
		octetsList = append(octetsList, hex.EncodeToString([]byte{b}))
	}
	return strings.Join(octetsList, ":")
}

func interfaceNames(index snapshotIndex, metadata config.MetadataConfig, metricPaths []string) []string {
	resolved := metadata.Resolved()
	nameKey := interfaceNameKey(metadata)
	names := make(map[string]struct{})
	collectName := func(name string) {
		if name != "" {
			names[name] = struct{}{}
		}
	}

	for _, cached := range index[resolved.Interface.Name] {
		if name, ok := stringValue(cached.Entry.Value); ok {
			collectName(name)
		}
		collectName(cached.Key.Keys[nameKey])
	}

	for _, path := range resolved.InterfaceLookupPaths() {
		for _, cached := range index[path] {
			collectName(cached.Key.Keys[nameKey])
		}
	}

	for _, path := range metricPaths {
		for _, cached := range index[path] {
			collectName(cached.Key.Keys[nameKey])
		}
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	return out
}

func interfaceNameKey(metadata config.MetadataConfig) string {
	for _, keyName := range metadata.Resolved().Interface.Keys {
		if keyName != "" {
			return keyName
		}
	}
	return "name"
}

func lldpNeighborKeys(index snapshotIndex, topology config.TopologyConfig) []map[string]string {
	if topology.IsZero() {
		return nil
	}

	interfaceKey := topology.NeighborInterfaceKey()
	neighborKey := topology.NeighborIDKey()
	seen := make(map[string]map[string]string)
	for _, path := range topology.LookupPaths() {
		for _, cached := range index[path] {
			interfaceName := cached.Key.Keys[interfaceKey]
			neighborID := cached.Key.Keys[neighborKey]
			if interfaceName == "" || neighborID == "" {
				continue
			}
			key := interfaceName + "\x00" + neighborID
			seen[key] = map[string]string{interfaceKey: interfaceName, neighborKey: neighborID}
		}
	}

	out := make([]map[string]string, 0, len(seen))
	for _, keys := range seen {
		out = append(out, keys)
	}
	return out
}
