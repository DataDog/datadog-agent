// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && !ios && cgo

package gpu

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework CoreFoundation -framework IOKit

#include "agx_darwin.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const maxAGXDevices = 16

type agxDeviceSnapshot struct {
	model                    string
	coreCount                int64
	hasCoreCount             bool
	utilization              float64
	hasUtilization           bool
	allocatedSystemMemory    int64
	hasAllocatedSystemMemory bool
	inUseSystemMemory        int64
	hasInUseSystemMemory     bool
}

type agxCollection struct {
	devices            []agxDeviceSnapshot
	propertyReadErrors uint32
	truncated          bool
}

func readAGXDevices() (agxCollection, error) {
	devices := make([]C.DDAGXDeviceInfo, maxAGXDevices)
	result := C.dd_collect_agx_devices(
		(*C.DDAGXDeviceInfo)(unsafe.Pointer(&devices[0])),
		C.uint32_t(len(devices)),
	)
	if result.error != 0 {
		return agxCollection{}, fmt.Errorf("IOKit AGX enumeration failed with code %#x", uint32(result.error))
	}

	count := int(result.count)
	if count > len(devices) {
		count = len(devices)
	}
	collection := agxCollection{
		devices:            make([]agxDeviceSnapshot, 0, count),
		propertyReadErrors: uint32(result.propertyReadErrors),
		truncated:          bool(result.truncated),
	}
	for i := 0; i < count; i++ {
		device := devices[i]
		snapshot := agxDeviceSnapshot{
			coreCount:                int64(device.coreCount),
			hasCoreCount:             bool(device.hasCoreCount),
			utilization:              float64(device.utilization),
			hasUtilization:           bool(device.hasUtilization),
			allocatedSystemMemory:    int64(device.allocatedSystemMemory),
			hasAllocatedSystemMemory: bool(device.hasAllocatedSystemMemory),
			inUseSystemMemory:        int64(device.inUseSystemMemory),
			hasInUseSystemMemory:     bool(device.hasInUseSystemMemory),
		}
		if bool(device.hasModel) {
			snapshot.model = C.GoString(&device.model[0])
		}
		collection.devices = append(collection.devices, snapshot)
	}

	return collection, nil
}
