// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

/*
#cgo LDFLAGS: -framework CoreWLAN -framework CoreLocation -framework Foundation

typedef struct {
    int rssi;
    const char *ssid;
    const char *bssid;
} WiFiInfo;

WiFiInfo GetWiFiInformation();
void InitLocationManager();
void StartLocationUpdates();
void StopLocationUpdates();
*/
import "C"
import (
	"fmt"
	"time"
)

type WiFiData struct {
	rssi  int
	ssid  string
	bssid string
}

func setupLocationAccess() {
	C.InitLocationManager()
	fmt.Println("Initialized Location Manager")

	// Start fetching location updates
	C.StartLocationUpdates()
	fmt.Println("Started Location Updates")

	// Keep the Go program running to allow location updates to be received.
	time.Sleep(30 * time.Second)

	// Stop fetching location updates
	C.StopLocationUpdates()
	fmt.Println("Stopped Location Updates")
}

func queryWiFiRSSI() (WiFiData, error) {
	wiFiData := C.GetWiFiInformation()
	fmt.Println(wiFiData)
	return WiFiData{
		rssi:  int(wiFiData.rssi),
		ssid:  C.GoString(wiFiData.ssid),
		bssid: C.GoString(wiFiData.bssid),
	}, nil
}
