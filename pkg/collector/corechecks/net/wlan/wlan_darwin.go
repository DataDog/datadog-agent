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
*/
import "C"
import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func GetWiFiInfo() (WiFiInfo, error) {
	C.InitLocationServices()
	log.Info("Initialized Location Manager")

	info := C.GetWiFiInformation()
	return WiFiInfo{
		Rssi:            int(info.rssi),
		Ssid:            C.GoString(info.ssid),
		Bssid:           C.GoString(info.bssid),
		Channel:         int(info.channel),
		Noise:           int(info.noise),
		TransmitRate:    float64(info.transmitRate),
		HardwareAddress: C.GoString(info.hardwareAddress),
		ActivePHYMode:   int(info.activePHYMode),
	}, nil
}
