// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package http2

import (
	"strings"

	"github.com/cilium/ebpf"
)

// CleanHTTP2Maps deletes all entries from the http2 maps. Test utility to allow reusing USM instance without carring
// over previous data.
func CleanHTTP2Maps() {
	if Spec.Instance == nil {
		return
	}

	m := Spec.Instance.(*Protocol).mgr
	if m == nil {
		return
	}

	for _, mapElement := range Spec.Maps {
		if strings.HasSuffix(mapElement.Name, "http2_batches") || strings.HasSuffix(mapElement.Name, "http2_telemetry") {
			continue
		}
		mapInstance, _, err := m.GetMap(mapElement.Name)
		if err != nil {
			continue
		}
		// We shouldn't clean percpu maps and percpu arrays.
		if mapInstance.Type() == ebpf.PerCPUArray || mapInstance.Type() == ebpf.Array || mapInstance.Type() == ebpf.PerCPUHash {
			continue
		}
		key := make([]byte, mapInstance.KeySize())
		value := make([]byte, mapInstance.ValueSize())
		mapEntries := mapInstance.Iterate()
		var keysToDelete [][]byte
		for mapEntries.Next(&key, &value) {
			keysToDelete = append(keysToDelete, key)
		}
		for _, key := range keysToDelete {
			_ = mapInstance.Delete(&key)
		}
	}
}
