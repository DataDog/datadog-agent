// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"
	"hash/fnv"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

const (
	MaxErrno    = 35
	MaxErrnoStr = "other"
)

type BPFTelemetry struct {
	MapErrMap    *ebpf.Map
	HelperErrMap *ebpf.Map
	maps         []string
	mapKeys      map[string]uint64
}

func NewBPFTelemetry() *BPFTelemetry {
	b := new(BPFTelemetry)
	b.mapKeys = make(map[string]uint64)

	return b
}

func (b *BPFTelemetry) RegisterMaps(maps []string) error {
	b.maps = append(b.maps, maps...)
	return b.initializeMapErrTelemetryMap()
}

func (b *BPFTelemetry) GetMapsTelemetry() map[string]interface{} {
	var val MapErrTelemetry
	t := make(map[string]interface{})

	for m, k := range b.mapKeys {
		err := b.MapErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
		}
		t[m] = getMapErrCount(&val)
	}

	return t
}

func getMapErrCount(v *MapErrTelemetry) map[string]uint32 {
	var errCount map[string]uint32

	for i, count := range v.Count {
		if count == 0 {
			continue
		}

		if (i + 1) == MaxErrno {
			errCount[MaxErrnoStr] = count
		} else {
			errCount[syscall.Errno(count).Error()] = count
		}
	}

	return errCount
}

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

func (b *BPFTelemetry) initializeMapErrTelemetryMap() error {
	z := new(MapErrTelemetry)
	h := fnv.New64a()

	for _, m := range b.maps {
		h.Write([]byte(m))
		key := h.Sum64()
		err := b.MapErrMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", m)
		}
		h.Reset()

		b.mapKeys[m] = key
	}

	return nil
}
