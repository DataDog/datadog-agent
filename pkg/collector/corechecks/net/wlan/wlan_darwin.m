// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#include <Foundation/Foundation.h>
#include <CoreWLAN/CoreWLAN.h>

// WiFiInfo struct to hold WiFi data
typedef struct {
    int rssi;
    const char *ssid;
    const char *bssid;
} WiFiInfo;

// GetWiFiInformation return wifi data
WiFiInfo GetWiFiInformation() {
    CWInterface *wifiInterface = [[CWWiFiClient sharedWiFiClient] interface];

    WiFiInfo info;
    
    info.rssi = (int)wifiInterface.rssiValue;
    info.ssid = [[wifiInterface ssid] UTF8String];
    info.bssid = [[wifiInterface bssid] UTF8String];

    return info;
}