// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"
	"hash/fnv"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

func BuildMapErrTelemetryKeys(mgr *manager.Manager) []manager.ConstantEditor {
	var keys []manager.ConstantEditor

	h := fnv.New64a()
	for _, m := range mgr.Maps {
		h.Write([]byte(m.Name))
		keys = append(keys, manager.ConstantEditor{
			Name:  m.Name + "_telemetry_key",
			Value: h.Sum64(),
		})
		h.Reset()
	}

	return keys
}

func InitializeMapErrTelemetryMap(mgr *manager.Manager, errMap *ebpf.Map) error {
	z := new(MapErrTelemetry)
	h := fnv.New64a()

	for _, m := range mgr.Maps {
		h.Write([]byte(m.Name))
		key := h.Sum64()
		err := errMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", m.Name)
		}
		h.Reset()
	}

	return nil
}
