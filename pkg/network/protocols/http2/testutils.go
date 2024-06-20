// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package http2

import (
	"strings"
	"testing"

	"github.com/cilium/ebpf"
)

var (
	mapTypesToZero = map[ebpf.MapType]struct{}{
		ebpf.PerCPUArray: {},
		ebpf.Array:       {},
		ebpf.PerCPUHash:  {},
	}
)

// CleanHTTP2Maps deletes all entries from the http2 maps. Test utility to allow reusing USM instance without caring
// over previous data.
func CleanHTTP2Maps(t *testing.T) {
	if Spec.Instance == nil {
		t.Log("http2 protocol not initialized")
		return
	}

	m := Spec.Instance.(*Protocol).mgr
	if m == nil {
		t.Log("http2 manager not initialized")
		return
	}

	// Getting all maps loaded into the manager
	maps, err := m.GetMaps()
	if err != nil {
		t.Logf("failed to get maps: %v", err)
		return
	}
	for mapName, mapInstance := range maps {
		// We only want to clean http2 maps
		if !strings.Contains(mapName, "http2") {
			continue
		}
		// Special case for batches, as the values is never "empty", but contain the CPU number.
		if strings.HasSuffix(mapName, "http2_batches") {
			continue
		}
		_, shouldOnlyZero := mapTypesToZero[mapInstance.Type()]

		key := make([]byte, mapInstance.KeySize())
		value := make([]byte, mapInstance.ValueSize())
		mapEntries := mapInstance.Iterate()
		var keys [][]byte
		for mapEntries.Next(&key, &value) {
			keys = append(keys, key)
		}

		if shouldOnlyZero {
			emptyValue := make([]byte, mapInstance.ValueSize())
			for _, key := range keys {
				if err := mapInstance.Put(&key, &emptyValue); err != nil {
					t.Log("failed zeroing map entry; error: ", err)
				}
			}
		} else {
			for _, key := range keys {
				if err := mapInstance.Delete(&key); err != nil {
					t.Log("failed deleting map entry; error: ", err)
				}
			}
		}
	}
}
