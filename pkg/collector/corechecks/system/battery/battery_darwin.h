#ifndef BATTERY_DARWIN_H
#define BATTERY_DARWIN_H

#include <stdbool.h>

typedef struct {
    bool hasValue;
    int value;
} OptionalInt;

typedef struct {
    bool hasValue;
    bool value;
} OptionalBool;

typedef struct {
    OptionalInt cycleCount;
    OptionalInt designCapacity;
    OptionalInt appleRawMaxCapacity;
    OptionalInt currentCapacity;
    OptionalInt voltage;
    OptionalInt instantAmperage;
    OptionalBool isCharging;
    OptionalBool externalConnected;
    OptionalBool isCritical;
} BatteryInfo;

bool hasInternalBattery(void);
BatteryInfo getBatteryInfo(void);

#endif
