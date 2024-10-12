// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

/*
#cgo LDFLAGS: -framework CoreWLAN -framework Foundation
#include <stdbool.h>
#include <stdlib.h>

typedef struct {
    int rssi;
    const char *ssid;
    const char *bssid;
} WiFiInfo;

WiFiInfo GetWiFiInformation();
*/
import "C"
import "fmt"

type WiFiData struct {
	rssi  int
	ssid  string
	bssid string
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
