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
    int channel;
    int noise;
    double transmitRate;
    const char *securityType;
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

type WiFiInfo struct {
	Rssi         int     `json:"rssi"`
	Ssid         string  `json:"ssid"`
	Bssid        string  `json:"bssid"`
	Channel      int     `json:"channel"`
	Noise        int     `json:"noise"`
	TransmitRate float64 `json:"transmit_rate"`
	SecurityType string  `json:"security_type"`
}

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
