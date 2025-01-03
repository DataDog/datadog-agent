#ifndef WLAN_DARWIN_H
#define WLAN_DARWIN_H

typedef struct {
    int rssi;
    const char *ssid;
    const char *bssid;
    int channel;
    int noise;
    double transmitRate;
    const char *hardwareAddress;
    int activePHYMode;
} WiFiInfo;

WiFiInfo GetWiFiInformation();
void InitLocationServices();

#endif
