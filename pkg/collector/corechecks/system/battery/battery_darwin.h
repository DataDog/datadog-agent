#ifndef BATTERY_DARWIN_H
#define BATTERY_DARWIN_H

#include <stdbool.h>

typedef struct {
    int cycleCount;
    int designCapacity;
    int appleRawMaxCapacity;
    int currentCapacity;
    int voltage;           // mV
    int instantAmperage;   // mA (negative = discharging, positive = charging)
    bool isCharging;        // 1 if charging, 0 otherwise
    bool externalConnected; // 1 if AC power connected, 0 otherwise
    bool found;             // 1 if battery found, 0 otherwise
} BatteryInfo;

BatteryInfo getBatteryInfo(void);

#endif
