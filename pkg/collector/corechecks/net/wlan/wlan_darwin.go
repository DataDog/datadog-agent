// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

/*
#cgo CFLAGS: -I .
#cgo LDFLAGS: -framework CoreWLAN -framework CoreLocation -framework Foundation
#include "wlan_darwin.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func GetWiFiInfo() (WiFiInfo, error) {
	info := C.GetWiFiInformation()
	errorMessage := C.GoString(info.errorMessage)

	ssid := C.GoString(info.ssid)
	bssid := C.GoString(info.bssid)
	hardwareAddress := C.GoString(info.hardwareAddress)

	// important: free the strings that we manually copied (strdup) on the Objective C side
	C.free(unsafe.Pointer(info.ssid))
	C.free(unsafe.Pointer(info.bssid))
	C.free(unsafe.Pointer(info.hardwareAddress))

	wifiInfo := WiFiInfo{
		Rssi:            int(info.rssi),
		Ssid:            ssid,
		Bssid:           bssid,
		Channel:         int(info.channel),
		Noise:           int(info.noise),
		TransmitRate:    float64(info.transmitRate),
		HardwareAddress: hardwareAddress,
		ActivePHYMode:   int(info.activePHYMode),
	}

	var err error
	if errorMessage != "" {
		err = fmt.Errorf("%s", errorMessage)
	}

	return wifiInfo, err
}
