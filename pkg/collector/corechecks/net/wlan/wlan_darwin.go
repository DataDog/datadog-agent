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
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func GetWiFiInfo() (WiFiInfo, error) {
	info := C.GetWiFiInformation()
	return WiFiInfo{
		Rssi:         int(info.rssi),
		Ssid:         C.GoString(info.ssid),
		Bssid:        C.GoString(info.bssid),
		Channel:      int(info.channel),
		Noise:        int(info.noise),
		TransmitRate: float64(info.transmitRate),
		SecurityType: C.GoString(info.securityType),
	}, nil
}

func SetupLocationAccess() {
	C.InitLocationManager()
	log.Info("Initialized Location Manager")

	// Start fetching location updates
	C.StartLocationUpdates()
	log.Info("Started Location Updates")

	// TODO: Is this sleep necessary?
	// Keep the Go program running to allow location updates to be received.
	time.Sleep(30 * time.Second)

	// Stop fetching location updates
	C.StopLocationUpdates()
	log.Info("Stopped Location Updates")
}
