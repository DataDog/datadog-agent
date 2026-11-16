// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package nicinfo

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework IOKit -framework SystemConfiguration

#include <stdlib.h>
#include "nicinfo_darwin.h"
*/
import "C"
import (
	"errors"
	"net"
	"unsafe"
)

// Info describes one network interface. Interfaces that are not in the IOKit registry, such as
// tunnels, only have a description, if any.
type Info struct {
	// Description is the display name of the interface, such as "Wi-Fi" or "USB 10/100/1000 LAN"
	Description string
	// PermanentMAC is the MAC address of the controller, which private Wi-Fi addresses leave unchanged
	PermanentMAC string
	// Builtin is nil when the IOKit registry does not have the interface
	Builtin *bool
	Vendor  string
	Model   string
	// DriverClass is the IOKit class of the controller, such as AppleUSBDeviceNCMData
	DriverClass string
	// Driver is the bundle identifier of the controller's driver
	Driver        string
	DriverVersion string
}

// Get returns the interfaces by BSD name.
func Get() (map[string]Info, error) {
	var count, failed C.int
	cInfos := C.copyNICInfo(&count, &failed)
	if failed != 0 {
		return nil, errors.New("unable to list the network interfaces of IOKit and SystemConfiguration")
	}
	if cInfos == nil {
		return map[string]Info{}, nil
	}
	defer C.freeNICInfo(cInfos, count)

	infos := make(map[string]Info, int(count))
	for _, cInfo := range unsafe.Slice(cInfos, int(count)) {
		info := Info{
			Description:   C.GoString(cInfo.description),
			Vendor:        C.GoString(cInfo.vendor),
			Model:         C.GoString(cInfo.model),
			DriverClass:   C.GoString(cInfo.driverClass),
			Driver:        C.GoString(cInfo.driver),
			DriverVersion: C.GoString(cInfo.driverVersion),
		}
		if cInfo.macLength > 0 {
			info.PermanentMAC = net.HardwareAddr(C.GoBytes(unsafe.Pointer(&cInfo.mac[0]), cInfo.macLength)).String()
		}
		if cInfo.inRegistry != 0 {
			builtin := cInfo.builtin != 0
			info.Builtin = &builtin
		}
		infos[C.GoString(cInfo.name)] = info
	}
	return infos, nil
}
