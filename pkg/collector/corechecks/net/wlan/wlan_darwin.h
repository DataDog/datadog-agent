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
    const char *errorMessage;
} WiFiInfo;

WiFiInfo GetWiFiInformation();
void InitLocationServices();

#endif
