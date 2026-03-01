// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build (linux && linux_bpf) || darwin

package ebpfless

// WriteMapWithSizeLimit updates a map via m[key] = val.
// However, if the map would overflow sizeLimit, it returns false instead.
func WriteMapWithSizeLimit[Key comparable, Val any](m map[Key]Val, key Key, val Val, sizeLimit int) bool {
	_, exists := m[key]
	if !exists && len(m) >= sizeLimit {
		return false
	}
	m[key] = val
	return true
}
