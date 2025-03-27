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
	"errors"
	"unsafe"
)

// phyMode represents the PHY mode of the WiFi connection.
type phyMode int

// See: https://developer.apple.com/documentation/corewlan/cwphymode?language=objc
const (
	phyModeNone phyMode = iota // No PHY mode.
	phyMode11a                 // IEEE 802.11a PHY.
	phyMode11b                 // IEEE 802.11b PHY.
	phyMode11g                 // IEEE 802.11g PHY.
	phyMode11n                 // IEEE 802.11n PHY.
	phyMode11ac                // IEEE 802.11ac PHY.
)

func (phy phyMode) String() string {
	switch phy {
	case phyModeNone:
		return "None"
	case phyMode11a:
		return "802.11a"
	case phyMode11b:
		return "802.11b"
	case phyMode11g:
		return "802.11g"
	case phyMode11n:
		return "802.11n"
	case phyMode11ac:
		return "802.11ac"
	default:
		return ""
	}
}

func GetWiFiInfo() (WiFiInfo, error) {
	info := C.GetWiFiInformation()

	ssid := C.GoString(info.ssid)
	bssid := C.GoString(info.bssid)
	hardwareAddress := C.GoString(info.hardwareAddress)
	errorMessage := C.GoString(info.errorMessage)

	// important: free the C strings fields
	if info.ssid != nil {
		C.free(unsafe.Pointer(info.ssid))
	}
	if info.bssid != nil {
		C.free(unsafe.Pointer(info.bssid))
	}
	if info.hardwareAddress != nil {
		C.free(unsafe.Pointer(info.hardwareAddress))
	}
	if info.errorMessage != nil {
		C.free(unsafe.Pointer(info.errorMessage))
	}

	wifiInfo := WiFiInfo{
		Rssi:         int(info.rssi),
		Ssid:         ssid,
		Bssid:        bssid,
		Channel:      int(info.channel),
		Noise:        int(info.noise),
		TransmitRate: float64(info.transmitRate), // in Mbps
		MacAddress:   hardwareAddress,
		PHYMode:      phyMode(info.activePHYMode),
	}

	var err error
	if errorMessage != "" {
		err = errors.New(errorMessage)
	}

	return wifiInfo, err
}
