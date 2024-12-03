#ifndef WIFI_INFO_H
#define WIFI_INFO_H

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

#endif