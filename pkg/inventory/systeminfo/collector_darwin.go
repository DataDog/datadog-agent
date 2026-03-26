// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package systeminfo

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework IOKit

#include <stdlib.h>
#include "systeminfo_darwin.h"
*/
import "C"
import (
	"strings"
	"unsafe"
)

func collect() (*SystemInfo, error) {
	cInfo := C.getDeviceInfo()
	defer C.free(unsafe.Pointer(cInfo.modelIdentifier))
	defer C.free(unsafe.Pointer(cInfo.modelNumber))
	defer C.free(unsafe.Pointer(cInfo.productName))
	defer C.free(unsafe.Pointer(cInfo.serialNumber))

	return &SystemInfo{
		Manufacturer: "Apple Inc.",
		ModelNumber:  C.GoString(cInfo.modelNumber),
		SerialNumber: C.GoString(cInfo.serialNumber),
		ModelName:    C.GoString(cInfo.productName),
		ChassisType:  getChassisType(C.GoString(cInfo.productName), C.GoString(cInfo.modelIdentifier)),
		Identifier:   C.GoString(cInfo.modelIdentifier),
	}, nil
}

func getChassisType(productName string, modelIdentifier string) string {
	lowerName := strings.ToLower(productName)
	lowerModel := strings.ToLower(modelIdentifier)

	// Check for virtual machines first
	// VMware VMs have modelIdentifier like "VMware7,1"
	// Apple Silicon VMs have modelIdentifier like "VirtualMac2,1" and productName "Apple Virtual Machine 1"
	// Parallels VMs have "Parallels" in the modelIdentifier
	if strings.Contains(lowerModel, "vmware") ||
		strings.Contains(lowerModel, "virtual") ||
		strings.Contains(lowerModel, "parallels") ||
		strings.Contains(lowerName, "virtual") {
		return "Virtual Machine"
	}

	if strings.HasPrefix(lowerName, "macbook") {
		return "Laptop"
	}

	if strings.HasPrefix(lowerName, "imac") ||
		strings.HasPrefix(lowerName, "mac mini") ||
		strings.HasPrefix(lowerName, "mac pro") ||
		strings.HasPrefix(lowerName, "mac studio") {
		return "Desktop"
	}

	return "Other"
}
